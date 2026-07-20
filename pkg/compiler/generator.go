package compiler

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Compile orchestrates the generation process.
func Compile(blueprintPath, outputDir string) error {
	// 1. Parse
	bp, err := ParseBlueprint(blueprintPath)
	if err != nil {
		return err
	}

	fmt.Printf("Compiling project '%s' version %s...\n", bp.Project, bp.Version)

	return GenerateProject(bp, outputDir)
}

// GenerateProject emits a project from an already-parsed blueprint. It is
// the press consumer of the declaration; ConstructSelf (construct.go) is the
// library consumer. Both take the SAME *Blueprint value — one truth, two
// consumers.
func GenerateProject(bp *Blueprint, outputDir string) error {
	// 2. Generate Main
	if err := generateMain(bp, outputDir); err != nil {
		return err
	}

	// 3. Generate Go Mod
	if err := generateGoMod(bp, outputDir); err != nil {
		return err
	}

	// 4. Generate Graph (Architecture)
	if err := generateGraph(bp, outputDir); err != nil {
		return err
	}

	// 5. Generate State
	if err := generateState(bp, outputDir); err != nil {
		return err
	}

	// 6. Generate Tests (DOC-02)
	if err := generateTests(bp, outputDir); err != nil {
		return err
	}

	return nil
}

// ... existing code ...

func generateTests(bp *Blueprint, outDir string) error {
	// 1. Generate Mock LLM
	mockTmpl := `package main

import (
	"context"

	"github.com/amangsingh/agora"
)

// MockLLM is a test helper that returns static responses.
type MockLLM struct {
	Response string
}

func (m *MockLLM) Invoke(ctx context.Context, req agora.ModelRequest) (agora.ModelResponse, error) {
	return agora.ModelResponse{
		Choices: []agora.Choice{
			{
				Message: agora.ChatMessage{
					Role:    "assistant",
					Content: m.Response,
				},
			},
		},
	}, nil
}
`
	if err := SafeWriteFile(outDir, "mock_llm.go", []byte(mockTmpl)); err != nil {
		return err
	}

	// 2. Generate Graph Test
	testTmpl := `package main

import (
	"context"
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/nodes"
)

func TestGraphExecution(t *testing.T) {
	// 1. Setup
	g := NewGraph()
	ctx := context.Background()

	// 2. Swap real LLMs with Mocks (Dependency Injection pattern needed in generated code or we mock the nodes)
	// For this Phase 2 generator, we are directly replacing the nodes in the map for testing 
	// because NewGraph() returns strict nodes.
	
	// Create a mock
	mock := &MockLLM{Response: "Test Response"}
	
	// Replace agents with mocked agents
	// Note: In a real system we might use a Factory or specific Setter.
	// Here we just overwrite the node in the map if it exists.
	// Assuming specific node naming convention from blueprint.
	{{range .Nodes}}
	{{if eq .Type "agent"}}
	// Mocking {{.Name}}
	g.AddNode("{{.Name}}", nodes.SimpleAgentNode(mock, "System Prompt"))
	{{end}}
	{{end}}

	// 3. Execute
	initialState := &ConversationState{
		BaseState: agora.NewBaseState(),
		Input:     "Test Input",
	}

	finalState, err := g.Execute(ctx, initialState)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	// 4. Verify
	fs := finalState.(*ConversationState)
	if len(fs.History) == 0 {
		t.Error("Expected history but got none")
	} else {
		last := fs.History[len(fs.History)-1]
		if last.Content != "Test Response" {
			t.Errorf("Expected 'Test Response', got '%s'", last.Content)
		}
	}
}
`
	t, err := template.New("test").Parse(testTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse test template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, bp); err != nil {
		return fmt.Errorf("failed to execute test template: %w", err)
	}

	return SafeWriteFile(outDir, "graph_test.go", buf.Bytes())
}

func generateMain(bp *Blueprint, outDir string) error {
	_ = bp
	tmpl := `package main

import (
	"context"
	"fmt"
	"log"

	"github.com/amangsingh/agora"
)

func main() {
	ctx := context.Background()

	// 1. Initialize Graph
	g := NewGraph()

	// 2. Execute
	initialState := &ConversationState{
		BaseState: agora.NewBaseState(),
		Input:     "Hello from Compiled Agent!",
	}

	fmt.Println("Running agent...")
	finalState, err := g.Execute(ctx, initialState)
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}

	// Output result
	// Assuming state has an 'output' field or similar for demonstration
	fs := finalState.(*ConversationState)
	if len(fs.History) > 0 {
		last := fs.History[len(fs.History)-1]
		fmt.Printf("Final Output (%s): %s\n", last.Role, last.Content)
	} else {
		fmt.Println("No history generated.")
	}
}
`
	return SafeWriteFile(outDir, "main.go", []byte(tmpl))
}

func generateGoMod(bp *Blueprint, outDir string) error {
	tmpl := fmt.Sprintf(`module %s

go 1.25

require (
	github.com/amangsingh/agora v0.0.0
)
`, bp.Project)
	return SafeWriteFile(outDir, "go.mod", []byte(tmpl))
}

