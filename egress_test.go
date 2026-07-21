package agora_test

// Step 6 tests: E becomes bidirectional — egress writers, the mind reaching
// out (behind the SEC gate).
//
// AC4 (framework proof) holds by construction of this file: it lives in the
// ROOT package's test package and imports ONLY the root agora package plus
// stdlib — a Self is constructed with a fake egress bank and delivery is
// observed without pkg/server anywhere in sight.
//
// AC1 is "the one who knocks": with NO inbound request whatsoever — no HTTP
// call, no externally invoked episode — an outbound message arrives at a fake
// egress sink, originated by the running Hum. RED on the pre-change tree
// (5157691): no egress path exists, this file does not compile there (the
// same red shape Step 5's AC1a carried against HEAD 9c291b4).
//
// AC2 is self-bound egress: two Selves with distinct sinks never cross.
// AC5 is containment: a failing writer is counted and the loop keeps going.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amangsingh/agora"
)

// fakeSink is the fake egress sink of the acceptance criteria: an
// EgressWriter that records every message delivered through it.
type fakeSink struct {
	name string

	mu       sync.Mutex
	messages []agora.OutboundMessage
}

func newFakeSink(name string) *fakeSink { return &fakeSink{name: name} }

func (f *fakeSink) Name() string { return f.name }

func (f *fakeSink) Write(_ context.Context, msg agora.OutboundMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, msg)
	return nil
}

// Messages returns a snapshot of everything delivered so far.
func (f *fakeSink) Messages() []agora.OutboundMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agora.OutboundMessage(nil), f.messages...)
}

// failingSink is the AC5 sink: every delivery fails.
type failingSink struct {
	name string
}

func (f *failingSink) Name() string { return f.name }

func (f *failingSink) Write(context.Context, agora.OutboundMessage) error {
	return errors.New("deliberate delivery failure: the wire is down")
}

// newReachingSelf constructs a Self over a fresh in-memory D bank, a scripted
// L bank, and the given egress writers.
func newReachingSelf(t *testing.T, id string, writers ...agora.EgressWriter) *agora.Self {
	t.Helper()
	self, err := agora.NewSelf(agora.Blueprint{
		ID:           id,
		Name:         id,
		Models:       []string{"mock"},
		Memory:       newMemoryBank(),
		ModelFactory: func(string) agora.Model { return &scriptedModel{} },
		Egress:       writers,
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}
	return self
}

// awaitSinkDelivery blocks until the sink has received at least n messages,
// failing the test on deadline.
func awaitSinkDelivery(t *testing.T, sink *fakeSink, n int) []agora.OutboundMessage {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if msgs := sink.Messages(); len(msgs) >= n {
			return msgs
		}
		time.Sleep(testInterval)
	}
	msgs := sink.Messages()
	t.Fatalf("sink %q received only %d outbound messages (wanted >= %d) within deadline — the mind is not reaching out",
		sink.Name(), len(msgs), n)
	return msgs
}

// TestEgress_TheOneWhoKnocks (AC1 + AC4): with no inbound request whatsoever
// after process start — this test makes no HTTP call and invokes no episode
// from outside — an outbound message arrives at a fake egress sink,
// originated by the running Hum. The Hum → egress path is Loop B's terminal
// arc: the iteration body queues the intent, the egress bank delivers it.
func TestEgress_TheOneWhoKnocks(t *testing.T) {
	sink := newFakeSink("fake")
	self := newReachingSelf(t, "knocking-self", sink)

	hum, err := self.StartHum(agora.HumConfig{
		Interval: testInterval,
		// The minimal Step 6 trigger (spec A2): a working-state threshold.
		// Every 2nd pulse queues one outbound heartbeat. Judgment is Step 7's.
		Body: agora.HeartbeatIteration(2, "fake", "test-target"),
	})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	msgs := awaitSinkDelivery(t, sink, 1)

	got := msgs[0]
	if got.SelfID != "knocking-self" {
		t.Errorf("AC1 FAIL: outbound message carries self id %q, want %q — outbound reach must be performed AS the self",
			got.SelfID, "knocking-self")
	}
	if got.Target != "test-target" {
		t.Errorf("AC1 FAIL: outbound message addressed %q, want %q", got.Target, "test-target")
	}
	if got.Content == "" {
		t.Error("AC1 FAIL: outbound message has no content")
	}
	if got.SentAt.IsZero() {
		t.Error("AC1 FAIL: outbound message carries no SentAt stamp")
	}
}

// TestEgress_SelfBoundWriters (AC2): two Selves with distinct sinks — each
// self's outbound messages arrive only at its own sink. Egress writers are
// bound to the Self whose banks they live in; there is no ambient global
// writer to leak across selves.
func TestEgress_SelfBoundWriters(t *testing.T) {
	sinkA := newFakeSink("sink-a")
	sinkB := newFakeSink("sink-b")
	selfA := newReachingSelf(t, "self-a", sinkA)
	selfB := newReachingSelf(t, "self-b", sinkB)

	humA, err := selfA.StartHum(agora.HumConfig{
		Interval: testInterval,
		Body:     agora.HeartbeatIteration(2, "sink-a", "target-a"),
	})
	if err != nil {
		t.Fatalf("StartHum(self-a) failed: %v", err)
	}
	defer humA.Stop()

	humB, err := selfB.StartHum(agora.HumConfig{
		Interval: testInterval,
		Body:     agora.HeartbeatIteration(2, "sink-b", "target-b"),
	})
	if err != nil {
		t.Fatalf("StartHum(self-b) failed: %v", err)
	}
	defer humB.Stop()

	msgsA := awaitSinkDelivery(t, sinkA, 2)
	msgsB := awaitSinkDelivery(t, sinkB, 2)

	for _, m := range msgsA {
		if m.SelfID != "self-a" {
			t.Errorf("AC2 FAIL: sink of self-a received a message from self %q — cross-self writer leakage", m.SelfID)
		}
	}
	for _, m := range msgsB {
		if m.SelfID != "self-b" {
			t.Errorf("AC2 FAIL: sink of self-b received a message from self %q — cross-self writer leakage", m.SelfID)
		}
	}

	// Direct reach through a writer name that lives in ANOTHER self's bank
	// must refuse: the bank boundary is the self boundary.
	err = selfA.Reach(context.Background(), agora.OutboundMessage{Writer: "sink-b", Target: "target-b", Content: "trespass"})
	if err == nil {
		t.Error("AC2 FAIL: self-a reached through self-b's writer — egress writers must be bound to their own Self")
	}
	for _, m := range sinkB.Messages() {
		if m.SelfID == "self-a" || m.Content == "trespass" {
			t.Errorf("AC2 FAIL: a message from self-a landed in self-b's sink: %+v", m)
		}
	}
}

