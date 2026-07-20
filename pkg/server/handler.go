package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/llm"
	"github.com/amangsingh/agora/nodes"
	"github.com/amangsingh/agora/pkg/storage"
)

type AgentHandler struct {
	Repo *storage.Repository

	// ModelFactory optionally overrides how the handler constructs its LLM
	// for a run. Nil preserves the default Ollama construction. This is a
	// wiring seam: tests inject a capturing mock here so recall can be
	// asserted on the exact payload the mind sends to its model.
	ModelFactory func(model string) llm.LLM
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

	// 3. Execute Graph (Synchronous for now)
	// In a real system, this might be async with a worker queue.
	// We will construct a simple standard agent here.
	ctx := r.Context()

	// Default to a simple LLM based agent for demonstration/Phase 3
	modelName := "llama3"
	if req.Model != "" {
		modelName = req.Model
	}
	var model llm.LLM
	if h.ModelFactory != nil {
		model = h.ModelFactory(modelName)
	} else {
		model = llm.NewOllamaLLM("http://localhost:11434/v1", modelName)
	}
	agent := nodes.SimpleAgentNode(model, "You are a helpful API agent.")

	g := agora.NewGraph()
	g.MaxSteps = 10
	g.AddNode("agent", agent)
	g.SetEntry("agent")

	// Recall: when the request addresses a self, load that self's stored
	// engrams back INTO the starting state, so this run begins where the
	// self's memory left off instead of from amnesia.
	warmHistory, warmEngrams, err := h.recallSelf(req.SelfID)
	if err != nil {
		http.Error(w, "Failed to recall self: "+err.Error(), http.StatusInternalServerError)
		return
	}

	baseState := agora.NewBaseState()
	if warmEngrams != nil {
		// Carry the full engram data (affective coefficient, relational
		// anchor) into the state alongside the chat-shaped history, so the
		// read path drops nothing the schema preserved.
		baseState.Set("engrams", warmEngrams)
	}
	initialState := &agora.ConversationState{
		BaseState: baseState,
		History:   warmHistory,
		Input:     req.Input,
	}

	finalStateRaw, err := g.Execute(ctx, initialState)

	status := "completed"
	output := ""
	if err != nil {
		status = "failed"
		output = err.Error()
	} else {
		// Extract output
		fs := finalStateRaw.(*agora.ConversationState)
		if len(fs.History) > 0 {
			lastMsg := fs.History[len(fs.History)-1]
			output = lastMsg.Content
			// Save history to DB
			for _, msg := range fs.History {
				_ = h.Repo.AppendMessage(execID, msg.Role, msg.Content)
			}
		}
		// Memory formation: persist only the turns THIS run added (the warm
		// prefix is already in the store) as engrams of the self.
		if req.SelfID != "" && len(fs.History) > len(warmHistory) {
			h.formEngrams(req.SelfID, fs.History[len(warmHistory):])
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

// recallSelf resolves the self addressed by the request and loads its stored
// engrams as a warm chat history. An empty selfID is the legacy amnesiac
// path: no self, no recall, and the returned history/engrams are nil.
// An unknown selfID is registered as a new self (Step 2 identity model) whose
// memory simply starts empty.
func (h *AgentHandler) recallSelf(selfID string) ([]agora.ChatMessage, []storage.Engram, error) {
	if selfID == "" {
		return nil, nil, nil
	}

	if _, err := h.Repo.GetSelf(selfID); err != nil {
		newSelf := storage.Self{ID: selfID, Name: selfID, CreatedAt: time.Now()}
		if err := h.Repo.CreateSelf(newSelf); err != nil {
			return nil, nil, fmt.Errorf("self %q could not be resolved or created: %w", selfID, err)
		}
	}

	engrams, err := h.Repo.GetEngrams(selfID)
	if err != nil {
		return nil, nil, fmt.Errorf("loading engrams for self %q: %w", selfID, err)
	}

	history := make([]agora.ChatMessage, 0, len(engrams))
	for _, e := range engrams {
		// The relational anchor of a conversational engram is the speaker it
		// was formed as (written from msg.Role at formation time), so the
		// role round-trips through the store without loss.
		history = append(history, agora.ChatMessage{Role: e.RelationalAnchor, Content: e.Content})
	}
	return history, engrams, nil
}

// formEngrams persists the new turns of a run as engrams of the self. The
// affective coefficient is neutral at formation (affect inference is a later
// step); the relational anchor records the speaker so recall can rebuild the
// turn faithfully. Failures are logged, not fatal: the response already
// happened, but a memory that failed to form must not fail silently.
func (h *AgentHandler) formEngrams(selfID string, turns []agora.ChatMessage) {
	for _, msg := range turns {
		engram := storage.Engram{
			SelfID:               selfID,
			Content:              msg.Content,
			AffectiveCoefficient: 0.0,
			RelationalAnchor:     msg.Role,
			CreatedAt:            time.Now(),
		}
		if err := h.Repo.SaveEngram(engram); err != nil {
			fmt.Printf("Failed to form engram for self %s: %v\n", selfID, err)
		}
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
