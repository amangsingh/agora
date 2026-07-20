package agora_test

// AC3 (framework-type proof, Pa's ruling made falsifiable): this file lives
// OUTSIDE pkg/server and imports ONLY the root agora package plus stdlib.
// It constructs a Self from a blueprint value and executes an episode against
// a mock LLM bank. If this test could not be written without importing
// pkg/server, the ruling ("agora.Self is a framework type; pkg/server is
// merely the first consumer") would be violated.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amangsingh/agora"
)

// scriptedModel is a mock member of the L bank: it records every request it
// is invoked with and returns a scripted assistant reply.
type scriptedModel struct {
	requests []agora.ModelRequest
}

func (m *scriptedModel) Invoke(_ context.Context, req agora.ModelRequest) (agora.ModelResponse, error) {
	m.requests = append(m.requests, req)
	return agora.ModelResponse{
		Choices: []agora.Choice{{
			FinishReason: "stop",
			Message: agora.ChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("scripted-reply-%d", len(m.requests)),
			},
		}},
	}, nil
}

// memoryBank is an in-memory D bank: a MemoryStore that lives entirely in the
// test, proving the Self depends on the contract, not on any concrete store.
type memoryBank struct {
	selves  map[string]bool
	engrams map[string][]agora.Engram
}

func newMemoryBank() *memoryBank {
	return &memoryBank{selves: map[string]bool{}, engrams: map[string][]agora.Engram{}}
}

func (b *memoryBank) EnsureSelf(id string) error {
	b.selves[id] = true
	return nil
}

func (b *memoryBank) RecallEngrams(selfID string) ([]agora.Engram, error) {
	return b.engrams[selfID], nil
}

func (b *memoryBank) FormEngram(e agora.Engram) error {
	b.engrams[e.SelfID] = append(b.engrams[e.SelfID], e)
	return nil
}

