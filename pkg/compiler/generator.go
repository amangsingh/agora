package compiler

import (
	"bytes"
	"fmt"
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
	"github.com/amangsingh/agora/llm"
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

func generateState(bp *Blueprint, outDir string) error {
	_ = bp // Suppress unused warning: v1 uses standard state, v2 will generate custom fields
	// For now, we use the standard ConversationState.
	// In the future, this could generate custom state structs based on YAML.
	tmpl := `package main

import (
	"encoding/json"
	"fmt"
	"github.com/amangsingh/agora"
)

type ConversationState struct {
	agora.BaseState ` + "`mapstructure:\",squash\"`" + `
	History   []agora.ChatMessage ` + "`mapstructure:\"history\"`" + `
	Input     string        ` + "`mapstructure:\"input\"`" + `
}

func (s *ConversationState) ToChatHistory() ([]agora.ChatMessage, error) {
	return append(s.History, agora.ChatMessage{Role: "user", Content: s.Input}), nil
}

func (s *ConversationState) AppendTurn(output agora.ChatMessage) error {
	s.History = append(s.History, agora.ChatMessage{Role: "user", Content: s.Input}, output)
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
