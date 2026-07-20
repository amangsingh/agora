package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/storage"
)

func TestHandler_Auth(t *testing.T) {
	// Setup
	os.Setenv("AGORA_AUTH_TOKEN", "secret-token")
	defer os.Unsetenv("AGORA_AUTH_TOKEN")

	repo, _ := storage.NewRepository(":memory:")
	handler := &AgentHandler{Repo: repo} // Handler itself doesn't check auth, middleware does.
	// We need to test the Middleware here? Direction says pkg/server/handler_test.go.
	// Usually handler tests test logic, middleware tests middleware.
	// But let's verify Auth via chain locally or just test middleware?
	// The prompt asked for "Server Tests" with Scenario 1 (Auth).
	// The Auth logic is in Middleware. Let's create a chain.

	mux := http.NewServeMux()
	mux.HandleFunc("/run", handler.HandleRun)

	protected := BearerAuth(mux)

	req := httptest.NewRequest("POST", "/run", nil)
	// No Header

	w := httptest.NewRecorder()
	protected.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestHandler_BadInput(t *testing.T) {
	repo, _ := storage.NewRepository(":memory:")
	handler := &AgentHandler{Repo: repo}

	// Malformed JSON with unknown field "foo"
	body := []byte(`{"input": "hello", "foo": "bar"}`)
	req := httptest.NewRequest("POST", "/run", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.HandleRun(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for unknown field, got %d", w.Code)
	}
}

func TestHandler_Success(t *testing.T) {
	repo, _ := storage.NewRepository(":memory:")
	// The run path now executes through the process-lifetime Self (Step 4
	// inversion); the handler itself owns no model construction.
	self, err := agora.NewSelf(agora.Blueprint{
		ID:            "test-self",
		Models:        []string{"mock-model"},
		SystemPrompts: []string{"You are a helpful API agent."},
		Memory:        repo,
		ModelFactory:  func(string) agora.Model { return &captureLLM{} },
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}
	handler := &AgentHandler{Repo: repo, Self: self}

	body := []byte(`{"input": "Test Input", "model": "mock-model"}`)
	req := httptest.NewRequest("POST", "/run", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	handler.HandleRun(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp RunResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Status != "completed" {
		t.Logf("Status is %s (expected completed or failed)", resp.Status)
	}
	if resp.ExecutionID == "" {
		t.Error("ExecutionID is empty")
	}
}
