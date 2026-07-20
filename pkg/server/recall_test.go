package server

// Recall tests: the first tests in this repository's history that prove
// stored memory is loaded back INTO a running mind (Step 3, Loop A warm).
// They assert on the mock LLM's captured request payload — the exact list of
// messages the mind saw — via the real HTTP path.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/storage"
)

// captureLLM records every request payload it is invoked with and returns a
// scripted assistant reply, so tests can assert on exactly what the mind saw.
type captureLLM struct {
	Requests []agora.ModelRequest
	Replies  []string
}

func (c *captureLLM) Invoke(_ context.Context, req agora.ModelRequest) (agora.ModelResponse, error) {
	c.Requests = append(c.Requests, req)
	reply := fmt.Sprintf("scripted-reply-%d", len(c.Requests))
	if len(c.Replies) >= len(c.Requests) {
		reply = c.Replies[len(c.Requests)-1]
	}
	return agora.ModelResponse{
		Choices: []agora.Choice{{
			FinishReason: "stop",
			Message:      agora.ChatMessage{Role: "assistant", Content: reply},
		}},
	}, nil
}

func newRecallHarness(t *testing.T) (*AgentHandler, *captureLLM, *storage.Repository) {
	t.Helper()
	// File-backed DB: recall must survive across requests through the store,
	// and a file DB keeps every pooled connection on the same database.
	repo, err := storage.NewRepository(t.TempDir() + "/recall.db")
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	mock := &captureLLM{}
	// The injection seam moved with the ownership inversion (Step 4): the
	// capturing mock is now planted in the Self's blueprint — the L bank is
	// built ONCE at construction — instead of on the handler per request.
	self, err := agora.NewSelf(agora.Blueprint{
		ID:            "server-self",
		Name:          "server-self",
		Models:        []string{"mock"},
		SystemPrompts: []string{"You are a helpful API agent."},
		Memory:        repo,
		ModelFactory:  func(string) agora.Model { return mock },
	})
	if err != nil {
		t.Fatalf("failed to construct Self: %v", err)
	}

	// Step 5 (AC6, coexistence): the server's Self HUMS while the whole
	// recall suite runs. Every test built on this harness now exercises
	// request-driven episodes interleaved with live Hum iterations — the
	// two loops sharing the one Self and its D bank.
	hum, err := self.StartHum(agora.HumConfig{Interval: 2 * time.Millisecond})
	if err != nil {
		t.Fatalf("failed to start Hum: %v", err)
	}
	t.Cleanup(hum.Stop)

	handler := &AgentHandler{Repo: repo, Self: self}
	return handler, mock, repo
}

// testCredentials caches per-handler credentials minted through the SEC
// gate's establish path, so the whole recall suite authenticates (AC7)
// without every call site carrying credential plumbing.
var (
	testCredentialsMu sync.Mutex
	testCredentials   = map[*AgentHandler]map[string]string{}
)

// credentialFor establishes selfID through the Self's establish mechanism on
// first contact and returns the cached credential thereafter.
func credentialFor(t *testing.T, h *AgentHandler, selfID string) string {
	t.Helper()
	testCredentialsMu.Lock()
	defer testCredentialsMu.Unlock()
	creds := testCredentials[h]
	if creds == nil {
		creds = map[string]string{}
		testCredentials[h] = creds
	}
	if cred, ok := creds[selfID]; ok {
		return cred
	}
	cred, err := h.Self.EstablishSelf(selfID)
	if err != nil {
		t.Fatalf("failed to establish self %q: %v", selfID, err)
	}
	creds[selfID] = cred
	return cred
}

func postRun(t *testing.T, h *AgentHandler, selfID, input string) {
	t.Helper()
	body, _ := json.Marshal(RunRequest{Input: input, Model: "mock", SelfID: selfID})
	req := httptest.NewRequest("POST", "/run", bytes.NewBuffer(body))
	if selfID != "" {
		// Authenticate as the addressed self (SEC gate, advisory A): the
		// suite proves recall THROUGH the authn boundary, not around it.
		req.Header.Set(SelfCredentialHeader, credentialFor(t, h, selfID))
	}
	w := httptest.NewRecorder()
	h.HandleRun(w, req)
	if w.Code != 200 {
		t.Fatalf("POST /run (self=%q input=%q): expected 200, got %d. Body: %s",
			selfID, input, w.Code, w.Body.String())
	}
}

func payloadContains(req agora.ModelRequest, needle string) bool {
	for _, m := range req.Messages {
		if strings.Contains(m.Content, needle) {
			return true
		}
	}
	return false
}

func payloadDump(req agora.ModelRequest) string {
	var b strings.Builder
	for i, m := range req.Messages {
		fmt.Fprintf(&b, "  [%d] %s: %q\n", i, m.Role, m.Content)
	}
	return b.String()
}

// TestRecall_SameSelf (AC1): POST /run twice against the SAME self. The
// second call's model payload must contain content from the first call —
// proof that a second request knows about the first.
func TestRecall_SameSelf(t *testing.T) {
	handler, mock, _ := newRecallHarness(t)

	const firstInput = "my name is Meridian and I keep bees"
	postRun(t, handler, "self-aurora", firstInput)
	postRun(t, handler, "self-aurora", "what is my name?")

	if len(mock.Requests) != 2 {
		t.Fatalf("expected 2 LLM invocations, got %d", len(mock.Requests))
	}
	second := mock.Requests[1]

	if !payloadContains(second, firstInput) {
		t.Errorf("AC1 FAIL: second call's payload does not contain the first call's input %q — the mind is amnesiac.\nSecond payload:\n%s",
			firstInput, payloadDump(second))
	}
	if !payloadContains(second, "scripted-reply-1") {
		t.Errorf("AC1 FAIL: second call's payload does not contain the first call's assistant reply %q.\nSecond payload:\n%s",
			"scripted-reply-1", payloadDump(second))
	}
}