// TestEgress_FailureContainment (AC5): egress failures do not kill the Hum.
// A deliberately failing sink makes every delivery fail; the failures are
// counted, and the loop keeps advancing across them.
func TestEgress_FailureContainment(t *testing.T) {
	self := newReachingSelf(t, "contained-self", &failingSink{name: "broken"})

	hum, err := self.StartHum(agora.HumConfig{
		Interval: testInterval,
		// Every pulse queues an intent; every delivery fails.
		Body: agora.HeartbeatIteration(1, "broken", "the-void"),
	})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	// Wait until at least one delivery has failed…
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && self.EgressFailures() == 0 {
		time.Sleep(testInterval)
	}
	if self.EgressFailures() == 0 {
		t.Fatal("AC5 FAIL: no egress failure was ever observed — the failing sink was never reached")
	}

	// …then assert the loop advances ACROSS the failing sink: 3+ more
	// iterations after the first observed failure, and more failures counted.
	failuresAtFirst := self.EgressFailures()
	from := hum.Iterations()
	awaitIterations(t, hum, from, 3)

	if hum.Iterations() < from+3 {
		t.Fatalf("AC5 FAIL: the Hum stalled after an egress failure (%d iterations since failure)", hum.Iterations()-from)
	}
	if got := self.EgressFailures(); got < failuresAtFirst {
		t.Errorf("AC5 FAIL: failure counter went backwards (%d -> %d)", failuresAtFirst, got)
	}
}

// TestEgress_ReachStampsIdentity (AC3 discipline at the framework): outbound
// reach is performed AS the self. A caller-supplied SelfID on the message is
// overwritten with the Self's own identity — the egress path cannot be used
// to speak as someone else, so nothing here reintroduces unauthenticated
// self addressing.
func TestEgress_ReachStampsIdentity(t *testing.T) {
	sink := newFakeSink("fake")
	self := newReachingSelf(t, "honest-self", sink)

	err := self.Reach(context.Background(), agora.OutboundMessage{
		SelfID:  "somebody-else", // spoof attempt: must be overwritten
		Target:  "wire",
		Content: "hello out there",
	})
	if err != nil {
		t.Fatalf("Reach failed: %v", err)
	}

	msgs := sink.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 delivery, got %d", len(msgs))
	}
	if msgs[0].SelfID != "honest-self" {
		t.Errorf("AC3 FAIL: delivered message carries self id %q, want %q — the Self must stamp its own identity",
			msgs[0].SelfID, "honest-self")
	}

	// A Self with no egress bank cannot reach at all — the port is declared,
	// never ambient.
	mute := newReachingSelf(t, "mute-self")
	if err := mute.Reach(context.Background(), agora.OutboundMessage{Target: "wire", Content: "x"}); err == nil {
		t.Error("a Self with no declared egress writers reached the world")
	}
}

// TestEgress_DeclarationDefects: E-bank declaration defects fail Self
// construction loudly — never a silent drop (same discipline as the S bank).
func TestEgress_DeclarationDefects(t *testing.T) {
	factory := func(string) agora.Model { return &scriptedModel{} }

	if _, err := agora.NewSelf(agora.Blueprint{
		ID: "x", ModelFactory: factory,
		Egress: []agora.EgressWriter{newFakeSink("")},
	}); err == nil {
		t.Error("an unnamed egress writer was accepted silently")
	}

	if _, err := agora.NewSelf(agora.Blueprint{
		ID: "x", ModelFactory: factory,
		Egress: []agora.EgressWriter{newFakeSink("dup"), newFakeSink("dup")},
	}); err == nil {
		t.Error("duplicate egress writer names were accepted silently")
	}
}

// TestStreamWriter_WritesJSONLines: the first concrete E-bank member — a
// JSON-line writer over an io.Writer. Deliberately modest (spec A3):
// transport hardening beyond the gate's posture is future work.
func TestStreamWriter_WritesJSONLines(t *testing.T) {
	var buf bytes.Buffer
	w := agora.NewStreamWriter("stream", &buf)

	if w.Name() != "stream" {
		t.Errorf("Name() = %q, want %q", w.Name(), "stream")
	}

	msg := agora.OutboundMessage{
		SelfID:  "stream-self",
		Writer:  "stream",
		Target:  "stdout",
		Content: "first light",
		SentAt:  time.Now(),
	}
	if err := w.Write(context.Background(), msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	line := buf.String()
	if !strings.HasSuffix(line, "\n") {
		t.Error("stream writer did not terminate the record with a newline")
	}
	var got agora.OutboundMessage
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("stream writer did not emit valid JSON: %v (line %q)", err, line)
	}
	if got.SelfID != "stream-self" || got.Content != "first light" || got.Target != "stdout" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
