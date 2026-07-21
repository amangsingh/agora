// in agora/egress.go

package agora

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// This file makes E bidirectional (Step 6): egress writers — the mind
// reaching out. Until this step the entire membrane was inbound; nothing in
// the process ever reached OUT. The E bank added here is the outward half:
// a writer/sink port the Self can address the world through.
//
// It ships BEHIND THE SEC GATE (task n3gs6n6d9anm3raf, advisories A/B/C
// closed): egress is the moment R touches the world, and the gate's identity
// discipline binds here — outbound reach is performed AS a self. Egress
// writers are bound to the Self whose banks they live in; there is no
// ambient global writer, and the delivered message's SelfID is stamped by
// the Self itself, never taken from the caller. Nothing in this path accepts
// or resolves a caller-supplied self identity, so the gate's authn-verified
// identity model survives intact.
//
// MECHANISM, NOT POLICY: the framework provides the port (the interface,
// the bank, the delivery act). Where and when to send — which writers exist,
// what triggers an intent — stays with the consumer (the server/blueprint
// declares writers; iteration bodies decide to queue intents). The one
// trigger shipped here (HeartbeatIteration) is the deliberately minimal
// working-state threshold the spec allows at this step; judgment and
// composition richness belong to Step 7.
//
// SOCIETY SEAM (spec A4): an egress TARGET is an address a writer delivers
// to — a string, never a mind-reference. Inter-mind delivery semantics are
// Castra's side of the seam (gap-analysis §4.1); nothing in this interface
// encodes them.

// OutboundMessage is one act of outward reach: what the mind sends out.
type OutboundMessage struct {
	// SelfID is the identity the message is sent AS. It is stamped by the
	// Self at delivery time (see Reach) — a caller-supplied value is
	// overwritten, so the egress path cannot speak as someone else.
	SelfID string `json:"self_id"`

	// Writer names the E-bank member to deliver through. Empty means the
	// first declared writer.
	Writer string `json:"writer,omitempty"`

	// Target is the address the writer delivers to. It is an address or
	// sink name, NEVER a mind-reference: society-layer delivery semantics
	// live outside this framework (spec A4, §4.1).
	Target string `json:"target"`

	// Content is what is said.
	Content string `json:"content"`

	// SentAt is stamped by the Self at delivery time.
	SentAt time.Time `json:"sent_at"`
}

// EgressWriter is the E-bank member contract: one pluggable way out of the
// process. Implementations are transports (a test sink, a stream, a webhook
// later); they must not encode society semantics. Delivery guarantees,
// retries, and TLS posture are future production-hardening work, named not
// solved (spec A3).
type EgressWriter interface {
	// Name identifies the writer within its Self's E bank.
	Name() string
	// Write delivers one outbound message. An error means the message did
	// not go out; the caller decides what failure costs (the Hum: log,
	// count, continue).
	Write(ctx context.Context, msg OutboundMessage) error
}

// Reach delivers one outbound message through this Self's E bank — the
// outward act. The message is sent AS this Self: its SelfID and SentAt are
// stamped here, whatever the caller put in them. The writer is resolved by
// name from this Self's own bank (empty means the first declared writer);
// a name outside the bank is a refusal, not a fallback — writers are bound
// to the Self whose banks they live in.
//
// Failures are observable and never fatal: every failed delivery (unknown
// writer, no bank, transport error) increments the Self's egress-failure
// counter and returns an error the caller may inspect.
func (s *Self) Reach(ctx context.Context, msg OutboundMessage) error {
	if len(s.e) == 0 {
		s.egressFailures.Add(1)
		return fmt.Errorf("self %q has no egress bank: no writers were declared, the mind has no way out", s.id)
	}

	name := msg.Writer
	if name == "" {
		name = s.e[0].Name()
	}
	var writer EgressWriter
	for _, w := range s.e {
		if w.Name() == name {
			writer = w
			break
		}
	}
	if writer == nil {
		s.egressFailures.Add(1)
		return fmt.Errorf("self %q has no egress writer %q: writers are bound to the Self whose banks they live in", s.id, name)
	}

	// Identity discipline (SEC gate): outbound reach is performed AS this
	// Self. The caller's SelfID — whatever it was — is overwritten.
	msg.SelfID = s.id
	msg.Writer = name
	msg.SentAt = time.Now()

	if err := writer.Write(ctx, msg); err != nil {
		s.egressFailures.Add(1)
		return fmt.Errorf("egress through writer %q of self %q failed: %w", name, s.id, err)
	}
	return nil
}

