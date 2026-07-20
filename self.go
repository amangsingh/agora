// in agora/self.go

package agora

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// This file introduces the 1 of the anatomy: agora.Self, the framework type
// that owns the resource banks for PROCESS lifetime. Before the Self,
// resources were conjured per-request inside a handler and drowned when it
// returned. After it, a constructed Self holds identity and the live banks —
// L (models), T (tools), S (persona material), M (working state),
// K (knowledge sources), D (the memory store) — and requests EXECUTE THROUGH
// them.
//
// The Self is a FRAMEWORK TYPE, never a server singleton: anyone importing
// the root agora package can construct one from a Blueprint without touching
// pkg/server. pkg/server is merely the first consumer.
//
// Import discipline: llm/, nodes/, and pkg/storage all import this package,
// so nothing here may import them. The bank contracts (Model, MemoryStore)
// are therefore defined structurally in this file; the llm/ providers satisfy
// Model as-is, and *storage.Repository implements MemoryStore.

// Model is the L-bank contract: what the Self requires of a large language
// model client. It is structurally identical to llm.LLM, so every provider in
// llm/ (Ollama, OpenAI, Google) is a Model without modification — but the
// contract lives here so the framework type never has to import upward.
type Model interface {
	Invoke(ctx context.Context, request ModelRequest) (ModelResponse, error)
}

// Engram is the framework-level unit of mind-memory flowing through the D
// bank: the same affective-relational schema the Step 2 store ratified
// (content born with an affective coefficient and a relational anchor — "if
// it isn't in the schema, it isn't in the soul"). It mirrors the persisted
// shape; the storage row identity stays a storage concern.
type Engram struct {
	SelfID               string
	Content              string
	AffectiveCoefficient float64 // valence/intensity in [-1.0, 1.0]
	RelationalAnchor     string  // who/what this memory is bound to; never blank
	CreatedAt            time.Time
}

// MemoryStore is the D-bank contract: what the Self requires of a memory
// store. *storage.Repository implements it; tests may implement it in-memory.
type MemoryStore interface {
	// EnsureSelf resolves a self id to a registered self, creating it when it
	// does not exist yet (the Step 2 identity model: an unknown self is a new
	// self whose memory starts empty).
	EnsureSelf(id string) error
	// RecallEngrams loads the stored engrams of a self, oldest first.
	RecallEngrams(selfID string) ([]Engram, error)
	// FormEngram persists one new engram of a self.
	FormEngram(e Engram) error
}

// KnowledgeSource is a declared member of the K bank. At this step it is a
// declaration only (name + locator); reading through knowledge sources is
// later work.
type KnowledgeSource struct {
	Name string
	URI  string
}

// Blueprint is the declaration a Self is constructed from. At this step it is
// a config struct; Step 8 feeds the same shape from a parsed YAML blueprint
// and grows the banks to full declarative plurality, without the constructor
// signature changing shape.
type Blueprint struct {
	// ID is the self id this Self is, in the Step 2/3 self_id identity model.
	ID   string
	Name string

	// Models declares the L bank: model names whose clients are constructed
	// ONCE, at Self construction. The first declared model is the default for
	// episodes that do not name one.
	Models []string

	// ModelFactory builds the client for a declared model name. It is the
	// Step 3 test seam, preserved at the Self level: tests inject counting or
	// capturing mocks here. It is required — the root package deliberately
	// has no provider dependency to default to; the consumer chooses one
	// (e.g. pkg/server wires llm.NewOllamaLLM).
	ModelFactory func(model string) Model

	// SystemPrompts declares the S bank: system-prompt / persona material.
	SystemPrompts []string

	// Tools declares the T bank.
	Tools []ToolDefinition

	// Knowledge declares the K bank.
	Knowledge []KnowledgeSource

	// Memory is the D bank handle. Nil is allowed and yields an amnesiac
	// Self: episodes that address a self id then fail loudly rather than
	// silently forgetting.
	Memory MemoryStore
}

// Self is the 1: the singular owner of identity and the live resource banks
// for the lifetime of the process that constructed it. Banks are plural in
// shape (maps and slices) even where this step populates a single member —
// Step 8 makes them declaratively plural; nothing here will need to change
// shape for that.
type Self struct {
	id   string
	name string

	mu           sync.Mutex
	l            map[string]Model // L: live model clients, keyed by model name
	factory      func(model string) Model
	defaultModel string

	t []ToolDefinition  // T: tools
	s []string          // S: system-prompt / persona material
	m BaseState         // M: working state for process lifetime
	k []KnowledgeSource // K: knowledge sources
	d MemoryStore       // D: the memory store handle
}

