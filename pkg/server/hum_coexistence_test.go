package server

// Step 5 (AC6): Loop A and the Hum share the Self and its D bank — the
// crossing the ratification calls the soul. These tests prove the two loops
// coexist: request-driven episodes and Hum iterations interleave without
// corrupting the transcript or the D write path.
//
// Note the harness itself (newRecallHarness) starts the Hum, so the entire
// Step 3 recall suite in recall_test.go already runs WITH the Hum humming;
// this file adds the explicit interleaving assertions on top.

import (
	"strings"
	"testing"
	"time"
)

// TestHum_CoexistsWithRequestEpisodes (AC6): drive the Step 3 recall flow
// while the Hum iterates underneath, then check both sides of the crossing:
// the request loop's transcript is whole, and D holds exactly the episode
// turns — no Hum residue, no corruption.
func TestHum_CoexistsWithRequestEpisodes(t *testing.T) {
	handler, mock, repo := newRecallHarness(t)

	const firstInput = "coexistence-fact: the Hum is running under this request"
	postRun(t, handler, "self-coexist", firstInput)

	// Let the Hum iterate between the two request-driven episodes.
	time.Sleep(20 * time.Millisecond)

	postRun(t, handler, "self-coexist", "what do you remember?")

	if len(mock.Requests) != 2 {
		t.Fatalf("expected 2 LLM invocations, got %d", len(mock.Requests))
	}
	second := mock.Requests[1]
	if !payloadContains(second, firstInput) {
		t.Errorf("AC6 FAIL: with the Hum running, the second request lost the first request's content — the Hum corrupted recall.\nSecond payload:\n%s",
			payloadDump(second))
	}
	if !payloadContains(second, "scripted-reply-1") {
		t.Errorf("AC6 FAIL: with the Hum running, the first episode's assistant turn is missing from the transcript.\nSecond payload:\n%s",
			payloadDump(second))
	}

	// The D write path: exactly the four episode turns (2x user, 2x
	// assistant), every one with its soul fields intact, and nothing from
	// the Hum — its working state is in-RAM by design and must never leak
	// into D.
	engrams, err := repo.GetEngrams("self-coexist")
	if err != nil {
		t.Fatalf("GetEngrams failed: %v", err)
	}
	if len(engrams) != 4 {
		t.Errorf("AC6 FAIL: expected exactly 4 engrams (2 turns x 2 episodes), got %d — the D write path is corrupted or the Hum wrote into it", len(engrams))
	}
	for _, e := range engrams {
		if strings.TrimSpace(e.RelationalAnchor) == "" {
			t.Errorf("AC6 FAIL: engram %q lost its relational anchor with the Hum running", e.Content)
		}
		if strings.Contains(e.Content, "hum.") || strings.Contains(e.Content, "pulse") {
			t.Errorf("AC6 FAIL: Hum working state leaked into the D bank: %q", e.Content)
		}
	}
}

// TestHum_AdvancesWhileServerIdle (AC2, server-side view): between requests
// the process is not idle — the Self's Hum advances with no inbound traffic.
func TestHum_AdvancesWhileServerIdle(t *testing.T) {
	handler, _, _ := newRecallHarness(t)

	hum := handler.Self.CurrentHum()
	if hum == nil {
		t.Fatalf("harness Self is not humming")
	}
	before := hum.Iterations()
	time.Sleep(20 * time.Millisecond) // zero requests in this window
	if hum.Iterations() <= before {
		t.Errorf("AC2 FAIL (server view): with zero inbound requests the Hum did not advance (still %d iterations)", before)
	}
}
