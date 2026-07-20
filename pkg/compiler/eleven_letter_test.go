package compiler

// Step 8 (task otbw6y3k4hkwbh72): the banks become plural and declarable —
// eleven-letter Blueprint feeding agora.Self.
//
// RED BASELINE (captured at HEAD ce0256f before this step landed):
//   - AC1/AC4: the eleven-letter YAML below parsed WITHOUT error and every
//     bank declaration was SILENTLY DROPPED — blueprint.go:4-11 (Project,
//     Version, Graph, Nodes, Edges) could not express them.
//   - AC2: a blueprint declaring state field resonance_index (float)
//     produced a state.go WITHOUT it — generateState discarded bp
//     (generator.go:209 `_ = bp`) and emitted a fixed template.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amangsingh/agora"
)

// countingModel records which L-bank declaration constructed it.
type countingModel struct {
	decl ModelDecl
}

func (c *countingModel) Invoke(ctx context.Context, req agora.ModelRequest) (agora.ModelResponse, error) {
	return agora.ModelResponse{Choices: []agora.Choice{{Message: agora.ChatMessage{
		Role: "assistant", Content: "from " + c.decl.Name,
	}}}}, nil
}

// countingFactory is the injected consumer seam: it counts constructions and
// remembers every declaration it was handed.
type countingFactory struct {
	constructed []ModelDecl
}

func (f *countingFactory) build(m ModelDecl) agora.Model {
	f.constructed = append(f.constructed, m)
	return &countingModel{decl: m}
}

func writeBlueprint(t *testing.T, yamlContent string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agora.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// twoByTwoBlueprint declares TWO LLMs and TWO knowledge sources with
// parameterized member names, so AC1's variants differ ONLY in the blueprint.
func twoByTwoBlueprint(modelA, modelB, knowA, knowB string) string {
	return fmt.Sprintf(`
project: ac1-two-by-two
version: 2.0.0
self:
  id: ac1-self
  name: TwoByTwo
graph:
  entry: agent
  max_steps: 5
nodes:
  - name: agent
    type: agent
    model: %s
edges:
  - from: agent
    to: END
models:
  - name: %s
    provider: ollama
    model: llama3
  - name: %s
    provider: ollama
    model: llama3:70b
knowledge:
  - name: %s
    kind: file
    uri: ./a.md
  - name: %s
    kind: file
    uri: ./b.md
`, modelA, modelA, modelB, knowA, knowB)
}

// constructFromBlueprintFile is THE construction code AC1 holds fixed across
// blueprint variants: no per-variant branches, no code edits between runs.
func constructFromBlueprintFile(t *testing.T, path string) (*agora.Self, *countingFactory) {
	t.Helper()
	bp, err := ParseBlueprint(path)
	if err != nil {
		t.Fatalf("ParseBlueprint failed: %v", err)
	}
	f := &countingFactory{}
	self, err := ConstructSelf(bp, SelfOptions{ModelFactory: f.build})
	if err != nil {
		t.Fatalf("ConstructSelf failed: %v", err)
	}
	return self, f
}

// AC1: a blueprint declaring TWO LLMs and TWO knowledge sources produces a
// Self holding two of each, each addressable, swappable by editing ONLY the
// blueprint.
func TestElevenLetter_AC1_PluralBanksSwappableByBlueprintOnly(t *testing.T) {
	variants := []struct {
		name                       string
		modelA, modelB             string
		knowA, knowB               string
	}{
		{name: "variant-1", modelA: "fast", modelB: "deep", knowA: "notes", knowB: "docs"},
		{name: "variant-2", modelA: "scout", modelB: "sage", knowA: "atlas", knowB: "journal"},
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			path := writeBlueprint(t, twoByTwoBlueprint(v.modelA, v.modelB, v.knowA, v.knowB))
			self, f := constructFromBlueprintFile(t, path)

			// L bank: two members, constructed at Self construction.
			if got := len(f.constructed); got != 2 {
				t.Fatalf("expected 2 L-bank members constructed, got %d", got)
			}
			if got := len(self.DeclaredModels()); got != 2 {
				t.Fatalf("expected Self to hold 2 L-bank members, got %d (%v)", got, self.DeclaredModels())
			}
			// Each addressable, and distinct.
			ma := self.ModelFor(v.modelA)
			mb := self.ModelFor(v.modelB)
			if ma == nil || mb == nil {
				t.Fatal("declared L-bank members are not addressable")
			}
			if ma == mb {
				t.Fatalf("L-bank members %q and %q resolve to the same client", v.modelA, v.modelB)
			}
			if ma.(*countingModel).decl.Name != v.modelA {
				t.Errorf("ModelFor(%q) resolved to declaration %q", v.modelA, ma.(*countingModel).decl.Name)
			}

			// K bank: two members, each addressable by name.
			for _, k := range []string{v.knowA, v.knowB} {
				if _, ok := self.Knowledge(k); !ok {
					t.Errorf("K-bank member %q is not addressable", k)
				}
			}

			// Swappability: the other variant's addresses must NOT resolve —
			// membership is a property of the blueprint, not the code.
			other := variants[0]
			if v.name == variants[0].name {
				other = variants[1]
			}
			if _, ok := self.Knowledge(other.knowA); ok {
				t.Errorf("K-bank member %q resolves but belongs to the other blueprint variant", other.knowA)
			}
		})
	}
}