// goFieldName converts a declared snake_case state-field name into the
// exported Go field the generated struct carries (resonance_index ->
// ResonanceIndex). Names are parse-validated identifiers, so this never
// mangles silently.
func goFieldName(name string) string {
	parts := strings.Split(name, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

// goFieldType maps a declared M-bank field type to its Go emission. The
// parser has already rejected unknown types; reaching the default here
// means generateState was fed an unvalidated blueprint, which must fail
// loudly, not emit a guess.
func goFieldType(declared string) (string, error) {
	if goType, ok := stateFieldTypes[declared]; ok {
		return goType, nil
	}
	return "", fmt.Errorf("M bank: state field type '%s' has no Go emission (known: string, int, float, bool)", declared)
}

// generateState emits the generated project's state.go. It CONSUMES the
// blueprint: every declared M-bank field becomes a typed field of the
// generated ConversationState. (Step 8 killed the `_ = bp` defect here —
// the emitted state used to be a fixed template regardless of declaration.)
func generateState(bp *Blueprint, outDir string) error {
	type stateFieldGen struct {
		GoName  string
		GoType  string
		YAMLKey string
	}
	fields := make([]stateFieldGen, 0, len(bp.State))
	for _, f := range bp.State {
		goType, err := goFieldType(f.Type)
		if err != nil {
			return fmt.Errorf("state field '%s': %w", f.Name, err)
		}
		fields = append(fields, stateFieldGen{
			GoName:  goFieldName(f.Name),
			GoType:  goType,
			YAMLKey: f.Name,
		})
	}

	const structTmpl = `package main

import (
	"encoding/json"
	"fmt"
	"github.com/amangsingh/agora"
)

// ConversationState carries the transcript plus the M-bank fields this
// project's blueprint declares.
type ConversationState struct {
	agora.BaseState ` + "`mapstructure:\",squash\"`" + `
	History   []agora.ChatMessage ` + "`mapstructure:\"history\"`" + `
	Input     string        ` + "`mapstructure:\"input\"`" + `
{{- range .Fields}}
	{{.GoName}} {{.GoType}} ` + "`mapstructure:\"{{.YAMLKey}}\" json:\"{{.YAMLKey}}\"`" + ` // declared state field: {{.YAMLKey}}
{{- end}}
}`

	t, err := template.New("state-struct").Parse(structTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse state template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct{ Fields []stateFieldGen }{fields}); err != nil {
		return fmt.Errorf("failed to execute state template: %w", err)
	}

	tmpl := buf.String() + `

// ToChatHistory returns a freshly allocated slice combining History with the
// pending Input (when not yet consumed). It shares no backing storage with
// History, so caller mutations can never corrupt the state.
func (s *ConversationState) ToChatHistory() ([]agora.ChatMessage, error) {
	messages := make([]agora.ChatMessage, 0, len(s.History)+1)
	messages = append(messages, s.History...)
	if s.Input != "" {
		messages = append(messages, agora.ChatMessage{Role: "user", Content: s.Input})
	}
	return messages, nil
}

// AppendTurn consumes the pending user Input exactly once, then appends the
// output. Subsequent turns append only their own output — the user turn is
// never re-injected into the transcript.
func (s *ConversationState) AppendTurn(output agora.ChatMessage) error {
	if s.Input != "" {
		s.History = append(s.History, agora.ChatMessage{Role: "user", Content: s.Input})
		s.Input = ""
	}
	s.History = append(s.History, output)
	return nil
}

func (s *ConversationState) DeepCopy() (agora.State, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal for DeepCopy: %w", err)
	}
	var newState ConversationState
	if err := json.Unmarshal(bytes, &newState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal for DeepCopy: %w", err)
	}
	return &newState, nil
}
`
	return SafeWriteFile(outDir, "state.go", []byte(tmpl))
}

func generateGraph(bp *Blueprint, outDir string) error {
	// We need to generate the code that builds the graph.
	// This involves initializing nodes and edges.

	const tmplStr = `package main

import (
	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/llm"
	"github.com/amangsingh/agora/nodes"
)

func NewGraph() *agora.Graph {
	g := agora.NewGraph()
	g.MaxSteps = {{.Graph.MaxSteps}}
	g.SetEntry("{{.Graph.Entry}}")

	// --- Nodes ---
	{{range .Nodes}}
	// Node: {{.Name}} ({{.Type}})
	{{if eq .Type "agent"}}
	// Assuming LLM config is handled or mocked for now.
	// In a real compiler, we'd generate code to load the specific model config.
	model_{{.Name}} := llm.NewOllamaLLM("http://localhost:11434/v1", "{{.Model}}") 
	node_{{.Name}} := nodes.SimpleAgentNode(model_{{.Name}}, "{{.Instructions}}")
	g.AddNode("{{.Name}}", node_{{.Name}})
	{{end}}
	{{end}}

	// --- Edges ---
	{{range .Edges}}
	{{if eq .To "END"}}
	// Edge to END is implied by not having a next node in strict mode if strictly linear,
	// but we can be explicit or just comment.
	// g.AddEdge("{{.From}}", "") 
	{{else}}
	g.AddEdge("{{.From}}", "{{.To}}")
	{{end}}
	{{end}}

	return g
}
`
	t, err := template.New("graph").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("failed to parse graph template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, bp); err != nil {
		return fmt.Errorf("failed to execute graph template: %w", err)
	}

	return SafeWriteFile(outDir, "graph.go", buf.Bytes())
}