// NewSelf constructs a Self from its Blueprint. Construction is where the
// banks are built — ONCE. Every declared model's client is constructed here
// and lives for the process; no request ever constructs one.
func NewSelf(bp Blueprint) (*Self, error) {
	if bp.ModelFactory == nil {
		return nil, fmt.Errorf("blueprint for self %q declares no ModelFactory: the L bank cannot be built", bp.ID)
	}

	s := &Self{
		id:      bp.ID,
		name:    bp.Name,
		l:       make(map[string]Model, len(bp.Models)),
		factory: bp.ModelFactory,
		t:       append([]ToolDefinition(nil), bp.Tools...),
		s:       append([]string(nil), bp.SystemPrompts...),
		m:       NewBaseState(),
		k:       append([]KnowledgeSource(nil), bp.Knowledge...),
		d:       bp.Memory,
	}

	for i, name := range bp.Models {
		if name == "" {
			return nil, fmt.Errorf("blueprint for self %q declares an empty model name (index %d)", bp.ID, i)
		}
		if _, dup := s.l[name]; dup {
			continue
		}
		s.l[name] = bp.ModelFactory(name)
		if i == 0 {
			s.defaultModel = name
		}
	}

	return s, nil
}

// ID returns the self id this Self is (the Step 2/3 identity model).
func (s *Self) ID() string { return s.id }

// Name returns the Self's display name.
func (s *Self) Name() string { return s.name }

// ModelFor returns the L-bank member for a model name, defaulting to the
// first declared model when the name is empty. A name outside the declared
// bank is constructed once via the blueprint's factory and then owned by the
// bank for the rest of the process — never per request.
func (s *Self) ModelFor(name string) Model {
	if name == "" {
		name = s.defaultModel
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.l[name]; ok {
		return m
	}
	m := s.factory(name)
	s.l[name] = m
	return m
}

// ResolveSelf is the single nameable choke point through which every
// request's self resolution passes on its way into the Self's banks. The
// SEC gate task (authn on self resolution) binds HERE and nowhere else.
//
// An empty selfID is the legacy amnesiac path (Step 3 backward compat): no
// self, no recall, no memory formation. An unknown selfID is registered as a
// new self whose memory starts empty.
func (s *Self) ResolveSelf(selfID string) (string, error) {
	if selfID == "" {
		return "", nil
	}
	if s.d == nil {
		return "", fmt.Errorf("self %q cannot be resolved: this Self has no memory bank (D)", selfID)
	}
	if err := s.d.EnsureSelf(selfID); err != nil {
		return "", fmt.Errorf("self %q could not be resolved or created: %w", selfID, err)
	}
	return selfID, nil
}

// EpisodeRequest addresses one episode of execution through the Self's banks.
type EpisodeRequest struct {
	// SelfID addresses the stable self whose memory this episode runs
	// within. Empty is the legacy amnesiac one-shot.
	SelfID string
	// Input is the user turn for this episode.
	Input string
	// Model names the L-bank member to think with; empty uses the default.
	Model string
}

// EpisodeResult is what an episode leaves behind for its caller.
type EpisodeResult struct {
	// Status is "completed" or "failed". A failed execution is a result, not
	// an error: the episode ran and its outcome is the failure text.
	Status string
	// Output is the final assistant content ("" when history is empty), or
	// the failure text when Status is "failed".
	Output string
	// History is the full final transcript of the episode (warm recalled
	// prefix + new turns) when Status is "completed".
	History []ChatMessage
	// NewTurns are the turns THIS episode added beyond the recalled prefix;
	// they have already been formed into the D bank when the episode had a
	// resolved self.
	NewTurns []ChatMessage
}

// RunEpisode executes one episode through the Self's banks: resolve the
// addressed self (choke point), recall its engrams from D into the starting
// state, think through the L bank under the S bank's persona, then form the
// new turns back into D. The returned error covers pre-flight failures
// (resolution, recall); execution failure is a Status of "failed".
func (s *Self) RunEpisode(ctx context.Context, req EpisodeRequest) (EpisodeResult, error) {
	resolvedID, err := s.ResolveSelf(req.SelfID)
	if err != nil {
		return EpisodeResult{}, err
	}

	// Recall: load the resolved self's stored engrams back INTO the starting
	// state, so this episode begins where the self's memory left off.
	var warmHistory []ChatMessage
	var warmEngrams []Engram
	if resolvedID != "" {
		warmEngrams, err = s.d.RecallEngrams(resolvedID)
		if err != nil {
			return EpisodeResult{}, fmt.Errorf("loading engrams for self %q: %w", resolvedID, err)
		}
		warmHistory = make([]ChatMessage, 0, len(warmEngrams))
		for _, e := range warmEngrams {
			// The relational anchor of a conversational engram is the speaker
			// it was formed as, so the role round-trips without loss.
			warmHistory = append(warmHistory, ChatMessage{Role: e.RelationalAnchor, Content: e.Content})
		}
	}

	model := s.ModelFor(req.Model)

	g := NewGraph()
	g.MaxSteps = 10
	g.AddNode("agent", s.agentStep(model))
	g.SetEntry("agent")

	baseState := NewBaseState()
	if warmEngrams != nil {
		// Carry the full engram data (affective coefficient, relational
		// anchor) into the state alongside the chat-shaped history, so the
		// read path drops nothing the schema preserved.
		baseState.Set("engrams", warmEngrams)
	}
	initialState := &ConversationState{
		BaseState: baseState,
		History:   warmHistory,
		Input:     req.Input,
	}

	s.recordEpisode()

	finalStateRaw, err := g.Execute(ctx, initialState)
	if err != nil {
		return EpisodeResult{Status: "failed", Output: err.Error()}, nil
	}

	fs := finalStateRaw.(*ConversationState)
	result := EpisodeResult{Status: "completed", History: fs.History}
	if len(fs.History) > 0 {
		result.Output = fs.History[len(fs.History)-1].Content
	}

	// Memory formation: persist only the turns THIS episode added (the warm
	// prefix is already in the store) as engrams of the self.
	if resolvedID != "" && len(fs.History) > len(warmHistory) {
		result.NewTurns = fs.History[len(warmHistory):]
		s.formEngrams(resolvedID, result.NewTurns)
	}

	return result, nil
}

// Episodes reports how many episodes this Self has executed — working state
// held in the M bank for the life of the process.
func (s *Self) Episodes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, _ := s.m.Get("episodes").(int)
	return n
}

