// in agora/hum.go

package agora

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// This file introduces R of the anatomy: the Hum — a non-returning loop OWNED
// BY THE SELF, holding working state IN MEMORY across iterations, invoking
// Execute as an *episode* rather than being invoked by it.
//
// The change of kind (gap-analysis §3.4, verdict §3.5) is the ownership
// inversion of State:
//
//	Before R: caller → Execute owns State for one call → returns it → drops it.
//	Under R:  R owns State for the PROCESS → lends it to Execute for an
//	          episode → takes it back → keeps it.
//
// What distinguishes the Hum from cron is exactly what it OWNS between
// iterations: working state that lives in RAM and is deliberately NOT
// reconstructed from storage each tick. "Cron is resurrection on a timer;
// faster gaps are still gaps" — the gap is the reconstruction, not the
// interval. The anti-cron discriminator in hum_test.go seizes on this:
// unpersisted in-memory residue must survive iterations.
//
// SEAM (architect spec addendum, note 071uxkkz3wu3wwrl): the per-iteration
// body is a nameable, replaceable unit (IterationFunc). WHAT an iteration
// does is pluggable; THAT the loop owns in-memory state across iterations is
// the invariant. Cadence (loop), iteration body (IterationFunc), and episode
// assembly (inside the body) are three separate named units — a future
// anatomy router may host inside the body without touching the loop. No
// router is built here.
//
// NO EGRESS: nothing in this file opens a network path. The Hum reaches the
// world only after Step 6 lands behind the SEC gate (advisories A/B/C).

// DefaultHumInterval is the Hum's default cadence. The loop must not idle
// forever by default: with zero external input, observable state advances at
// least this often.
const DefaultHumInterval = time.Second

// IterationFunc is the pluggable per-iteration body of the Hum — the seam.
// The Hum LENDS its owned working state to the body; the body may run it
// through Graph.Execute as an episode against the Self's banks; the Hum takes
// the returned state back and KEEPS IT IN RAM for the next iteration.
//
// A body returning (nil, err) leaves the previous working state untouched —
// a failed iteration never costs the Self its accumulated state.
type IterationFunc func(ctx context.Context, self *Self, working State) (State, error)

// HumConfig configures a Hum at start. The zero value is valid: default
// cadence, default pulse body.
type HumConfig struct {
	// Interval is the iteration cadence. Zero or negative means
	// DefaultHumInterval. Cadence is a property of the loop, not of the body.
	Interval time.Duration

	// Body is the per-iteration body (the seam). Nil means PulseIteration.
	Body IterationFunc
}

// Hum is R: the non-returning loop the Self owns. It is constructed only by
// (*Self).StartHum. Its goroutine is the first deliberately unjoined-by-a-
// caller lifetime in this codebase — bounded not by a caller's wait but by
// Stop, because liveness includes dying well.
type Hum struct {
	self     *Self
	body     IterationFunc
	interval time.Duration

	// mu guards working. working is THE owned working state: it is created
	// once at StartHum, mutated in place or replaced by iteration results,
	// and never reconstructed from storage.
	mu      sync.Mutex
	working State

	iterations atomic.Uint64

	cancel   context.CancelFunc
	done     chan struct{}
	stopOnce sync.Once
}

// StartHum starts the Self's Hum: an explicit act on a constructed Self.
// A Self hums at most once at a time; stopping the Hum (or closing the Self)
// makes room for a new one.
func (s *Self) StartHum(cfg HumConfig) (*Hum, error) {
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultHumInterval
	}
	body := cfg.Body
	if body == nil {
		body = PulseIteration
	}

	ctx, cancel := context.WithCancel(context.Background())
	h := &Hum{
		self:     s,
		body:     body,
		interval: interval,
		// The working state is created ONCE, here. From this line until Stop,
		// it lives in RAM and is never rebuilt from D.
		working: &ConversationState{BaseState: NewBaseState()},
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	s.mu.Lock()
	if s.hum != nil {
		s.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("self %q is already humming: stop the running Hum before starting another", s.id)
	}
	s.hum = h
	s.mu.Unlock()

	go h.loop(ctx)
	return h, nil
}

// CurrentHum returns the Self's running Hum, or nil when the Self is not
// humming.
func (s *Self) CurrentHum() *Hum {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hum
}

// Close stops the Self's Hum if it is humming and blocks until the loop has
// fully exited. Stopping is part of the Self's lifecycle contract: a Self
// that dies well leaves no goroutine behind.
func (s *Self) Close() error {
	s.mu.Lock()
	h := s.hum
	s.mu.Unlock()
	if h != nil {
		h.Stop()
	}
	return nil
}

// loop is the non-returning loop itself. It owns cadence and nothing else:
// what an iteration does lives in the body (the seam), and what the Hum owns
// across iterations lives in h.working. It returns only when the Hum is
// stopped.
func (h *Hum) loop(ctx context.Context) {
	defer close(h.done)
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.iterate(ctx)
		}
	}
}

// iterate runs one iteration: lend the owned working state to the body, take
// the result back, keep it. The swap-back happens only on success, under the
// working-state lock — a failing or cancelled iteration leaves the previous
// state whole (no half-written working state).
func (h *Hum) iterate(ctx context.Context) {
	h.mu.Lock()
	working := h.working
	h.mu.Unlock()

	next, err := h.body(ctx, h.self, working)
	if err == nil && next != nil {
		h.mu.Lock()
		h.working = next
		h.mu.Unlock()
	}

	h.iterations.Add(1)
}

// Stop stops the Hum and blocks until its goroutine has fully exited. It is
// idempotent and safe to call concurrently. After Stop returns, the Self may
// start a new Hum.
func (h *Hum) Stop() {
	h.stopOnce.Do(func() {
		h.cancel()
		<-h.done

		s := h.self
		s.mu.Lock()
		if s.hum == h {
			s.hum = nil
		}
		s.mu.Unlock()
	})
}

// Iterations reports how many iterations the Hum has completed — the
// observable advance proving the process does not idle with zero external
// input.
func (h *Hum) Iterations() uint64 {
	return h.iterations.Load()
}

// Working reads a key from the Hum's owned working state.
func (h *Hum) Working(key string) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.working.Get(key)
}

// SetWorking mutates the Hum's owned working state in place. Values set here
// live in RAM only — nothing writes them to D. That unpersisted residue is
// the load-bearing difference between a Hum and a cron: reconstruct-from-
// storage cannot resurrect what storage never had.
func (h *Hum) SetWorking(key string, value any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.working.Set(key, value)
}

// PulseIteration is the default iteration body: the minimal act that is still
// a real episode. It assembles a one-node episode graph and runs the Hum's
// working state through Graph.Execute — the ownership inversion in miniature:
// the Hum lends the state, Execute threads it and hands it back, the Hum
// keeps it. Richness of judgment (V) is Step 7's composition; this body
// deliberately does no cognition and touches no network.
func PulseIteration(ctx context.Context, _ *Self, working State) (State, error) {
	g := NewGraph()
	g.AddNode("pulse", func(_ context.Context, st State) (NodeResult, error) {
		n, _ := st.Get("hum.pulses").(int)
		st.Set("hum.pulses", n+1)
		return NodeResult{State: st, IsDone: true}, nil
	})
	g.SetEntry("pulse")
	return g.Execute(ctx, working)
}