// AC2: the semantic test the compile gate structurally cannot supply — the
// emitted state.go must actually CONTAIN what the blueprint declares.
func TestElevenLetter_AC2_GeneratedStateReflectsDeclaredFields(t *testing.T) {
	const blueprint = `
project: ac2-semantic
version: 2.0.0
graph:
  entry: agent
  max_steps: 5
nodes:
  - name: agent
    type: agent
    model: llama3
edges:
  - from: agent
    to: END
state:
  - name: resonance_index
    type: float
  - name: watch_active
    type: bool
`
	outDir := generateProject(t, blueprint)
	raw, err := os.ReadFile(filepath.Join(outDir, "state.go"))
	if err != nil {
		t.Fatalf("emitted state.go not readable: %v", err)
	}
	// gofmt column-aligns struct fields, so collapse runs of whitespace
	// before asserting field presence.
	src := strings.Join(strings.Fields(string(raw)), " ")

	for _, want := range []string{
		"ResonanceIndex float64",
		`mapstructure:"resonance_index"`,
		"WatchActive bool",
		`mapstructure:"watch_active"`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("emitted state.go does not contain %q — the generator did not consume the M declaration\n--- emitted state.go ---\n%s", want, string(raw))
		}
	}
}

// elevenLetterAnatomyBlueprint is THE example the press must be able to
// print: a blueprint expressing the ratified resident-mind anatomy
// (specs/agora-ratified-anatomy.md) — all eleven letters, plural where the
// ratification says plural. Letter tags: 1 self, N nodes, L models, T tools,
// S system, M state, K knowledge, D memory, O graph+edges, E endpoints
// (ingress AND egress), R rhythm.
const elevenLetterAnatomyBlueprint = `
project: ratified-anatomy
version: 2.0.0
self:                      # 1 — the ONLY singular element
  id: anatomy-self
  name: The Resident Mind
graph:                     # O — operations / internal logic
  entry: converse
  max_steps: 10
nodes:                     # N — functions / outward services (plural)
  - name: converse
    type: agent
    model: fast
    instructions: "Converse."
  - name: reflect
    type: agent
    model: deep
    instructions: "Reflect."
edges:                     # O — routing
  - from: converse
    to: reflect
  - from: reflect
    to: END
models:                    # L — plural, pluggable
  - name: fast
    provider: ollama
    model: llama3
  - name: deep
    provider: ollama
    model: llama3:70b
tools:                     # T — plural, pluggable
  - name: file_reader
    description: "Reads files."
  - name: http_client
    description: "Calls the web."
system:                    # S — plural persona material
  - name: core-persona
    content: "You are the resident mind."
  - name: house-voice
    content: "Speak in the family register."
state:                     # M — declared working-state shape (plural fields)
  - name: episodes_seen
    type: int
  - name: resonance_index
    type: float
knowledge:                 # K — plural, pluggable
  - name: anatomy
    kind: file
    uri: ./specs/agora-ratified-anatomy.md
  - name: journal
    kind: file
    uri: ./journal.md
memory:                    # D — the engram store
  kind: sqlite
  dsn: ./mind.db
endpoints:                 # E — the BIDIRECTIONAL membrane
  ingress:
    - name: api
      kind: rest
      addr: ":8080"
  egress:
    - name: outbox
      kind: writer
      target: ./outbox
rhythm:                    # R — the Hum (declared; behavior is Steps 5/9)
  enabled: true
`