func (s *Self) recordEpisode() {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, _ := s.m.Get("episodes").(int)
	s.m.Set("episodes", n+1)
}

// agentStep is the Self's single reasoning step over a member of its L bank:
// present the S bank's persona and the conversation, invoke the model, append
// the turn. It is expressed here (rather than reusing nodes.SimpleAgentNode)
// because nodes/ imports this package and can never be imported back.
func (s *Self) agentStep(model Model) NodeFunc {
	instructions := strings.Join(s.s, "\n\n")
	return func(ctx context.Context, st State) (NodeResult, error) {
		messagesForLLM, err := st.ToChatHistory()
		if err != nil {
			return NodeResult{State: st}, fmt.Errorf("could not get chat history: %w", err)
		}

		fullMessages := messagesForLLM
		if instructions != "" {
			fullMessages = append([]ChatMessage{
				{Role: "system", Content: instructions},
			}, messagesForLLM...)
		}

		response, err := model.Invoke(ctx, ModelRequest{Messages: fullMessages})
		if err != nil {
			return NodeResult{State: st}, fmt.Errorf("failed to invoke LLM: %w", err)
		}
		if len(response.Choices) == 0 {
			return NodeResult{State: st}, fmt.Errorf("LLM returned no choices")
		}
		assistantMessage := response.Choices[0].Message

		st.Set("output", assistantMessage.Content)
		if err := st.AppendTurn(assistantMessage); err != nil {
			return NodeResult{State: st}, fmt.Errorf("could not append turn to history: %w", err)
		}

		// Mirrors nodes.SimpleAgentNode: no next node, no done signal — the
		// edgeless episode graph terminates implicitly, exactly as before.
		return NodeResult{State: st}, nil
	}
}

// formEngrams persists the new turns of an episode as engrams of the self.
// The affective coefficient is neutral at formation (affect inference is a
// later step); the relational anchor records the speaker so recall can
// rebuild the turn faithfully. Failures are logged, not fatal: the episode
// already happened, but a memory that failed to form must not fail silently.
func (s *Self) formEngrams(selfID string, turns []ChatMessage) {
	for _, msg := range turns {
		engram := Engram{
			SelfID:               selfID,
			Content:              msg.Content,
			AffectiveCoefficient: 0.0,
			RelationalAnchor:     msg.Role,
			CreatedAt:            time.Now(),
		}
		if err := s.d.FormEngram(engram); err != nil {
			fmt.Printf("Failed to form engram for self %s: %v\n", selfID, err)
		}
	}
}
