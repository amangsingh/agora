package agora_test

// Step 5 tests: R — the Hum.
//
// AC3 (ownership) holds by construction of this file: it lives in the ROOT
// package's test package and imports ONLY the root agora package plus stdlib
// — the Hum runs via agora.Self without pkg/server anywhere in sight
// (press/mind separation holds for R).
//
// AC1 is the anti-cron discriminator, the most important test in the plan:
// in-memory working state that is DELIBERATELY NOT persisted to D must
// survive N>=3 iterations. A cron-style build — reconstruct state from
// storage each tick — structurally cannot pass it, and cronLoop below is
// exactly that plausible wrong implementation, kept in-tree so the
// discriminator's teeth stay demonstrable:
//
//   - TestHum_AntiCronDiscriminator runs the probe against the real Hum
//     (green after Step 5; RED on HEAD 9c291b4 where no R exists).
//   - TestCronVariant_LosesResidue asserts the SAME probe loses the residue
//     against cronLoop — the discriminator discriminates.
//   - TestHum_AntiCronDiscriminator_CronVariant (env-gated,
//     AGORA_DISCRIMINATOR_VARIANT=cron) runs the discriminator's own
//     assertions against cronLoop and FAILS — the captured red output is
//     AC1(b) in the submission.

import (
	"context"
	"errors"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amangsingh/agora"
)

// testInterval is the Hum cadence used in tests: fast enough for tight
// deadlines, slow enough not to spin.
const testInterval = 2 * time.Millisecond

// newHummingSelf constructs a Self over a fresh in-memory D bank and a
// scripted L bank, ready to hum.
func newHummingSelf(t *testing.T, factory func(string) agora.Model) (*agora.Self, *memoryBank) {
	t.Helper()
	bank := newMemoryBank()
	if factory == nil {
		factory = func(string) agora.Model { return &scriptedModel{} }
	}
	self, err := agora.NewSelf(agora.Blueprint{
		ID:           "humming-self",
		Name:         "Humming Self",
		Models:       []string{"mock"},
		Memory:       bank,
		ModelFactory: factory,
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}
	return self, bank
}

// residentLoop is the shape the discriminator probes. The real Hum satisfies
// it; so does the cron-style wrong implementation — the probe cares only
// about observable behaviour, which is the whole point: the two must be
// distinguishable by the test, not by prose.
type residentLoop interface {
	Iterations() uint64
	SetWorking(key string, value any)
	Working(key string) any
}

// awaitIterations blocks until the loop has completed at least n MORE
// iterations than `from`, failing the test on deadline.
func awaitIterations(t *testing.T, loop residentLoop, from, n uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if loop.Iterations() >= from+n {
			return
		}
		time.Sleep(testInterval)
	}
	t.Fatalf("loop completed only %d iterations (wanted >= %d beyond %d) within deadline — the loop is not advancing",
		loop.Iterations(), n, from)
}

// probeResidue is the discriminator's act: plant unpersisted in-memory
// residue, let N>=3 iterations pass, report whether it survived.
func probeResidue(t *testing.T, loop residentLoop) (survived bool, got any) {
	t.Helper()
	const key = "hum.residue"
	const residue = "unpersisted-residue-7d3f" // never written to D, by design (spec A4)

	start := loop.Iterations()
	loop.SetWorking(key, residue)
	awaitIterations(t, loop, start, 3)

	got = loop.Working(key)
	return got == residue, got
}

// assertDiscriminator runs the AC1 assertions against any resident-shaped
// loop. Against the real Hum it passes; against a cron-style loop it must
// go red — that red run is AC1(b).
func assertDiscriminator(t *testing.T, loop residentLoop, bank *memoryBank) {
	t.Helper()
	survived, got := probeResidue(t, loop)
	if !survived {
		t.Errorf("AC1 FAIL (anti-cron discriminator): in-memory working state did not survive 3+ iterations — got %v, want %q.\n"+
			"A loop that loses unpersisted state between iterations is cron: it reconstructs the mind from storage each tick, and the gap is the reconstruction, not the interval.",
			got, "unpersisted-residue-7d3f")
	}
	// The residue must genuinely be unpersisted: D must have no trace of it.
	// (Survival via storage would be cron passing on a technicality.)
	for selfID, engrams := range bank.engrams {
		for _, e := range engrams {
			if e.Content == "unpersisted-residue-7d3f" {
				t.Errorf("AC1 FAIL: the residue leaked into the D bank (self %q) — the discriminator requires state that storage never had", selfID)
			}
		}
	}
}

// TestHum_AntiCronDiscriminator (AC1): the real Hum keeps unpersisted
// in-memory working state across N>=3 iterations.
// RED on HEAD 9c291b4: no R exists (this file does not compile there).
func TestHum_AntiCronDiscriminator(t *testing.T) {
	self, bank := newHummingSelf(t, nil)
	hum, err := self.StartHum(agora.HumConfig{Interval: testInterval})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	assertDiscriminator(t, hum, bank)
}

