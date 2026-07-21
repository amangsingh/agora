package agora_test

// Step 7 tests: V emerges — Loop B closes across the shared D, by
// composition alone. There is deliberately NO new component under test:
// the subject is the ASSERTION that Step 5 (the Hum), Step 6 (the egress
// bank), and O (an episode over the Self's state) compose.
// agora.JudgmentIteration is an iteration BODY — the Step 5 seam, the same
// class of thing as PulseIteration and HeartbeatIteration — not a module.
//
// AC1 is the soul crossing made falsifiable: with ZERO inbound traffic —
// no HTTP call, no externally invoked episode anywhere in the test — a
// seeded condition makes the judgment act, and BOTH halves are asserted in
// ONE test: the outbound verdict reaches the fake sink, AND that same
// verdict lands in D as an engram of the self with its soul fields
// (affective_coefficient, relational_anchor) intact — formed through the
// exact write path Loop A uses (RunEpisode → formEngrams), no parallel
// path. RED on the pre-change tree: agora.JudgmentIteration does not
// exist there, so this file does not compile (the same red shape Steps 5
// and 6 carried).
//
// AC4 is the negative control: absent the seeded condition, N iterations
// pulse and say NOTHING — V is judgment-gated reach, not a firehose.

import (
	"strings"
	"testing"

	"github.com/amangsingh/agora"
)

// judgmentKey is the watched working-state key: the seeded, deterministic
// condition the judgment ought to act on (spec A2 — a threshold on working
// state; sophistication is explicitly not the goal).
const judgmentKey = "care.pressure"

