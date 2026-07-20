package server

// SEC gate test (advisory B at the read path): the engram load per run is
// bounded by a recall window. On the pre-change tree GetEngrams loads ALL
// engrams of a self every run (unbounded DB read, unbounded model payload),
// so this test is red there.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amangsingh/agora/pkg/storage"
)

// windowUnderTest mirrors the default recall window the gate introduces.
// The pre-change tree has no window constant, so the value is written out
// here to keep the red capture compilable; once the gate lands this must
// equal agora's declared default.
const windowUnderTest = 50

// TestRecallWindow_BoundsPayload (AC3): seed a self with more engrams than
// the window, then run. The model payload must carry at most the window's
// worth of recalled engrams — the NEWEST ones, in chronological order — and
// the store must not be truncated to achieve it (window at query time).
func TestRecallWindow_BoundsPayload(t *testing.T) {
	handler, mock, repo := newRecallHarness(t)

	const selfID = "self-windowed"
	const seeded = windowUnderTest + 10

	// First contact through the real path (establishes the self under the
	// post-gate flow; mints it via run on the pre-change tree).
	postRun(t, handler, selfID, "hello, I am about to remember a great deal")

	for i := 1; i <= seeded; i++ {
		if err := repo.SaveEngram(storage.Engram{
			SelfID:               selfID,
			Content:              fmt.Sprintf("windowed-fact-%03d", i),
			AffectiveCoefficient: 0.25,
			RelationalAnchor:     "user",
			CreatedAt:            time.Now(),
		}); err != nil {
			t.Fatalf("failed to seed engram %d: %v", i, err)
		}
	}

	postRun(t, handler, selfID, "what do you remember?")

	last := mock.Requests[len(mock.Requests)-1]

	var recalled []string
	for _, m := range last.Messages {
		if strings.HasPrefix(m.Content, "windowed-fact-") {
			recalled = append(recalled, m.Content)
		}
	}

	if len(recalled) > windowUnderTest {
		t.Errorf("AC3 FAIL: payload carries %d seeded engrams, window is %d — recall is unbounded", len(recalled), windowUnderTest)
	}
	if len(recalled) == 0 {
		t.Fatalf("AC3 FAIL: payload carries none of the seeded engrams — recall broke outright.\nPayload:\n%s", payloadDump(last))
	}

	// Newest-first coherence: the window must hold the newest engrams and
	// present them in chronological order (oldest of the window first).
	wantLast := fmt.Sprintf("windowed-fact-%03d", seeded)
	if recalled[len(recalled)-1] != wantLast {
		t.Errorf("AC3 FAIL: window did not keep the newest engrams: last recalled is %q, want %q", recalled[len(recalled)-1], wantLast)
	}
	for i := 1; i < len(recalled); i++ {
		if recalled[i] <= recalled[i-1] {
			t.Errorf("AC3 FAIL: recalled engrams out of chronological order at %d: %q after %q", i, recalled[i], recalled[i-1])
		}
	}

	// Window at query time, never destructive truncation: every seeded
	// engram is still in the store.
	all, err := repo.GetEngrams(selfID)
	if err != nil {
		t.Fatalf("GetEngrams: %v", err)
	}
	stored := 0
	for _, e := range all {
		if strings.HasPrefix(e.Content, "windowed-fact-") {
			stored++
		}
	}
	if stored != seeded {
		t.Errorf("AC3 FAIL: store holds %d of %d seeded engrams — windowing must not delete or compact", stored, seeded)
	}
}
