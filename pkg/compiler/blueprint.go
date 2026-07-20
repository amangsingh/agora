package compiler

// Blueprint represents the unmarshalled YAML declaration of a mind — the
// eleven ratified letters (specs/agora-ratified-anatomy.md). A blueprint
// DECLARES banks; agora.Self is CONSTRUCTED from that declaration (see
// construct.go — the Step 4 constructor is fed, never forked).
//
// Letter map (ratified 2026-06-21, adapted to the Step 4 Self bank naming):
//
//	1 = Self       (identity — the ONLY singular element)
//	N = Nodes      (functions / outward services)
//	L = Models     (LLM compute)
//	T = Tools      (external action)
//	S = System     (system-prompt / persona material)
//	M = State      (working-state shape)
//	K = Knowledge  (reference sources)
//	D = Memory     (storage, the engram store)
//	O = Graph+Edges (operations / internal logic)
//	E = Endpoints  (the bidirectional I/O membrane: ingress + egress)
//	R = Rhythm     (the Hum — declared here; behavior is Steps 5/9)
//
// Every bank other than Self is PLURAL and PLUGGABLE. All letter sections
// are optional in YAML, so pre-Step-8 (v1) blueprints — project / version /
// graph / nodes / edges — parse unchanged. Unknown or malformed declarations
// fail loudly at parse (parser.go); nothing is silently dropped.
type Blueprint struct {
	Project string `yaml:"project"`
	Version string `yaml:"version"`

	// Self is the 1: the identity this blueprint declares a mind for.
	Self SelfDecl `yaml:"self"`

	// Graph and Edges are the O bank: operations / internal logic.
	Graph GraphConfig `yaml:"graph"`
	Edges []EdgeGen   `yaml:"edges"`

	// Nodes is the N bank: functions / outward services.
	Nodes []NodeGen `yaml:"nodes"`

	// Models is the L bank: named, plural, swappable LLM declarations.
	Models []ModelDecl `yaml:"models"`

	// Tools is the T bank.
	Tools []ToolDecl `yaml:"tools"`

	// System is the S bank: named system-prompt / persona material.
	System []SystemDecl `yaml:"system"`

	// State is the M bank: the declared shape of working state. The
	// generator emits these as fields of the generated state struct, and
	// the Self seeds its working state with them at construction.
	State []StateFieldDecl `yaml:"state"`

	// Knowledge is the K bank.
	Knowledge []KnowledgeDecl `yaml:"knowledge"`

	// Memory is the D bank declaration.
	Memory *MemoryDecl `yaml:"memory"`

	// Endpoints is the E membrane: ingress AND egress, declared. Egress
	// behavior is Step 6/9 work; the declaration layer is complete here.
	Endpoints EndpointsDecl `yaml:"endpoints"`

	// Rhythm is the R declaration: the Hum. Declared-but-thin at this step
	// (honest naming): starting R from generated output is Step 9.
	Rhythm *RhythmDecl `yaml:"rhythm"`
}

// SelfDecl is the 1 — identity in the Step 2/3 self_id model.
type SelfDecl struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type GraphConfig struct {
	Entry    string `yaml:"entry"`
	MaxSteps int    `yaml:"max_steps"`
}

type NodeGen struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"` // "agent", "tool", "subgraph"
	Model        string   `yaml:"model,omitempty"`
	Instructions string   `yaml:"instructions,omitempty"` // For agents
	Tools        []string `yaml:"tools,omitempty"`        // List of tool names
}

type EdgeGen struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// ModelDecl is one member of the L bank. Name is the bank address (what
// nodes and episodes refer to); Provider/Model/BaseURL describe the client
// to construct for it.
type ModelDecl struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"` // "ollama", "openai", "google"
	Model    string `yaml:"model"`    // provider-side model identifier
	BaseURL  string `yaml:"base_url,omitempty"`
}

// ToolDecl is one member of the T bank.
type ToolDecl struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// SystemDecl is one member of the S bank: named persona material.
type SystemDecl struct {
	Name    string `yaml:"name"`
	Content string `yaml:"content"`
}

// StateFieldDecl is one field of the M bank's declared working-state shape.
type StateFieldDecl struct {
	Name string `yaml:"name"` // snake_case identifier
	Type string `yaml:"type"` // one of stateFieldTypes
}

// KnowledgeDecl is one member of the K bank.
type KnowledgeDecl struct {
	Name string `yaml:"name"`
	Kind string `yaml:"kind"` // "file" (single ratified kind at this step)
	URI  string `yaml:"uri"`
}

// MemoryDecl declares the D bank.
type MemoryDecl struct {
	Kind string `yaml:"kind"` // "sqlite" or "none"
	DSN  string `yaml:"dsn,omitempty"`
}

// EndpointsDecl declares the E membrane — bidirectional by construction.
type EndpointsDecl struct {
	Ingress []IngressDecl `yaml:"ingress"`
	Egress  []EgressDecl  `yaml:"egress"`
}

// IngressDecl is how the world reaches IN (ratified kinds: rest, cron,
// listener).
type IngressDecl struct {
	Name string `yaml:"name"`
	Kind string `yaml:"kind"`
	Addr string `yaml:"addr,omitempty"`
}

// EgressDecl is how the mind reaches OUT (ratified kind: writer). Behavior
// is Step 6/9; the declaration is real now.
type EgressDecl struct {
	Name   string `yaml:"name"`
	Kind   string `yaml:"kind"`
	Target string `yaml:"target,omitempty"`
}

// RhythmDecl declares R, the Hum. Declaration-level only at this step.
type RhythmDecl struct {
	Enabled bool `yaml:"enabled"`
}