// TestRecall_Isolation (AC2): runs against DIFFERENT selves must not see each
// other's history — while each self still recalls its OWN. The positive half
// makes this test red on the pre-Step-3 tree alongside AC1 (per AC3): without
// recall it fails; with leaky recall it also fails.
func TestRecall_Isolation(t *testing.T) {
	handler, mock, _ := newRecallHarness(t)

	const secretInput = "self-one-secret: the vault code is 4417"
	const ownFact = "self-two-fact: I collect meteorites"
	postRun(t, handler, "self-one", secretInput)
	postRun(t, handler, "self-two", ownFact)
	postRun(t, handler, "self-two", "hello, who am I?")

	if len(mock.Requests) != 3 {
		t.Fatalf("expected 3 LLM invocations, got %d", len(mock.Requests))
	}
	third := mock.Requests[2]

	if !payloadContains(third, ownFact) {
		t.Errorf("AC2 FAIL: self-two's second run does not recall self-two's own history.\nThird payload:\n%s",
			payloadDump(third))
	}
	if payloadContains(third, "self-one-secret") || payloadContains(third, "4417") {
		t.Errorf("AC2 FAIL: self-two's payload leaked self-one's history.\nThird payload:\n%s",
			payloadDump(third))
	}
	if payloadContains(third, "scripted-reply-1") {
		t.Errorf("AC2 FAIL: self-two's payload leaked self-one's assistant reply.\nThird payload:\n%s",
			payloadDump(third))
	}
}

// TestRecall_ReadsSelfKeyedStore (AC4): recall reads through the Step 2
// self-keyed engram store, not the legacy execution_id message log. An engram
// written directly to the store surfaces in the payload with its relational
// anchor carried into the message role (no lossy drop on the read path); a
// row written to the legacy messages table does not surface.
func TestRecall_ReadsSelfKeyedStore(t *testing.T) {
	handler, mock, repo := newRecallHarness(t)

	const selfID = "self-keyed"
	// Establish (rather than raw-create) the self: under the SEC gate a
	// server-addressable self is born credentialed, and this test's run
	// authenticates as it.
	credentialFor(t, handler, selfID)
	if err := repo.SaveEngram(storage.Engram{
		SelfID:               selfID,
		Content:              "engram-only-fact: the lighthouse is painted red",
		AffectiveCoefficient: 0.75,
		RelationalAnchor:     "user",
		CreatedAt:            time.Now(),
	}); err != nil {
		t.Fatalf("failed to seed engram: %v", err)
	}

	// Legacy run-log row keyed on an execution — must NOT be a recall source.
	if err := repo.SaveExecution(storage.Execution{ID: "exec-legacy", Status: "completed", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("failed to save execution: %v", err)
	}
	if err := repo.AppendMessage("exec-legacy", "user", "legacy-log-fact: the lighthouse is painted green"); err != nil {
		t.Fatalf("failed to append legacy message: %v", err)
	}

	postRun(t, handler, selfID, "what colour is the lighthouse?")

	if len(mock.Requests) != 1 {
		t.Fatalf("expected 1 LLM invocation, got %d", len(mock.Requests))
	}
	payload := mock.Requests[0]

	if !payloadContains(payload, "engram-only-fact") {
		t.Errorf("AC4 FAIL: engram stored via the self-keyed store did not surface in the payload — recall is not reading D.\nPayload:\n%s",
			payloadDump(payload))
	}
	for _, m := range payload.Messages {
		if strings.Contains(m.Content, "engram-only-fact") && m.Role != "user" {
			t.Errorf("AC4 FAIL: recalled engram lost its relational anchor on the read path: role=%q, want %q", m.Role, "user")
		}
	}
	if payloadContains(payload, "legacy-log-fact") {
		t.Errorf("AC4 FAIL: legacy execution-keyed message leaked into recall — read path must be the self-keyed store only.\nPayload:\n%s",
			payloadDump(payload))
	}
}

// TestRecall_PersistsEngramsFaithfully (AC4, write half): the turns a run
// persists land in the engram store with the soul fields present — a non-null
// in-range affective coefficient and a non-blank relational anchor per turn.
func TestRecall_PersistsEngramsFaithfully(t *testing.T) {
	handler, _, repo := newRecallHarness(t)

	postRun(t, handler, "self-faithful", "remember the tide tables")

	engrams, err := repo.GetEngrams("self-faithful")
	if err != nil {
		t.Fatalf("GetEngrams failed: %v", err)
	}
	if len(engrams) == 0 {
		t.Fatal("AC4 FAIL: run persisted no engrams for the self — memory was never formed")
	}
	for _, e := range engrams {
		if strings.TrimSpace(e.RelationalAnchor) == "" {
			t.Errorf("engram %d has a blank relational anchor", e.ID)
		}
		if e.AffectiveCoefficient < -1.0 || e.AffectiveCoefficient > 1.0 {
			t.Errorf("engram %d affective coefficient %f out of range", e.ID, e.AffectiveCoefficient)
		}
	}
}
