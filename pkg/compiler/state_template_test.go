package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGeneratedState_TranscriptHonesty is AC4 of the transcript-honesty task:
// the T1/T2 fixes must hold in the GENERATED project's state.go, not only in
// the library. generateState hand-copies ToChatHistory/AppendTurn into every
// generated project, so a fix that only touches the library ships the defect
// to every generated module.
//
// Mechanism: run Compile, lift the emitted state.go verbatim into a minimal
// temp module (go.mod + replace directive to this repo + the transcript test
// below), and `go test` it hermetically (GOPROXY=off — same discipline as
// TestCompile_OutputCompiles). The generated state.go is judged in isolation
// so that unrelated generator defects (dangling tool-node edges, the
// mock_llm.go unused import — covered by TestCompile_OutputCompiles) cannot
// mask or fake this verdict.
//
// The injected test mirrors AC1–AC3 against the generated ConversationState:
//   - AC1: ToChatHistory must not alias the History backing array.
//   - AC2: a two-hop agent -> tool -> agent loop yields a transcript with the
//     user input exactly once, at index 0, in correct order.
//   - AC3: tool_calls survive the generated DeepCopy and still execute.
func TestGeneratedState_TranscriptHonesty(t *testing.T) {
	const blueprint = `
project: gen-state-honesty
version: 0.1.0
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
	outDir := generateProject(t, blueprint)

	generatedState, err := os.ReadFile(filepath.Join(outDir, "state.go"))
	if err != nil {
		t.Fatalf("generated state.go not readable: %v", err)
	}

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve repo root: %v", err)
	}

	harnessDir := t.TempDir()
	files := map[string]string{
		"state.go": string(generatedState),
		"go.mod": fmt.Sprintf(`module gen-state-honesty-harness

go 1.25

require github.com/amangsingh/agora v0.0.0

replace github.com/amangsingh/agora => %s
`, repoRoot),
		// A main package needs an entry point to compile as a test binary's
		// package under test; the generated main.go is out of scope here.
		"main.go":            "package main\n\nfunc main() {}\n",
		"transcript_test.go": generatedStateTranscriptTest,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(harnessDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write harness file %s: %v", name, err)
		}
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go toolchain not found: %v", err)
	}
	cmd := exec.Command(goBin, "test", "./...")
	cmd.Dir = harnessDir
	cmd.Env = append(os.Environ(),
		"GOPROXY=off",      // hermetic: module downloads impossible, not just avoided
		"GOFLAGS=-mod=mod", // allow go to record resolved requirements in the temp module
		"GOWORK=off",       // insulate from any ambient workspace file
		"GOSUMDB=off",      // no checksum-db lookups for cache-resolved modules
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("generated state.go fails the transcript-honesty tests:\n%s", out)
	}
}

// generatedStateTranscriptTest is the test file injected next to the
// generated state.go. It is package main because generateState emits package
// main. It exercises the GENERATED ConversationState through the library's
// tool nodes — the exact pairing every generated project ships with.
const generatedStateTranscriptTest = `package main

import (
	"context"
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/nodes"
)

type echoTool struct {
	Executions int
}

func (e *echoTool) Definition() agora.ToolDefinition {
	return agora.ToolDefinition{
		Type: "function",
		Function: agora.Function{
			Name:        "echo",
			Description: "Echoes its arguments back.",
			Parameters:  map[string]interface{}{"type": "object"},
		},
	}
}

func (e *echoTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	e.Executions++
	return args, nil
}

func newEchoToolCall(id string) agora.ToolCall {
	call := agora.ToolCall{ID: id, Type: "function"}
	call.Function.Name = "echo"
	call.Function.Arguments = map[string]interface{}{"value": "ping"}
	return call
}

type hookLLM struct {
	InvokeFunc func(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error)
}

func (m *hookLLM) Invoke(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
	return m.InvokeFunc(ctx, request)
}

// AC1 against the generated state.go.
func TestGeneratedToChatHistory_DoesNotAliasHistoryBacking(t *testing.T) {
	history := make([]agora.ChatMessage, 1, 8)
	history[0] = agora.ChatMessage{Role: "user", Content: "turn-1"}

	s := &ConversationState{
		BaseState: agora.NewBaseState(),
		History:   history,
		Input:     "turn-2",
	}

	messages, err := s.ToChatHistory()
	if err != nil {
		t.Fatalf("ToChatHistory returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	messages[len(messages)-1].Content = "corrupted-by-caller"

	backing := s.History[:cap(s.History)]
	for i := range backing {
		if backing[i].Content == "corrupted-by-caller" {
			t.Fatalf("ToChatHistory aliases the History backing array: caller mutation reached backing index %d", i)
		}
	}
	if s.History[0].Content != "turn-1" {
		t.Fatalf("History[0] corrupted: got %q, want %q", s.History[0].Content, "turn-1")
	}
}

// AC2 against the generated state.go.
func TestGeneratedAgentToolLoop_TranscriptHonest(t *testing.T) {
	const userInput = "What is in the box?"

	echo := &echoTool{}
	registry := agora.NewToolRegistry()
	registry.Register(echo)

	invocations := 0
	mockLLM := &hookLLM{
		InvokeFunc: func(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
			invocations++
			message := agora.ChatMessage{Role: "assistant"}
			if invocations == 1 {
				message.ToolCalls = []agora.ToolCall{newEchoToolCall("call-1")}
			} else {
				message.Content = "final answer"
			}
			return agora.ModelResponse{Choices: []agora.Choice{{Message: message}}}, nil
		},
	}

	g := agora.NewGraph()
	g.SetEntry("agent")
	g.AddNode("agent", nodes.ToolAgentNode(mockLLM, "You are a tool-using bot.", registry))
	g.AddNode("tools", nodes.ToolExecutorNode(registry))
	g.SetConditionalEdge("agent", func(s agora.State) string {
		if s.Get("tool_calls") != nil {
			return "tools"
		}
		return "END"
	})
	g.AddEdge("tools", "agent")

	state := &ConversationState{
		BaseState: agora.NewBaseState(),
		History:   []agora.ChatMessage{},
		Input:     userInput,
	}

	finalStateRaw, err := g.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("graph execution failed: %v", err)
	}
	if invocations != 2 {
		t.Fatalf("expected 2 LLM invocations, got %d", invocations)
	}
	if echo.Executions != 1 {
		t.Fatalf("expected the tool to execute exactly once, got %d", echo.Executions)
	}

	final := finalStateRaw.(*ConversationState)
	history := final.History

	userCount := 0
	for i, msg := range history {
		if msg.Role == "user" {
			userCount++
			if i != 0 {
				t.Errorf("user message found at index %d; the only user turn must be at index 0", i)
			}
			if msg.Content != userInput {
				t.Errorf("user message content = %q, want %q", msg.Content, userInput)
			}
		}
	}
	if userCount != 1 {
		t.Errorf("user input appears %d times in History, want exactly 1\nfull history: %+v", userCount, history)
	}

	wantRoles := []string{"user", "assistant", "tool", "assistant"}
	if len(history) != len(wantRoles) {
		t.Fatalf("History length = %d, want %d\nfull history: %+v", len(history), len(wantRoles), history)
	}
	for i, want := range wantRoles {
		if history[i].Role != want {
			t.Errorf("History[%d].Role = %q, want %q", i, history[i].Role, want)
		}
	}
	if len(history[1].ToolCalls) == 0 {
		t.Error("History[1] should carry the assistant's tool_calls")
	}
	if history[2].Role != "tool" || history[2].ToolCallID != "call-1" {
		t.Errorf("History[2] should be the tool result for call-1, got role=%q tool_call_id=%q",
			history[2].Role, history[2].ToolCallID)
	}
}

// AC3 against the generated state.go's DeepCopy.
func TestGeneratedToolExecutor_SurvivesDeepCopy(t *testing.T) {
	echo := &echoTool{}
	registry := agora.NewToolRegistry()
	registry.Register(echo)

	state := &ConversationState{
		BaseState: agora.NewBaseState(),
		History:   []agora.ChatMessage{},
	}
	state.Set("tool_calls", []agora.ToolCall{newEchoToolCall("call-1")})

	copied, err := state.DeepCopy()
	if err != nil {
		t.Fatalf("DeepCopy failed: %v", err)
	}

	result, err := nodes.ToolExecutorNode(registry)(context.Background(), copied)
	if err != nil {
		t.Fatalf("ToolExecutorNode failed on deep-copied state: %v", err)
	}
	if echo.Executions != 1 {
		t.Fatalf("expected the tool to execute exactly once on the forked state, got %d", echo.Executions)
	}

	finalState := result.State.(*ConversationState)
	if len(finalState.History) != 1 {
		t.Fatalf("expected 1 tool result message in History, got %d", len(finalState.History))
	}
	toolMsg := finalState.History[0]
	if toolMsg.Role != "tool" || toolMsg.ToolCallID != "call-1" {
		t.Errorf("expected tool result for call-1, got role=%q tool_call_id=%q", toolMsg.Role, toolMsg.ToolCallID)
	}
}
`
