package compiler

import (
	"fmt"

	"github.com/amangsingh/agora"
)

// This file is the library half of "one truth, two consumers": the SAME
// parsed *Blueprint value that the press generates code from (Compile /
// GenerateProject) also constructs a live agora.Self here. The Step 4
// constructor (agora.NewSelf) is FED, never forked — this file only maps
// declaration to declaration and hands over.

// SelfOptions carries the live handles a YAML declaration cannot contain:
// the factory that turns a declared L-bank member into a client, and the
// D-bank store handle fulfilling the memory declaration.
type SelfOptions struct {
	// ModelFactory builds the client for one declared L-bank member. It is
	// the Step 3/4 test seam, surfaced unchanged: the root package has no
	// provider dependency to default to, so the consumer chooses one.
	ModelFactory func(m ModelDecl) agora.Model

	// Memory fulfills the D declaration with a live store handle. Nil
	// yields an amnesiac Self (agora.NewSelf's documented contract) — the
	// declaration is still carried, but episodes addressing a self id fail
	// loudly. Wiring a store FROM the declaration (e.g. opening the
	// declared sqlite DSN) is a consumer concern; the compiler stays free
	// of storage dependencies.
	Memory agora.MemoryStore
}

// SelfBlueprint maps the eleven-letter declaration onto the root framework
// declaration (agora.Blueprint) — the exact input Step 4's constructor was
// built for.
func (bp *Blueprint) SelfBlueprint(opts SelfOptions) (agora.Blueprint, error) {
	if opts.ModelFactory == nil {
		return agora.Blueprint{}, fmt.Errorf("constructing a Self from blueprint %q requires a ModelFactory: the L bank cannot be built without one", bp.Project)
	}

	// 1: identity. A blueprint that does not declare a self falls back to
	// the project name — the pre-Step-8 identity of a generated mind.
	id := bp.Self.ID
	if id == "" {
		id = bp.Project
	}
	name := bp.Self.Name
	if name == "" {
		name = bp.Project
	}

	// L: declared members become bank addresses; the factory closes over
	// the full declaration so provider/model/base_url survive the mapping.
	declByName := make(map[string]ModelDecl, len(bp.Models))
	modelNames := make([]string, 0, len(bp.Models))
	for _, m := range bp.Models {
		declByName[m.Name] = m
		modelNames = append(modelNames, m.Name)
	}
	factory := func(modelName string) agora.Model {
		decl, ok := declByName[modelName]
		if !ok {
			// An address outside the declared bank still reaches the
			// consumer's factory (agora.Self's documented on-demand path),
			// carrying the name as the provider-side model id.
			decl = ModelDecl{Name: modelName, Model: modelName}
		}
		return opts.ModelFactory(decl)
	}

	// T: declarations become framework tool definitions.
	tools := make([]agora.ToolDefinition, 0, len(bp.Tools))
	for _, t := range bp.Tools {
		tools = append(tools, agora.ToolDefinition{
			Type: "function",
			Function: agora.Function{
				Name:        t.Name,
				Description: t.Description,
			},
		})
	}

	// S: named persona material, declared order preserved.
	system := make([]agora.SystemPrompt, 0, len(bp.System))
	for _, s := range bp.System {
		system = append(system, agora.SystemPrompt{Name: s.Name, Content: s.Content})
	}

	// M: the declared working-state shape.
	state := make([]agora.StateField, 0, len(bp.State))
	for _, f := range bp.State {
		state = append(state, agora.StateField{Name: f.Name, Type: f.Type})
	}

	// K: declared knowledge sources.
	knowledge := make([]agora.KnowledgeSource, 0, len(bp.Knowledge))
	for _, k := range bp.Knowledge {
		knowledge = append(knowledge, agora.KnowledgeSource{Name: k.Name, URI: k.URI})
	}

	return agora.Blueprint{
		ID:           id,
		Name:         name,
		Models:       modelNames,
		ModelFactory: factory,
		System:       system,
		Tools:        tools,
		State:        state,
		Knowledge:    knowledge,
		Memory:       opts.Memory,
	}, nil
}

// ConstructSelf builds a live agora.Self from a parsed blueprint by feeding
// Step 4's constructor. This is the direct-library consumer of the
// declaration; Compile/GenerateProject is the press consumer. One truth,
// two consumers.
func ConstructSelf(bp *Blueprint, opts SelfOptions) (*agora.Self, error) {
	selfBP, err := bp.SelfBlueprint(opts)
	if err != nil {
		return nil, err
	}
	return agora.NewSelf(selfBP)
}
