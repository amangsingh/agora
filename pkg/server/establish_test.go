package server

// SEC gate tests for the authn-establishing path: POST /selves is the only
// server path that mints a self, creation and credential are one act, and
// first contact is race-safe end to end (advisories A/B/C at the membrane).

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
)

func postEstablish(t *testing.T, h *AgentHandler, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/selves", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.HandleEstablishSelf(w, req)
	return w.Code, w.Body.String()
}

// TestEstablish_MintsGuardedSelf: establishment returns the credential once,
// and the credential is exactly what /run then authenticates with.
func TestEstablish_MintsGuardedSelf(t *testing.T) {
	handler, mock, _ := newRecallHarness(t)

	code, body := postEstablish(t, handler, `{"self_id":"self-established"}`)
	if code != 201 {
		t.Fatalf("establish: expected 201, got %d. Body: %s", code, body)
	}
	var resp EstablishSelfResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("establish response is not the documented shape: %v", err)
	}
	if resp.SelfID != "self-established" || resp.Credential == "" {
		t.Fatalf("establish response incomplete: %+v", resp)
	}

	// The minted credential authenticates a run...
	runCode, _ := postRunRaw(t, handler, "self-established", resp.Credential, "first authenticated contact")
	if runCode != 200 {
		t.Errorf("minted credential rejected on /run: got %d", runCode)
	}
	// ...and its absence still rejects (the establish path grants nothing
	// to credential-less callers).
	runCode, _ = postRunRaw(t, handler, "self-established", "", "anonymous follow-up")
	if runCode != 401 {
		t.Errorf("anonymous run against established self: got %d, want 401", runCode)
	}
	if len(mock.Requests) != 1 {
		t.Errorf("expected exactly the authenticated run to reach the model, got %d invocations", len(mock.Requests))
	}
}

// TestEstablish_TakenIdRejectsCleanly: a taken id is a clean 409 that never
// re-issues or leaks the standing credential.
func TestEstablish_TakenIdRejectsCleanly(t *testing.T) {
	handler, _, _ := newRecallHarness(t)

	code, body := postEstablish(t, handler, `{"self_id":"self-taken"}`)
	if code != 201 {
		t.Fatalf("first establish: expected 201, got %d", code)
	}
	var first EstablishSelfResponse
	if err := json.Unmarshal([]byte(body), &first); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	code, body = postEstablish(t, handler, `{"self_id":"self-taken"}`)
	if code != 409 {
		t.Errorf("second establish: expected 409, got %d", code)
	}
	if bytes.Contains([]byte(body), []byte(first.Credential)) {
		t.Errorf("second establish leaked the standing credential")
	}
}

// TestEstablish_ConcurrentFirstContact (AC5 at the establish path): many
// concurrent establishers of the same new id — exactly one wins a
// credential, nobody 500s.
func TestEstablish_ConcurrentFirstContact(t *testing.T) {
	handler, _, _ := newRecallHarness(t)

	const workers = 16
	start := make(chan struct{})
	codes := make([]int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i], _ = postEstablish(t, handler, `{"self_id":"self-contested"}`)
		}(i)
	}
	close(start)
	wg.Wait()

	winners, conflicts := 0, 0
	for i, code := range codes {
		switch code {
		case 201:
			winners++
		case 409:
			conflicts++
		default:
			t.Errorf("AC5 FAIL: concurrent establish %d returned %d (want 201 or 409, never 500)", i, code)
		}
	}
	if winners != 1 {
		t.Errorf("AC5 FAIL: %d establishers won the same new id (want exactly 1; %d conflicts)", winners, conflicts)
	}
}

// TestEstablish_RejectsMalformed: the establish path validates like the rest
// of the membrane — strict JSON, no blank identities.
func TestEstablish_RejectsMalformed(t *testing.T) {
	handler, _, _ := newRecallHarness(t)

	for name, body := range map[string]string{
		"empty self_id":      `{"self_id":""}`,
		"whitespace self_id": `{"self_id":"   "}`,
		"unknown field":      `{"self_id":"x","admin":true}`,
		"not json":           `not-json`,
	} {
		if code, _ := postEstablish(t, handler, body); code != 400 {
			t.Errorf("%s: expected 400, got %d", name, code)
		}
	}
}