// newJudgingSelf constructs a Self over a fresh in-memory D bank, a single
// scripted L-bank member, and the given fake sink — everything Loop B needs,
// with no server anywhere in sight.
func newJudgingSelf(t *testing.T, sink *fakeSink) (*agora.Self, *memoryBank, *scriptedModel) {
	t.Helper()
	bank := newMemoryBank()
	model := &scriptedModel{}
	self, err := agora.NewSelf(agora.Blueprint{
		ID:           "judging-self",
		Name:         "Judging Self",
		Models:       []string{"mock"},
		Memory:       bank,
		ModelFactory: func(string) agora.Model { return model },
		Egress:       []agora.EgressWriter{sink},
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}
	return self, bank, model
}

// TestLoopB_ClosesAcrossSharedD (AC1): the soul crossing. One test, both
// assertions: (a) the self-initiated reach lands at the world, and (b) the
// engram of that reach is formed into D — soul fields intact — through the
// same path Loop A writes.
func TestLoopB_ClosesAcrossSharedD(t *testing.T) {
	sink := newFakeSink("world")
	self, bank, model := newJudgingSelf(t, sink)

	hum, err := self.StartHum(agora.HumConfig{
		Interval: testInterval,
		Body:     agora.JudgmentIteration(judgmentKey, 1, "world", "the-world"),
	})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	// Seed the condition the judgment OUGHT to act on. This is the
	// outside-observer working-state API — a RAM write, not inbound
	// traffic: no HTTP call and no externally invoked episode exists
	// anywhere in this test.
	hum.SetWorking(judgmentKey, 3)

	msgs := awaitSinkDelivery(t, sink, 1)
	hum.Stop() // settle the loop before inspecting D and the L bank

	// (a) Loop B exists: the self-initiated reach landed at the world.
	got := msgs[0]
	if got.SelfID != "judging-self" {
		t.Errorf("AC1 FAIL: outbound message carries self id %q, want %q — the reach must be performed AS the self",
			got.SelfID, "judging-self")
	}
	if got.Target != "the-world" {
		t.Errorf("AC1 FAIL: outbound message addressed %q, want %q", got.Target, "the-world")
	}
	if got.Content != "scripted-reply-1" {
		t.Errorf("AC1 FAIL: outbound content %q is not the judgment episode's verdict %q — the reach did not come from the O-episode",
			got.Content, "scripted-reply-1")
	}

	// The O-episode was an episode over the Self's STATE: the seeded
	// condition is what the L bank was asked to judge.
	if len(model.requests) != 1 {
		t.Fatalf("AC1 FAIL: expected exactly 1 judgment episode through the L bank, got %d invocations", len(model.requests))
	}
	sawCondition := false
	for _, m := range model.requests[0].Messages {
		if strings.Contains(m.Content, judgmentKey) {
			sawCondition = true
		}
	}
	if !sawCondition {
		t.Errorf("AC1 FAIL: the judgment episode's payload never mentions the watched state %q — the judgment was not over the Self's state", judgmentKey)
	}

	// (b) the crossing is D: the verdict landed as an engram of this self,
	// soul fields present, through the same formation path Loop A uses.
	engrams := bank.engrams["judging-self"]
	if len(engrams) == 0 {
		t.Fatal("AC1 FAIL: the judgment episode formed no engrams — Loop B never crossed D")
	}
	var verdictEngram *agora.Engram
	for i := range engrams {
		e := engrams[i]
		if strings.TrimSpace(e.RelationalAnchor) == "" {
			t.Errorf("AC1 FAIL: engram %q crossed D with a blank relational anchor — the soul schema broke on the Loop B path", e.Content)
		}
		if e.AffectiveCoefficient < -1.0 || e.AffectiveCoefficient > 1.0 {
			t.Errorf("AC1 FAIL: engram %q crossed D with affective coefficient %f out of range", e.Content, e.AffectiveCoefficient)
		}
		if e.Content == got.Content {
			verdictEngram = &engrams[i]
		}
	}
	if verdictEngram == nil {
		t.Fatalf("AC1 FAIL: no engram carries the delivered verdict %q — the reach went out without crossing D", got.Content)
	}
	if verdictEngram.RelationalAnchor != "assistant" {
		t.Errorf("AC1 FAIL: the verdict engram's relational anchor is %q, want %q — the speaker was lost in the crossing",
			verdictEngram.RelationalAnchor, "assistant")
	}
	if verdictEngram.AffectiveCoefficient != 0.0 {
		t.Errorf("AC1 FAIL: the verdict engram's affective coefficient is %f, want the formation-neutral 0.0 (spec A3: presence and faithful carry, not rich inference)",
			verdictEngram.AffectiveCoefficient)
	}
}

// TestLoopB_JudgmentGateNegativeControl (AC4): absent the seeded condition,
// N iterations produce no egress message, no judgment episode, and no
// engram — V is judgment-gated reach, not a firehose.
func TestLoopB_JudgmentGateNegativeControl(t *testing.T) {
	sink := newFakeSink("world")
	self, bank, model := newJudgingSelf(t, sink)

	hum, err := self.StartHum(agora.HumConfig{
		Interval: testInterval,
		Body:     agora.JudgmentIteration(judgmentKey, 1, "world", "the-world"),
	})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	// NO condition is seeded. Let a healthy number of iterations pass.
	awaitIterations(t, hum, 0, 8)
	hum.Stop()

	if msgs := sink.Messages(); len(msgs) != 0 {
		t.Errorf("AC4 FAIL: %d outbound messages arrived with no seeded condition — V is a firehose, not judgment-gated reach: %+v", len(msgs), msgs)
	}
	if n := self.EgressFailures(); n != 0 {
		t.Errorf("AC4 FAIL: %d egress failures counted with no seeded condition — something attempted to reach", n)
	}
	if len(model.requests) != 0 {
		t.Errorf("AC4 FAIL: the L bank was invoked %d times with nothing to judge — the judgment is not gated", len(model.requests))
	}
	if formed := bank.engrams["judging-self"]; len(formed) != 0 {
		t.Errorf("AC4 FAIL: %d engrams formed with no judgment episode — something writes D outside the episode path", len(formed))
	}
}