// EgressFailures reports how many outbound deliveries have failed for this
// Self — the AC5 observability: a failing writer is counted, never silent
// and never fatal.
func (s *Self) EgressFailures() uint64 {
	return s.egressFailures.Load()
}

// humOutboundKey is the working-state key iteration bodies queue outbound
// intents under (via QueueOutbound). The Hum drains it after every
// successful iteration and delivers through the Self's E bank — the
// Hum → egress path, Loop B's terminal arc.
const humOutboundKey = "hum.outbound"

// QueueOutbound queues one outbound intent in an iteration's working state.
// The Hum delivers queued intents through its Self's E bank after the
// iteration completes; a request-driven caller may instead call Reach
// directly. Bodies queue rather than dial: the working state stays the one
// place an iteration's effects accumulate, and delivery happens outside the
// working-state lock.
func QueueOutbound(st State, msg OutboundMessage) {
	queued, _ := st.Get(humOutboundKey).([]OutboundMessage)
	st.Set(humOutboundKey, append(queued, msg))
}

// drainOutbound removes and returns all queued outbound intents from a
// working state. Called by the Hum under its working-state lock.
func drainOutbound(st State) []OutboundMessage {
	queued, _ := st.Get(humOutboundKey).([]OutboundMessage)
	if len(queued) > 0 {
		st.Set(humOutboundKey, []OutboundMessage(nil))
	}
	return queued
}

// HeartbeatIteration returns an iteration body that behaves like
// PulseIteration and, every `every` pulses, queues ONE outbound heartbeat
// intent for the named writer and target. It is the minimal Step 6
// origination trigger — a working-state threshold, exactly the shape spec
// assumption A2 allows. What a mind SHOULD say, and when, is judgment:
// Step 7's composition, not this body's.
func HeartbeatIteration(every int, writer, target string) IterationFunc {
	if every < 1 {
		every = 1
	}
	return func(ctx context.Context, self *Self, working State) (State, error) {
		next, err := PulseIteration(ctx, self, working)
		if err != nil {
			return nil, err
		}
		if pulses, _ := next.Get("hum.pulses").(int); pulses%every == 0 {
			QueueOutbound(next, OutboundMessage{
				Writer:  writer,
				Target:  target,
				Content: fmt.Sprintf("heartbeat: pulse %d from a humming self", pulses),
			})
		}
		return next, nil
	}
}

// StreamWriter is the first concrete E-bank member: a JSON-line writer over
// an io.Writer (stdout, a file). Deliberately modest — one message, one
// line, no retries, no transport policy (spec A3: hardening beyond the SEC
// gate's posture is future work, named not solved).
type StreamWriter struct {
	name string

	mu sync.Mutex
	w  io.Writer
}

// NewStreamWriter builds a StreamWriter delivering to w under the given
// E-bank name.
func NewStreamWriter(name string, w io.Writer) *StreamWriter {
	return &StreamWriter{name: name, w: w}
}

// Name implements EgressWriter.
func (sw *StreamWriter) Name() string { return sw.name }

// Write implements EgressWriter: one message as one JSON line.
func (sw *StreamWriter) Write(_ context.Context, msg OutboundMessage) error {
	line, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encoding outbound message: %w", err)
	}
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if _, err := sw.w.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("writing outbound message to stream: %w", err)
	}
	return nil
}
