package server

// SEC gate tests (advisories A and C at the membrane): the authn boundary on
// self resolution and the first-contact behavior of the /run path.
//
// Red-first discipline: every test in this file compiles against the
// pre-change tree (90bce99) and the A-advisory tests FAIL there — today any
// caller presenting a self_id gets that self's full memory, and novel
// self_ids mint selves rows on contact. The credential travels in an HTTP
// header precisely so the pre-change server IGNORES it (a body field would
// bounce off DisallowUnknownFields with a 400 and fake a rejection).

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amangsingh/agora/pkg/storage"
)

// selfCredentialHeader aliases the server's header constant. The red
// capture on the pre-change tree carried this name as a local literal
// (the constant did not exist there); it now binds to the real one.
const selfCredentialHeader = SelfCredentialHeader

// postRunRaw drives POST /run without failing the test on a non-200: the
// gate tests assert on rejection statuses and exact body bytes.
func postRunRaw(t *testing.T, h *AgentHandler, selfID, credential, input string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(RunRequest{Input: input, Model: "mock", SelfID: selfID})
	req := httptest.NewRequest("POST", "/run", bytes.NewBuffer(body))
	if credential != "" {
		req.Header.Set(selfCredentialHeader, credential)
	}
	w := httptest.NewRecorder()
	h.HandleRun(w, req)
	return w.Code, w.Body.String()
}

// TestAuthn_ExistingSelfNoCredentialRejected (AC1): a request addressing an
// EXISTING self with no credential — and with a wrong credential — is
// rejected and returns none of that self's memory, and none of it ever
// reaches the model payload.
func TestAuthn_ExistingSelfNoCredentialRejected(t *testing.T) {
	handler, mock, repo := newRecallHarness(t)

	const selfID = "self-guarded"
	const secret = "guarded-secret: the reactor code is 7731"
	if err := repo.CreateSelf(storage.Self{ID: selfID, Name: selfID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("failed to seed self: %v", err)
	}
	if err := repo.SaveEngram(storage.Engram{
		SelfID: selfID, Content: secret,
		AffectiveCoefficient: 0.5, RelationalAnchor: "user", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("failed to seed engram: %v", err)
	}

	for _, tc := range []struct {
		name, credential string
	}{
		{"no credential", ""},
		{"wrong credential", "not-the-credential-of-self-guarded"},
	} {
		code, body := postRunRaw(t, handler, selfID, tc.credential, "what do you remember?")
		if code == 200 {
			t.Errorf("AC1 FAIL (%s): request addressing existing self %q was accepted (200); an unauthenticated caller reached the self's memory", tc.name, selfID)
		}
		if strings.Contains(body, "7731") || strings.Contains(body, "guarded-secret") {
			t.Errorf("AC1 FAIL (%s): rejection response leaked the self's memory. Body: %s", tc.name, body)
		}
	}
	for i, req := range mock.Requests {
		if payloadContains(req, "guarded-secret") || payloadContains(req, "7731") {
			t.Errorf("AC1 FAIL: model payload %d carried the guarded self's memory for an unauthenticated request:\n%s", i, payloadDump(req))
		}
	}
}

// TestAuthn_NoExistenceOracle (AC2): rejection for a wrong credential on an
// EXISTING self must be indistinguishable — status AND body — from a request
// addressing a self that does not exist. A caller probing ids learns nothing.
func TestAuthn_NoExistenceOracle(t *testing.T) {
	handler, _, repo := newRecallHarness(t)

	const present = "self-present"
	if err := repo.CreateSelf(storage.Self{ID: present, Name: present, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("failed to seed self: %v", err)
	}

	codeA, bodyA := postRunRaw(t, handler, present, "wrong-credential", "hello")
	codeB, bodyB := postRunRaw(t, handler, "self-shadow-never-created", "wrong-credential", "hello")

	if codeA == 200 {
		t.Errorf("AC2 FAIL: wrong credential on existing self was accepted (200)")
	}
	if codeB == 200 {
		t.Errorf("AC2 FAIL: request addressing unknown self was accepted (200)")
	}
	if codeA != codeB {
		t.Errorf("AC2 FAIL: existence oracle via status: existing-self=%d unknown-self=%d", codeA, codeB)
	}
	if bodyA != bodyB {
		t.Errorf("AC2 FAIL: existence oracle via body shape:\n existing-self: %q\n unknown-self:  %q", bodyA, bodyB)
	}
}

// TestAuthn_AnonymousStreamMintsNoSelves (AC4): a stream of anonymous
// requests with novel random self_ids must not mint a selves row per
// request — creation is gated to the credential-establishing path.
func TestAuthn_AnonymousStreamMintsNoSelves(t *testing.T) {
	handler, _, repo := newRecallHarness(t)

	const n = 25
	ids := make([]string, n)
	for i := range ids {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			t.Fatalf("rand: %v", err)
		}
		ids[i] = "drive-by-" + hex.EncodeToString(b)
		postRunRaw(t, handler, ids[i], "", fmt.Sprintf("anonymous request %d", i))
	}

	minted := 0
	for _, id := range ids {
		if _, err := repo.GetSelf(id); err == nil {
			minted++
		}
	}
	if minted != 0 {
		t.Errorf("AC4 FAIL: %d/%d anonymous novel self_ids minted selves rows; creation must be gated to the establishing path", minted, n)
	}
}

// TestAuthn_ConcurrentFirstContactNo500 (AC5, membrane half): concurrent
// first-contact requests for the same new self_id must never 500, whatever
// else the policy decides about them. The storage half of AC5 (the actual
// get-then-create race, which Step 5's episode serialization masks at the
// single-Self HTTP layer) lives in pkg/storage/first_contact_race_test.go.
func TestAuthn_ConcurrentFirstContactNo500(t *testing.T) {
	handler, _, _ := newRecallHarness(t)

	const workers = 8
	start := make(chan struct{})
	codes := make([]int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i], _ = postRunRaw(t, handler, "self-first-contact", "", fmt.Sprintf("contact %d", i))
		}(i)
	}
	close(start)
	wg.Wait()

	for i, code := range codes {
		if code == 500 {
			t.Errorf("AC5 FAIL: concurrent first-contact request %d returned 500", i)
		}
	}
}
