package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/storage"
)

// SelfCredentialHeader carries the per-self credential on every request that
// addresses a self by id. The credential is minted once, by POST /selves.
const SelfCredentialHeader = "X-Agora-Self-Credential"

// selfAuthRejection is THE rejection for a request that fails self authn.
// One status, one body, for every failure shape — wrong credential, missing
// credential, self that does not exist — so responses carry no existence
// oracle (SEC gate, advisory A).
const selfAuthRejection = "Unauthorized: self authentication failed"

// AgentHandler is the first consumer of the framework's Self: it owns the
// HTTP concerns (parsing, the execution log, the response) and executes every
// run THROUGH the one process-lifetime Self constructed at start-up. It
// conjures no resources of its own — the banks live on the Self.
type AgentHandler struct {
	Repo *storage.Repository

	// Self is the process-lifetime owner of the resource banks. It is
	// constructed ONCE (see cmd/agora-server/main.go) and serves every
	// request; the model-injection test seam now lives on its Blueprint
	// (agora.Blueprint.ModelFactory), not on the handler.
	Self *agora.Self
}

type RunRequest struct {
	Input string `json:"input"`
	Model string `json:"model"` // Optional, default to internal config

	// SelfID addresses a stable self across requests (the Step 2 identity
	// model). When present, the run recalls that self's stored engrams into
	// its starting state and persists the new turns back as engrams.
	// When absent, the run is a legacy amnesiac one-shot.
	SelfID string `json:"self_id"`
}

type RunResponse struct {
	ExecutionID string `json:"execution_id"`
	Status      string `json:"status"`
	Output      string `json:"output"`
}

func (h *AgentHandler) HandleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Strict JSON Parsing
	var req RunRequest
	// Limit request body to 1MB to prevent DOS
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1048576))
	dec.DisallowUnknownFields() // Security: Validation
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 2. Self authn — the POLICY half of the SEC gate (advisory A), enforced
	// at the membrane before anything is logged, resolved, or recalled. A
	// request addressing a self must authenticate as that self; the
	// verification mechanism lives on agora.Self beside the ResolveSelf
	// choke point. Every failure — wrong credential, missing credential,
	// unknown self, even a misconfigured D bank — collapses into the one
	// indistinguishable rejection. The empty-SelfID amnesiac path predates
	// selves and stays open: no self memory is at stake there.
	if req.SelfID != "" {
		if err := h.Self.VerifySelf(req.SelfID, r.Header.Get(SelfCredentialHeader)); err != nil {
			http.Error(w, selfAuthRejection, http.StatusUnauthorized)
			return
		}
	}

	// 3. Setup Execution
	execID := generateID()
	logEntry := storage.Execution{
		ID:        execID,
		Status:    "running",
		Input:     req.Input,
		CreatedAt: time.Now(),
	}
	if err := h.Repo.SaveExecution(logEntry); err != nil {
		http.Error(w, "Failed to save execution: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. Execute THROUGH the Self's banks (Synchronous for now).
	// Recall, model access, the episode graph, and memory formation all live
	// on the Self — the handler holds no per-request resources.
	episode, err := h.Self.RunEpisode(r.Context(), agora.EpisodeRequest{
		SelfID: req.SelfID,
		Input:  req.Input,
		Model:  req.Model,
	})
	if err != nil {
		http.Error(w, "Failed to recall self: "+err.Error(), http.StatusInternalServerError)
		return
	}

	status := episode.Status
	output := episode.Output
	if status == "completed" {
		// Save history to DB
		for _, msg := range episode.History {
			_ = h.Repo.AppendMessage(execID, msg.Role, msg.Content)
		}
	}

	// 5. Update Record
	if err := h.Repo.UpdateExecution(execID, status, output); err != nil {
		// Log error but we already processed
		fmt.Printf("Failed to update execution: %v\n", err)
	}

	// 6. Response
	resp := RunResponse{
		ExecutionID: execID,
		Status:      status,
		Output:      output,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *AgentHandler) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	execID := r.URL.Query().Get("execution_id")
	if execID == "" {
		http.Error(w, "Missing execution_id", http.StatusBadRequest)
		return
	}

	history, err := h.Repo.GetHistory(execID)
	if err != nil {
		http.Error(w, "Failed to retrieve history: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

// EstablishSelfRequest asks for a new self to be established.
type EstablishSelfRequest struct {
	SelfID string `json:"self_id"`
}

// EstablishSelfResponse returns the minted credential — the only time it is
// ever transmitted. Losing it means losing access to the self's memory:
// there is no recovery path at dev stage.
type EstablishSelfResponse struct {
	SelfID     string `json:"self_id"`
	Credential string `json:"credential"`
}

// HandleEstablishSelf is the authn-establishing path (SEC gate, advisories
// A and B): the ONLY server path that creates a self. Creation and
// credential minting are one act, so anonymous /run traffic can never mint
// selves rows (growth gate), and every server-established self is born
// guarded. An id that is already taken — or lost to a concurrent
// establisher — is a clean 409, never a 500 (advisory C).
func (h *AgentHandler) HandleEstablishSelf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EstablishSelfRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1048576))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.SelfID) == "" {
		http.Error(w, "Missing self_id", http.StatusBadRequest)
		return
	}

	credential, err := h.Self.EstablishSelf(req.SelfID)
	if err != nil {
		if errors.Is(err, agora.ErrSelfUnavailable) {
			http.Error(w, "Self unavailable", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to establish self", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(EstablishSelfResponse{SelfID: req.SelfID, Credential: credential})
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