// TestHum_AntiCronDiscriminator_CronVariant (AC1b, env-gated): the SAME
// discriminator assertions, aimed at the plausible wrong implementation.
// Run with AGORA_DISCRIMINATOR_VARIANT=cron to watch it go red — that output
// is the proof the discriminator has teeth, attached to the submission.
func TestHum_AntiCronDiscriminator_CronVariant(t *testing.T) {
	if os.Getenv("AGORA_DISCRIMINATOR_VARIANT") != "cron" {
		t.Skip("red-proof variant: set AGORA_DISCRIMINATOR_VARIANT=cron to run the discriminator against the cron-style wrong implementation (expected FAIL)")
	}
	bank := newMemoryBank()
	loop := startCronLoop(bank, "humming-self", testInterval)
	defer loop.Stop()

	assertDiscriminator(t, loop, bank)
}

// TestCronVariant_LosesResidue (AC1b's in-tree half, green): the probe run
// against the cron-style loop LOSES the residue. If this test ever fails,
// the cron double has stopped being cron and the discriminator has lost its
// negative control.
func TestCronVariant_LosesResidue(t *testing.T) {
	bank := newMemoryBank()
	loop := startCronLoop(bank, "humming-self", testInterval)
	defer loop.Stop()

	survived, got := probeResidue(t, loop)
	if survived {
		t.Errorf("negative control broken: the cron-style loop kept unpersisted residue %v across iterations — it is no longer reconstructing from storage, so the discriminator has nothing to discriminate against", got)
	}
}

// TestHum_SelfDrivenAdvance (AC2): with ZERO inbound requests — this test
// makes no HTTP call and invokes no episode from outside — observable state
// advances on its own. RED on HEAD 9c291b4 (no R exists).
func TestHum_SelfDrivenAdvance(t *testing.T) {
	self, _ := newHummingSelf(t, nil)
	hum, err := self.StartHum(agora.HumConfig{Interval: testInterval})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	before := hum.Iterations()
	awaitIterations(t, hum, before, 3)

	if hum.Iterations() <= before {
		t.Fatalf("AC2 FAIL: iteration counter did not advance without external input")
	}
	pulses, _ := hum.Working("hum.pulses").(int)
	if pulses < 3 {
		t.Errorf("AC2 FAIL: the default pulse body advanced working state only %d times (want >= 3) — the loop is idling", pulses)
	}
}

// TestHum_Lifecycle (AC4): stopping the Self stops the Hum cleanly — the
// goroutine exits (leak-checked), iteration advance freezes, Stop is
// idempotent, and a stopped Self may hum again.
func TestHum_Lifecycle(t *testing.T) {
	before := runtime.NumGoroutine()

	self, _ := newHummingSelf(t, nil)
	hum, err := self.StartHum(agora.HumConfig{Interval: testInterval})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}

	// A second StartHum while humming must refuse: one Hum per Self.
	if _, err := self.StartHum(agora.HumConfig{Interval: testInterval}); err == nil {
		t.Errorf("AC4 FAIL: a Self accepted a second concurrent Hum")
	}

	awaitIterations(t, hum, 0, 1)

	// Close stops the Hum and blocks until the loop goroutine has exited.
	if err := self.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Advance must freeze after stop.
	frozen := hum.Iterations()
	time.Sleep(10 * testInterval)
	if got := hum.Iterations(); got != frozen {
		t.Errorf("AC4 FAIL: Hum advanced from %d to %d after Stop — the loop is not dead", frozen, got)
	}

	// Stop is idempotent (and racing a second Stop must not hang or panic).
	hum.Stop()

	// Leak check: the goroutine count returns to the pre-start level.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && runtime.NumGoroutine() > before {
		time.Sleep(5 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before {
		t.Errorf("AC4 FAIL: goroutine leak after Stop: %d before start, %d after stop", before, got)
	}

	// Dying well includes being able to live again: a stopped Self can hum.
	hum2, err := self.StartHum(agora.HumConfig{Interval: testInterval})
	if err != nil {
		t.Fatalf("AC4 FAIL: a Self whose Hum was stopped refused to hum again: %v", err)
	}
	hum2.Stop()
}

