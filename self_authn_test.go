package agora_test

// AC6 (mechanism/policy split, Pa's ruling made falsifiable): this file
// exercises the SEC gate's credential VERIFICATION mechanism from the root
// package, importing ONLY agora plus stdlib. If the mechanism could not be
// exercised without pkg/server, or if the framework itself enforced the
// require-credential POLICY, the split would be wrong — policy is asserted
// only in pkg/server's tests, and the framework proof here is that an
// episode still runs WITHOUT any credential when the consumer's policy
// does not demand one.
//
// It also pins advisory B's seam at the framework level: the recall window
// is declarable on the blueprint's D-bank declaration, and the strategy
// behind it is swappable without touching the store or its schema.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/amangsingh/agora"
)

// credentialedBank is a memoryBank with the SEC gate's optional D-bank
// capabilities: establishment and verification, in memory.
type credentialedBank struct {
	*memoryBank
	credentials map[string]string
}

func newCredentialedBank() *credentialedBank {
	return &credentialedBank{memoryBank: newMemoryBank(), credentials: map[string]string{}}
}

func (b *credentialedBank) EstablishSelf(id, name string) (string, error) {
	if _, taken := b.credentials[id]; taken {
		return "", agora.ErrSelfUnavailable
	}
	cred := "cred-for-" + id
	b.credentials[id] = cred
	b.selves[id] = true
	return cred, nil
}

func (b *credentialedBank) VerifySelfCredential(selfID, credential string) error {
	stored, known := b.credentials[selfID]
	if !known || stored != credential {
		return agora.ErrSelfAuthentication
	}
	return nil
}

