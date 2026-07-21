package server

// Step 7 (AC2): the crossing is SHARED, not parallel. The engram Loop B
// forms must be readable back through the SAME recall path Loop A uses — a
// subsequent AUTHENTICATED /run for the self sees the Loop B verdict in its
// warm model payload, through the real HTTP path, the authn boundary, and
// the file-backed store. RED on the pre-change tree:
// agora.JudgmentIteration does not exist there, so this file does not
// compile (the chain's red shape for framework-absence).

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/storage"
)

// loopbSink is the server-side fake egress sink: it records every outbound
// message delivered through it.
type loopbSink struct {
	mu       sync.Mutex
	messages []agora.OutboundMessage
}

func (s *loopbSink) Name() string { return "loopb-sink" }

func (s *loopbSink) Write(_ context.Context, msg agora.OutboundMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	return nil
}

func (s *loopbSink) Messages() []agora.OutboundMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]agora.OutboundMessage(nil), s.messages...)
}

// TestLoopB_EngramReadableThroughLoopARecall (AC2): seed the judgment
// condition, let Loop B act with zero inbound traffic, then run a Loop A
// episode through the authenticated /run path for the SAME self. The warm
// payload the model sees must contain the Loop B verdict — proof the Loop B
// engram went into, and came back out of, the one shared D.
func TestLoopB_EngramReadableThroughLoopARecall(t *testing.T) {
	const selfID = "server-self"

	repo, err := storage.NewRepository(t.TempDir() + "/loopb.db")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	mock := &captureLLM{}
	sink := &loopbSink{}
	self, err := agora.NewSelf(agora.Blueprint{
		ID:            selfID,
		Name:          selfID,
		Models:        []string{"mock"},
		SystemPrompts: []string{"You are a helpful API agent."},
		Memory:        repo,
		ModelFactory:  func(string) agora.Model { return mock },
		Egress:        []agora.EgressWriter{sink},
	})
	if err != nil {
		t.Fatalf("failed to construct Self: %v", err)
	}
	handler := &AgentHandler{Repo: repo, Self: self}

	// Establish the self BEFORE anything touches D: under the SEC gate the
	// server-addressable self is born credentialed, and the later /run
	// authenticates as it.
	credentialFor(t, handler, selfID)

	hum, err := self.StartHum(agora.HumConfig{
		Interval: 2 * time.Millisecond,
		Body:     agora.JudgmentIteration("care.pressure", 1, "loopb-sink", "the-world"),
	})
	if err != nil {
		t.Fatalf("failed to start Hum: %v", err)
	}
	defer hum.Stop()

	// Seed the condition; Loop B acts with ZERO inbound traffic (this side
	// of the test makes no HTTP call).
	hum.SetWorking("care.pressure", 3)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(sink.Messages()) == 0 {
		time.Sleep(2 * time.Millisecond)
	}
	msgs := sink.Messages()
	if len(msgs) == 0 {
		t.Fatal("AC2 FAIL: no outbound message ever reached the sink — Loop B never closed, nothing to recall")
	}
	verdict := msgs[0].Content
	if verdict == "" {
		t.Fatal("AC2 FAIL: the delivered verdict is empty")
	}

	// Loop A, through the front door: an authenticated /run for the same
	// self. Its warm payload must carry the Loop B turn — same recall path,
	// same D, no parallel channel.
	postRun(t, handler, selfID, "what did you decide while I was away?")

	if len(mock.Requests) < 2 {
		t.Fatalf("expected >= 2 model invocations (judgment episode + /run episode), got %d", len(mock.Requests))
	}
	runPayload := mock.Requests[len(mock.Requests)-1]
	if !payloadContains(runPayload, verdict) {
		t.Errorf("AC2 FAIL: the authenticated /run's warm payload does not contain the Loop B verdict %q — the crossing is parallel, not shared.\nPayload:\n%s",
			verdict, payloadDump(runPayload))
	}
}
