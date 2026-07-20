package server

// AC2 (persistence of the owner): the SAME Self instance serves two
// sequential requests. Identity is asserted two ways: pointer identity of the
// Self the handler serves through, and the Self's own episode count in its M
// bank (working state), which can only reach 2 if both requests executed
// through that one instance.

import (
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/storage"
)

func TestSelf_SameInstanceServesSequentialRequests(t *testing.T) {
	repo, err := storage.NewRepository(t.TempDir() + "/same.db")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	mock := &captureLLM{}
	self, err := agora.NewSelf(agora.Blueprint{
		ID:            "server-self",
		Models:        []string{"mock"},
		SystemPrompts: []string{"You are a helpful API agent."},
		Memory:        repo,
		ModelFactory:  func(string) agora.Model { return mock },
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}

	handler := &AgentHandler{Repo: repo, Self: self}

	servedFirst := handler.Self
	postRun(t, handler, "self-persist", "first request")
	servedSecond := handler.Self
	postRun(t, handler, "self-persist", "second request")
	servedThird := handler.Self

	if servedFirst != servedSecond || servedSecond != servedThird {
		t.Fatalf("AC2 FAIL: the handler swapped Self instances between requests (%p, %p, %p)", servedFirst, servedSecond, servedThird)
	}
	if servedFirst != self {
		t.Fatalf("AC2 FAIL: the handler served through a different Self than the one constructed at process start")
	}
	if got := self.Episodes(); got != 2 {
		t.Fatalf("AC2 FAIL: the process-start Self recorded %d episodes across 2 requests (want 2) — requests are not executing through the persistent owner", got)
	}
}