func newAuthnSelf(t *testing.T, bank agora.MemoryStore, window int) (*agora.Self, *scriptedModel) {
	t.Helper()
	model := &scriptedModel{}
	self, err := agora.NewSelf(agora.Blueprint{
		ID:           "root-authn-self",
		Name:         "Root Authn Self",
		Models:       []string{"scripted"},
		ModelFactory: func(string) agora.Model { return model },
		Memory:       bank,
		RecallWindow: window,
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}
	return self, model
}

// TestSelf_CredentialMechanism (AC6): establish and verify through the Self
// with no server anywhere in sight.
func TestSelf_CredentialMechanism(t *testing.T) {
	bank := newCredentialedBank()
	self, _ := newAuthnSelf(t, bank, 0)

	cred, err := self.EstablishSelf("self-root")
	if err != nil {
		t.Fatalf("EstablishSelf failed: %v", err)
	}
	if cred == "" {
		t.Fatal("EstablishSelf minted an empty credential")
	}

	if err := self.VerifySelf("self-root", cred); err != nil {
		t.Errorf("AC6 FAIL: the minted credential does not verify: %v", err)
	}
	if err := self.VerifySelf("self-root", "wrong"); err == nil {
		t.Error("AC6 FAIL: a wrong credential verified")
	}
	if err := self.VerifySelf("self-never-established", cred); err == nil {
		t.Error("AC6 FAIL: an unestablished self verified")
	}

	// Re-establishment is a clean, typed refusal — first contact is a race
	// the loser survives.
	if _, err := self.EstablishSelf("self-root"); err != agora.ErrSelfUnavailable {
		t.Errorf("re-establishing a taken id: got %v, want ErrSelfUnavailable", err)
	}
}

// TestSelf_MechanismWithoutPolicy (AC6, the split itself): the framework
// enforces nothing. An episode addressing a self runs credential-free when
// the consumer's policy does not demand verification — require-credential
// is pkg/server's stance, asserted in pkg/server's tests.
func TestSelf_MechanismWithoutPolicy(t *testing.T) {
	bank := newCredentialedBank()
	self, _ := newAuthnSelf(t, bank, 0)

	result, err := self.RunEpisode(context.Background(), agora.EpisodeRequest{
		SelfID: "self-unguarded-by-framework",
		Input:  "hello",
	})
	if err != nil {
		t.Fatalf("AC6 FAIL: the framework hard-codes credential policy — episode refused: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("episode did not complete: %+v", result)
	}
}

// TestSelf_MechanismAbsentFailsClosed: a D bank without the verification
// capability cannot authenticate anyone — VerifySelf errors rather than
// silently passing.
func TestSelf_MechanismAbsentFailsClosed(t *testing.T) {
	self, _ := newAuthnSelf(t, newMemoryBank(), 0)

	if err := self.VerifySelf("self-anyone", "any-credential"); err == nil {
		t.Error("VerifySelf succeeded against a D bank with no verification mechanism")
	}
	if _, err := self.EstablishSelf("self-anyone"); err == nil {
		t.Error("EstablishSelf succeeded against a D bank with no establishment mechanism")
	}
}

// TestSelf_RecallWindowDeclarable (advisory B at the framework): the window
// is declared on the blueprint's D-bank declaration; an episode warms up
// from at most the newest N engrams, chronological, and the store keeps
// everything (the memoryBank has no windowed capability, so this also pins
// the in-memory bounding fallback).
func TestSelf_RecallWindowDeclarable(t *testing.T) {
	bank := newCredentialedBank()
	const window = 3
	self, model := newAuthnSelf(t, bank, window)

	const selfID = "self-windowed-root"
	if _, err := self.EstablishSelf(selfID); err != nil {
		t.Fatalf("EstablishSelf failed: %v", err)
	}
	for i := 1; i <= 7; i++ {
		bank.engrams[selfID] = append(bank.engrams[selfID], agora.Engram{
			SelfID:               selfID,
			Content:              fmt.Sprintf("root-fact-%d", i),
			AffectiveCoefficient: 0.1,
			RelationalAnchor:     "user",
			CreatedAt:            time.Now(),
		})
	}

	if _, err := self.RunEpisode(context.Background(), agora.EpisodeRequest{SelfID: selfID, Input: "recall"}); err != nil {
		t.Fatalf("RunEpisode failed: %v", err)
	}

	payload := model.requests[len(model.requests)-1]
	var recalled []string
	for _, m := range payload.Messages {
		if len(m.Content) > 10 && m.Content[:10] == "root-fact-" {
			recalled = append(recalled, m.Content)
		}
	}
	want := []string{"root-fact-5", "root-fact-6", "root-fact-7"}
	if len(recalled) != len(want) {
		t.Fatalf("declared window %d, payload carries %d engrams (%v)", window, len(recalled), recalled)
	}
	for i := range want {
		if recalled[i] != want[i] {
			t.Errorf("window content/order wrong at %d: got %q, want %q (newest N, chronological)", i, recalled[i], want[i])
		}
	}
	if len(bank.engrams[selfID]) < 7 {
		t.Errorf("windowing mutated the store: %d engrams remain of 7 — the bound must live on the read", len(bank.engrams[selfID]))
	}
}

// TestSelf_RecallStrategySeam (architect addendum): the strategy is a
// nameable seam on the D bank — a declared strategy replaces most-recent-N
// without any change to the store or its schema.
func TestSelf_RecallStrategySeam(t *testing.T) {
	bank := newCredentialedBank()
	model := &scriptedModel{}
	self, err := agora.NewSelf(agora.Blueprint{
		ID:           "root-seam-self",
		Models:       []string{"scripted"},
		ModelFactory: func(string) agora.Model { return model },
		Memory:       bank,
		Recall:       oldestOne{},
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}

	const selfID = "self-seam"
	if _, err := self.EstablishSelf(selfID); err != nil {
		t.Fatalf("EstablishSelf failed: %v", err)
	}
	for i := 1; i <= 4; i++ {
		bank.engrams[selfID] = append(bank.engrams[selfID], agora.Engram{
			SelfID: selfID, Content: fmt.Sprintf("seam-fact-%d", i),
			AffectiveCoefficient: 0, RelationalAnchor: "user", CreatedAt: time.Now(),
		})
	}
	if _, err := self.RunEpisode(context.Background(), agora.EpisodeRequest{SelfID: selfID, Input: "recall"}); err != nil {
		t.Fatalf("RunEpisode failed: %v", err)
	}

	payload := model.requests[len(model.requests)-1]
	sawOldest, sawOthers := false, false
	for _, m := range payload.Messages {
		if m.Content == "seam-fact-1" {
			sawOldest = true
		}
		if m.Content == "seam-fact-2" || m.Content == "seam-fact-3" || m.Content == "seam-fact-4" {
			sawOthers = true
		}
	}
	if !sawOldest || sawOthers {
		t.Errorf("declared strategy did not drive recall (oldest-one): sawOldest=%v sawOthers=%v", sawOldest, sawOthers)
	}
}

// oldestOne is a deliberately un-default strategy: exactly the single oldest
// engram. It exists to prove the seam turns.
type oldestOne struct{}

func (oldestOne) Name() string { return "oldest-one" }

func (oldestOne) Recall(d agora.MemoryStore, selfID string) ([]agora.Engram, error) {
	all, err := d.RecallEngrams(selfID)
	if err != nil || len(all) == 0 {
		return all, err
	}
	return all[:1], nil
}