// AC4: the press prints THE example — the ratified anatomy parses,
// validates, and constructs a Self via the library path.
func TestElevenLetter_AC4_RatifiedAnatomyConstructs(t *testing.T) {
	path := writeBlueprint(t, elevenLetterAnatomyBlueprint)
	bp, err := ParseBlueprint(path)
	if err != nil {
		t.Fatalf("the ratified anatomy blueprint does not parse: %v", err)
	}

	// Every letter present in the parsed declaration.
	if bp.Self.ID != "anatomy-self" {
		t.Errorf("1 (self): got id %q", bp.Self.ID)
	}
	if len(bp.Nodes) != 2 {
		t.Errorf("N (nodes): got %d, want 2", len(bp.Nodes))
	}
	if len(bp.Models) != 2 {
		t.Errorf("L (models): got %d, want 2", len(bp.Models))
	}
	if len(bp.Tools) != 2 {
		t.Errorf("T (tools): got %d, want 2", len(bp.Tools))
	}
	if len(bp.System) != 2 {
		t.Errorf("S (system): got %d, want 2", len(bp.System))
	}
	if len(bp.State) != 2 {
		t.Errorf("M (state): got %d, want 2", len(bp.State))
	}
	if len(bp.Knowledge) != 2 {
		t.Errorf("K (knowledge): got %d, want 2", len(bp.Knowledge))
	}
	if bp.Memory == nil || bp.Memory.Kind != "sqlite" {
		t.Errorf("D (memory): got %+v, want sqlite declaration", bp.Memory)
	}
	if bp.Graph.Entry != "converse" || len(bp.Edges) != 2 {
		t.Errorf("O (graph/edges): entry=%q edges=%d", bp.Graph.Entry, len(bp.Edges))
	}
	if len(bp.Endpoints.Ingress) != 1 || len(bp.Endpoints.Egress) != 1 {
		t.Errorf("E (endpoints): ingress=%d egress=%d, want 1 and 1 (bidirectional)", len(bp.Endpoints.Ingress), len(bp.Endpoints.Egress))
	}
	if bp.Rhythm == nil || !bp.Rhythm.Enabled {
		t.Errorf("R (rhythm): got %+v, want enabled declaration", bp.Rhythm)
	}

	// And it CONSTRUCTS via the library path — the Step 4 constructor fed,
	// not forked.
	f := &countingFactory{}
	self, err := ConstructSelf(bp, SelfOptions{ModelFactory: f.build})
	if err != nil {
		t.Fatalf("the ratified anatomy does not construct a Self: %v", err)
	}
	if self.ID() != "anatomy-self" || self.Name() != "The Resident Mind" {
		t.Errorf("Self identity: id=%q name=%q", self.ID(), self.Name())
	}
	if got := len(self.DeclaredModels()); got != 2 {
		t.Errorf("Self L bank: %d members, want 2", got)
	}
	if _, ok := self.Tool("file_reader"); !ok {
		t.Error("Self T bank: file_reader not addressable")
	}
	if _, ok := self.Tool("http_client"); !ok {
		t.Error("Self T bank: http_client not addressable")
	}
	if _, ok := self.SystemPromptByName("house-voice"); !ok {
		t.Error("Self S bank: house-voice not addressable")
	}
	if _, ok := self.Knowledge("journal"); !ok {
		t.Error("Self K bank: journal not addressable")
	}
	if got := len(self.StateShape()); got != 2 {
		t.Errorf("Self M bank: declared shape has %d fields, want 2", got)
	}
	if v, ok := self.WorkingValue("resonance_index").(float64); !ok || v != 0.0 {
		t.Errorf("Self M bank: resonance_index not seeded as float64 zero (got %v)", self.WorkingValue("resonance_index"))
	}
}

// AC3: the compile gate still bites — Step 0's compile-the-output discipline
// passes over a project generated from an eleven-letter blueprint. Same
// neutralization as TestCompile_OutputCompiles' type-checking arms: the
// go.mod resolution defect is covered there, not re-litigated here.
func TestElevenLetter_AC3_GeneratedProjectCompiles(t *testing.T) {
	outDir := generateProject(t, elevenLetterAnatomyBlueprint)
	replaceAgoraWithLocalSource(t, outDir)
	parseGeneratedGoFiles(t, outDir)
	vetGeneratedModule(t, outDir)
}

