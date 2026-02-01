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
}

type RunRequest struct {
	Input string `json:"input"`
	Model string `json:"model"` // Optional, default to internal config
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
	model := llm.NewOllamaLLM("http://localhost:11434/v1", modelName)
	agent := nodes.SimpleAgentNode(model, "You are a helpful API agent.")

	g := agora.NewGraph()
	g.MaxSteps = 10
	g.AddNode("agent", agent)
	g.SetEntry("agent")

	initialState := &agora.ConversationState{
		BaseState: agora.NewBaseState(),
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
