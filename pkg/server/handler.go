package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/storage"
)

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

	// 2. Setup Execution
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

	// 3. Execute THROUGH the Self's banks (Synchronous for now).
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

	// 4. Update Record
	if err := h.Repo.UpdateExecution(execID, status, output); err != nil {
		// Log error but we already processed
		fmt.Printf("Failed to update execution: %v\n", err)
	}

	// 5. Response
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

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
