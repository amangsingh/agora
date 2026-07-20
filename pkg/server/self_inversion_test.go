package server

// Step 4 inversion tests: the ownership flip from "resources conjured
// per-request inside a handler" to "one process-lifetime agora.Self that
// owns the banks".
//
// AC1 (once-per-process): a counting constructor proves the LLM client is
// built ONCE per process, not once per request. Verified red on the
// pre-change tree (HEAD 9c291b4, handler constructing inside HandleRun):
//   AC1 FAIL: LLM client constructed 2 times across 2 sequential requests

import (
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/storage"
)

// TestSelf_LLMConstructedOncePerProcess (AC1): drive two sequential requests
// through the server path with a counting LLM constructor; the client must be
// constructed exactly once for the process — at Self construction — not once
// per request.
func TestSelf_LLMConstructedOncePerProcess(t *testing.T) {
	repo, err := storage.NewRepository(t.TempDir() + "/once.db")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	constructions := 0
	mock := &captureLLM{}
	self, err := agora.NewSelf(agora.Blueprint{
		ID:            "once-self",
		Models:        []string{"mock"},
		SystemPrompts: []string{"You are a helpful API agent."},
		Memory:        repo,
		ModelFactory: func(string) agora.Model {
			constructions++
			return mock
		},
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}

	handler := &AgentHandler{Repo: repo, Self: self}

	postRun(t, handler, "self-once", "first request")
	postRun(t, handler, "self-once", "second request")

	if len(mock.Requests) != 2 {
		t.Fatalf("expected 2 LLM invocations across 2 requests, got %d", len(mock.Requests))
	}
	if constructions != 1 {
		t.Fatalf("AC1 FAIL: LLM client constructed %d times across 2 sequential requests; the Self must build its L bank ONCE per process (want 1)", constructions)
	}
}