// TestSelf_IsAFrameworkType (AC3): construct a Self from a blueprint value
// and execute an episode through its banks — no pkg/server anywhere.
func TestSelf_IsAFrameworkType(t *testing.T) {
	model := &scriptedModel{}
	bank := newMemoryBank()

	self, err := agora.NewSelf(agora.Blueprint{
		ID:            "framework-self",
		Name:          "Framework Self",
		Models:        []string{"mock"},
		SystemPrompts: []string{"You are a framework-constructed mind."},
		Memory:        bank,
		ModelFactory: func(string) agora.Model {
			return model
		},
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}

	result, err := self.RunEpisode(context.Background(), agora.EpisodeRequest{
		SelfID: "framework-self",
		Input:  "hello from outside pkg/server",
		Model:  "mock",
	})
	if err != nil {
		t.Fatalf("RunEpisode failed: %v", err)
	}

	if result.Output != "scripted-reply-1" {
		t.Errorf("AC3 FAIL: episode output %q, want %q", result.Output, "scripted-reply-1")
	}
	if len(model.requests) != 1 {
		t.Fatalf("AC3 FAIL: expected exactly 1 invocation of the mock L bank member, got %d", len(model.requests))
	}
	sawInput := false
	for _, m := range model.requests[0].Messages {
		if strings.Contains(m.Content, "hello from outside pkg/server") {
			sawInput = true
		}
	}
	if !sawInput {
		t.Errorf("AC3 FAIL: the episode's input never reached the L bank member")
	}
}

// TestSelf_EpisodeFlowsThroughDBank (AC3 continuation): memory formed by one
// episode is recalled by the next, entirely through the blueprint-injected D
// bank — the framework type owns the whole recall loop, not the server.
func TestSelf_EpisodeFlowsThroughDBank(t *testing.T) {
	model := &scriptedModel{}
	bank := newMemoryBank()

	// Seed a pre-existing engram directly through the D bank contract.
	if err := bank.FormEngram(agora.Engram{
		SelfID:               "warm-self",
		Content:              "seeded-fact: the observatory dome is copper",
		AffectiveCoefficient: 0.5,
		RelationalAnchor:     "user",
		CreatedAt:            time.Now(),
	}); err != nil {
		t.Fatalf("seeding engram failed: %v", err)
	}

	self, err := agora.NewSelf(agora.Blueprint{
		ID:     "warm-self",
		Models: []string{"mock"},
		Memory: bank,
		ModelFactory: func(string) agora.Model {
			return model
		},
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}

	if _, err := self.RunEpisode(context.Background(), agora.EpisodeRequest{
		SelfID: "warm-self",
		Input:  "what is the dome made of?",
		Model:  "mock",
	}); err != nil {
		t.Fatalf("RunEpisode failed: %v", err)
	}

	// The seeded engram must have been recalled into the model payload.
	sawSeed := false
	for _, m := range model.requests[0].Messages {
		if strings.Contains(m.Content, "seeded-fact") {
			sawSeed = true
		}
	}
	if !sawSeed {
		t.Errorf("AC3 FAIL: seeded engram did not flow from the D bank into the episode payload")
	}

	// The episode's new turns must have been formed back through the D bank.
	formed, err := bank.RecallEngrams("warm-self")
	if err != nil {
		t.Fatalf("RecallEngrams failed: %v", err)
	}
	if len(formed) < 3 { // seed + user turn + assistant turn
		t.Errorf("AC3 FAIL: expected the episode to form new engrams through the D bank (want >= 3 incl. seed, got %d)", len(formed))
	}
	for _, e := range formed {
		if strings.TrimSpace(e.RelationalAnchor) == "" {
			t.Errorf("engram %q has a blank relational anchor", e.Content)
		}
	}
}

// --- Step 8 (task otbw6y3k4hkwbh72): plural, declarable banks ---

// TestSelf_Step8_NamedBanksAddressable: the S bank takes named entries (with
// the unnamed Step 4 form still folding in after them), the M bank seeds its
// declared shape zero-valued, and T/K members are addressable by name.
func TestSelf_Step8_NamedBanksAddressable(t *testing.T) {
	self, err := agora.NewSelf(agora.Blueprint{
		ID:     "step8-self",
		Models: []string{"fast", "deep"},
		ModelFactory: func(model string) agora.Model {
			return &scriptedModel{}
		},
		System: []agora.SystemPrompt{
			{Name: "core", Content: "named persona"},
		},
		SystemPrompts: []string{"legacy unnamed prompt"},
		Tools: []agora.ToolDefinition{
			{Type: "function", Function: agora.Function{Name: "echo"}},
		},
		State: []agora.StateField{
			{Name: "resonance_index", Type: "float"},
			{Name: "episodes_seen", Type: "int"},
		},
		Knowledge: []agora.KnowledgeSource{
			{Name: "journal", URI: "./journal.md"},
		},
	})
	if err != nil {
		t.Fatalf("NewSelf failed: %v", err)
	}

	if got := len(self.DeclaredModels()); got != 2 {
		t.Errorf("L bank: %d members, want 2", got)
	}
	if sp, ok := self.SystemPromptByName("core"); !ok || sp.Content != "named persona" {
		t.Errorf("S bank: named entry 'core' not addressable (got %+v, ok=%v)", sp, ok)
	}
	if _, ok := self.Tool("echo"); !ok {
		t.Error("T bank: 'echo' not addressable")
	}
	if _, ok := self.Knowledge("journal"); !ok {
		t.Error("K bank: 'journal' not addressable")
	}
	if v, ok := self.WorkingValue("resonance_index").(float64); !ok || v != 0.0 {
		t.Errorf("M bank: resonance_index not seeded float64 zero, got %v", self.WorkingValue("resonance_index"))
	}
	if v, ok := self.WorkingValue("episodes_seen").(int); !ok || v != 0 {
		t.Errorf("M bank: episodes_seen not seeded int zero, got %v", self.WorkingValue("episodes_seen"))
	}
	if got := len(self.StateShape()); got != 2 {
		t.Errorf("M bank: declared shape %d fields, want 2", got)
	}
}

// TestSelf_Step8_MalformedDeclarationsFailLoudly: declaration defects fail
// Self construction — never a silent drop.
func TestSelf_Step8_MalformedDeclarationsFailLoudly(t *testing.T) {
	factory := func(model string) agora.Model { return &scriptedModel{} }

	cases := []struct {
		name string
		bp   agora.Blueprint
	}{
		{
			name: "unknown M-bank field type",
			bp: agora.Blueprint{
				ID: "x", ModelFactory: factory,
				State: []agora.StateField{{Name: "mood", Type: "quaternion"}},
			},
		},
		{
			name: "duplicate M-bank field",
			bp: agora.Blueprint{
				ID: "x", ModelFactory: factory,
				State: []agora.StateField{{Name: "mood", Type: "string"}, {Name: "mood", Type: "int"}},
			},
		},
		{
			name: "unnamed S-bank entry",
			bp: agora.Blueprint{
				ID: "x", ModelFactory: factory,
				System: []agora.SystemPrompt{{Content: "nameless"}},
			},
		},
		{
			name: "duplicate S-bank entry",
			bp: agora.Blueprint{
				ID: "x", ModelFactory: factory,
				System: []agora.SystemPrompt{{Name: "core", Content: "a"}, {Name: "core", Content: "b"}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := agora.NewSelf(tc.bp); err == nil {
				t.Fatal("malformed declaration was accepted silently")
			}
		})
	}
}