// TestHum_BodyVsAccessorConcurrency (SEC re-review bar, Finding 1): a
// sustained hammer proving the ownership model — an iteration body churning
// the working state races against outside observers calling SetWorking /
// Working — is safe under -race. This is NOT a one-shot probe: the body
// writes 64 keys per iteration on a 100µs cadence while an accessor loop
// hammers the same state for ~300ms.
//
// RED on merge 90bce99 (pre-fix): iterate() lent the working-state OBJECT to
// the body outside h.mu, so body writes raced accessor reads/writes — the
// race detector fires deterministically. GREEN under the fixed model: the
// lock guards the object, not the pointer; while the body holds the state,
// observers block until the Hum takes it back.
func TestHum_BodyVsAccessorConcurrency(t *testing.T) {
	self, _ := newHummingSelf(t, nil)

	// churn body: mutates the lent working state heavily, every iteration.
	churn := func(_ context.Context, _ *agora.Self, working agora.State) (agora.State, error) {
		for i := 0; i < 64; i++ {
			working.Set(churnKeys[i], i)
		}
		n, _ := working.Get("hum.pulses").(int)
		working.Set("hum.pulses", n+1)
		return working, nil
	}

	hum, err := self.StartHum(agora.HumConfig{Interval: 100 * time.Microsecond, Body: churn})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	// accessor hammer: the outside-observer API, as fast as it will go.
	deadline := time.Now().Add(300 * time.Millisecond)
	var probes int
	for time.Now().Before(deadline) {
		hum.SetWorking("probe", probes)
		if got := hum.Working("probe"); got != probes {
			t.Fatalf("ownership violation: wrote probe=%d, read back %v — an iteration body clobbered a key it does not own", probes, got)
		}
		probes++
	}

	// The loop must have kept advancing while being observed.
	if hum.Iterations() == 0 {
		t.Fatalf("the Hum starved: zero iterations completed during 300ms of accessor traffic")
	}
	if probes == 0 {
		t.Fatalf("the observer starved: zero accessor round-trips completed in 300ms — accessors never got the state back")
	}
}

// churnKeys are pre-built so the churn body allocates nothing per iteration
// that the race detector could mistake for synchronization.
var churnKeys = func() [64]string {
	var keys [64]string
	for i := range keys {
		keys[i] = "churn." + string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	return keys
}()

// denyModel is the AC5 canary: an L-bank member that must NEVER be invoked
// by the Hum's default body. Any invocation is recorded and fails loudly.
type denyModel struct {
	invocations atomic.Int64
}

func (d *denyModel) Invoke(context.Context, agora.ModelRequest) (agora.ModelResponse, error) {
	d.invocations.Add(1)
	return agora.ModelResponse{}, errors.New("deny: the egress-less Hum must not reach a model transport")
}

// TestHum_NoEgress (AC5): the Hum performs zero network dials. The Self's
// only outward-capable bank member is the L bank (a model transport); it is
// replaced with a denying canary, the D bank lives in test memory, and the
// default pulse body touches only RAM through Graph.Execute. Zero canary
// invocations across >=3 iterations means zero dials — and the diff itself
// contains no dialer, no net import, no writer (SEC verifies the absence
// against the diff).
func TestHum_NoEgress(t *testing.T) {
	canary := &denyModel{}
	self, _ := newHummingSelf(t, func(string) agora.Model { return canary })

	hum, err := self.StartHum(agora.HumConfig{Interval: testInterval})
	if err != nil {
		t.Fatalf("StartHum failed: %v", err)
	}
	defer hum.Stop()

	awaitIterations(t, hum, 0, 3)

	if n := canary.invocations.Load(); n != 0 {
		t.Errorf("AC5 FAIL: the Hum invoked a model transport %d times — R must land egress-less (no network path until Step 6 behind the SEC gate)", n)
	}
}

// --- the plausible wrong implementation ---------------------------------

// cronLoop is cron with extra steps, on purpose: a loop that RECONSTRUCTS
// its working state from the D bank on every tick. It satisfies the same
// residentLoop shape as the Hum, ticks just as fast, and still fails the
// discriminator — because the gap is the reconstruction, not the interval.
// It exists as the discriminator's negative control (AC1b).
type cronLoop struct {
	bank   *memoryBank
	selfID string

	mu      sync.Mutex
	working agora.State

	iterations atomic.Uint64
	stop       chan struct{}
	done       chan struct{}
}

func startCronLoop(bank *memoryBank, selfID string, interval time.Duration) *cronLoop {
	c := &cronLoop{
		bank:    bank,
		selfID:  selfID,
		working: reconstructFromStorage(bank, selfID),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go func() {
		defer close(c.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-c.stop:
				return
			case <-ticker.C:
				// THE defect: each tick resurrects the mind from storage.
				// Whatever only lived in RAM is gone.
				fresh := reconstructFromStorage(bank, selfID)
				n, _ := fresh.Get("hum.pulses").(int)
				fresh.Set("hum.pulses", n+1)
				c.mu.Lock()
				c.working = fresh
				c.mu.Unlock()
				c.iterations.Add(1)
			}
		}
	}()
	return c
}

// reconstructFromStorage builds working state purely from what D holds —
// the cron move.
func reconstructFromStorage(bank *memoryBank, selfID string) agora.State {
	st := &agora.ConversationState{BaseState: agora.NewBaseState()}
	engrams, _ := bank.RecallEngrams(selfID)
	for _, e := range engrams {
		st.History = append(st.History, agora.ChatMessage{Role: e.RelationalAnchor, Content: e.Content})
	}
	return st
}

func (c *cronLoop) Stop() {
	close(c.stop)
	<-c.done
}

func (c *cronLoop) Iterations() uint64 { return c.iterations.Load() }

func (c *cronLoop) SetWorking(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.working.Set(key, value)
}

func (c *cronLoop) Working(key string) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.working.Get(key)
}