// AC5: one truth — the compiler path and the direct-library path construct
// from the SAME declaration type, asserted on one blueprint value.
func TestElevenLetter_AC5_OneDeclarationTwoConsumers(t *testing.T) {
	path := writeBlueprint(t, elevenLetterAnatomyBlueprint)
	bp, err := ParseBlueprint(path)
	if err != nil {
		t.Fatalf("ParseBlueprint failed: %v", err)
	}

	// Consumer 1: the press.
	outDir := filepath.Join(t.TempDir(), "build")
	if err := GenerateProject(bp, outDir); err != nil {
		t.Fatalf("press consumer (GenerateProject) failed on the declaration: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(outDir, "state.go"))
	if err != nil {
		t.Fatalf("press consumer emitted no state.go: %v", err)
	}
	if !strings.Contains(string(raw), "ResonanceIndex") {
		t.Error("press consumer did not print the declared M field resonance_index")
	}

	// Consumer 2: the library, on the SAME value.
	f := &countingFactory{}
	self, err := ConstructSelf(bp, SelfOptions{ModelFactory: f.build})
	if err != nil {
		t.Fatalf("library consumer (ConstructSelf) failed on the same declaration: %v", err)
	}
	if v, ok := self.WorkingValue("resonance_index").(float64); !ok || v != 0.0 {
		t.Error("library consumer did not seed the declared M field resonance_index")
	}
}

// AC6: unknown or malformed letter declarations fail LOUDLY at parse — the
// silent-drop history does not repeat at the blueprint layer.
func TestElevenLetter_AC6_MalformedDeclarationsFailLoudly(t *testing.T) {
	const validCore = `
project: ac6
version: 2.0.0
graph:
  entry: agent
  max_steps: 5
nodes:
  - name: agent
    type: agent
edges:
  - from: agent
    to: END
`
	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name:    "unknown top-level letter",
			yaml:    validCore + "banks:\n  - nope\n",
			wantSub: "banks",
		},
		{
			name:    "typo'd known letter",
			yaml:    validCore + "modles:\n  - name: fast\n",
			wantSub: "modles",
		},
		{
			name:    "unknown field inside a bank member",
			yaml:    validCore + "models:\n  - name: fast\n    provider: ollama\n    model: llama3\n    tempreture: 0.7\n",
			wantSub: "tempreture",
		},
		{
			name:    "model without a name",
			yaml:    validCore + "models:\n  - provider: ollama\n    model: llama3\n",
			wantSub: "declares no name",
		},
		{
			name:    "duplicate model names",
			yaml:    validCore + "models:\n  - name: fast\n    provider: ollama\n    model: a\n  - name: fast\n    provider: ollama\n    model: b\n",
			wantSub: "duplicate model name",
		},
		{
			name:    "node addressing an undeclared L member",
			yaml:    strings.Replace(validCore, "type: agent", "type: agent\n    model: ghost", 1) + "models:\n  - name: fast\n    provider: ollama\n    model: llama3\n",
			wantSub: "not declared in the L bank",
		},
		{
			name:    "unknown state field type",
			yaml:    validCore + "state:\n  - name: mood\n    type: quaternion\n",
			wantSub: "unknown type",
		},
		{
			name:    "state field with invalid identifier",
			yaml:    validCore + "state:\n  - name: mood-vector\n    type: string\n",
			wantSub: "invalid",
		},
		{
			name:    "unknown memory kind",
			yaml:    validCore + "memory:\n  kind: postgres\n  dsn: whatever\n",
			wantSub: "unknown kind",
		},
		{
			name:    "sqlite memory without dsn",
			yaml:    validCore + "memory:\n  kind: sqlite\n",
			wantSub: "no dsn",
		},
		{
			name:    "unknown ingress kind",
			yaml:    validCore + "endpoints:\n  ingress:\n    - name: api\n      kind: websocket\n",
			wantSub: "unknown kind",
		},
		{
			name:    "unknown egress kind",
			yaml:    validCore + "endpoints:\n  egress:\n    - name: out\n      kind: teleport\n",
			wantSub: "unknown kind",
		},
		{
			name:    "knowledge without uri",
			yaml:    validCore + "knowledge:\n  - name: notes\n    kind: file\n",
			wantSub: "no uri",
		},
		{
			name:    "unknown knowledge kind",
			yaml:    validCore + "knowledge:\n  - name: notes\n    kind: telepathy\n    uri: ./x\n",
			wantSub: "unknown kind",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeBlueprint(t, tc.yaml)
			_, err := ParseBlueprint(path)
			if err == nil {
				t.Fatalf("malformed declaration was ACCEPTED (silent drop):\n%s", tc.yaml)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error does not name the defect: got %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// Backward compatibility: a pre-Step-8 (v1) blueprint — project / version /
// graph / nodes / edges only — parses and generates unchanged.
func TestElevenLetter_V1BlueprintStillParses(t *testing.T) {
	const v1 = `
project: legacy
version: 1.0.0
graph:
  entry: agent
  max_steps: 5
nodes:
  - name: agent
    type: agent
    model: llama3
edges:
  - from: agent
    to: END
`
	path := writeBlueprint(t, v1)
	bp, err := ParseBlueprint(path)
	if err != nil {
		t.Fatalf("v1 blueprint no longer parses: %v", err)
	}
	if len(bp.Models) != 0 || len(bp.State) != 0 || bp.Memory != nil {
		t.Error("v1 blueprint grew phantom bank declarations")
	}
	// v1 semantics: node.model is a raw provider model id, no L bank to
	// cross-check against.
	if bp.Nodes[0].Model != "llama3" {
		t.Errorf("v1 node model: got %q", bp.Nodes[0].Model)
	}
}
