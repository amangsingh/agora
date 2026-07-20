package tests

import (
	"context"
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/nodes"
)

// echoTool is a minimal agora.Tool used to prove tool execution actually
// happened (Executions is incremented per call).
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

// newEchoToolCall builds a typed ToolCall targeting the echo tool.
func newEchoToolCall(id string) agora.ToolCall {
	call := agora.ToolCall{ID: id, Type: "function"}
	call.Function.Name = "echo"
	call.Function.Arguments = map[string]interface{}{"value": "ping"}
	return call
}

// TestToChatHistory_DoesNotAliasHistoryBacking is AC1 (T1).
//
// ToChatHistory documents that it "does NOT mutate the state". That promise
// breaks when History has spare capacity: append writes the synthetic user
// turn into History's backing array, and any caller mutation of the returned
// slice corrupts state the History slice can later grow back over.
func TestToChatHistory_DoesNotAliasHistoryBacking(t *testing.T) {
	history := make([]agora.ChatMessage, 1, 8) // cap > len: the aliasing condition
	history[0] = agora.ChatMessage{Role: "user", Content: "turn-1"}

	s := &agora.ConversationState{
		BaseState: agora.NewBaseState(),
		History:   history,
		Input:     "turn-2",
	}

	messages, err := s.ToChatHistory()
	if err != nil {
		t.Fatalf("ToChatHistory returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (history + current input), got %d", len(messages))
	}

	// Mutate the returned slice's last element.
	messages[len(messages)-1].Content = "corrupted-by-caller"

	// The original History backing array must be unaffected.
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

// TestAgentToolLoop_TranscriptHonest is AC2 (T2).
//
// Drives a two-iteration agent -> tool -> agent loop against the mock LLM and
// asserts the final History is an honest transcript: the user input appears
// EXACTLY ONCE, at index 0, with the assistant tool_calls turn, the tool
// result, and the final assistant turn in order — and no stray user message
// wedged between an assistant tool_calls message and its tool result.
func TestAgentToolLoop_TranscriptHonest(t *testing.T) {
	const userInput = "What is in the box?"

	echo := &echoTool{}
	registry := agora.NewToolRegistry()
	registry.Register(echo)

	invocations := 0
	mockLLM := &MockLLM{
		InvokeFunc: func(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
			invocations++
			message := agora.ChatMessage{Role: "assistant"}
			if invocations == 1 {
				// First hop: the model asks for a tool.
				message.ToolCalls = []agora.ToolCall{newEchoToolCall("call-1")}
			} else {
				// Second hop: the model answers using the tool result.
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

	state := &agora.ConversationState{
		BaseState: agora.NewBaseState(),
		History:   []agora.ChatMessage{},
		Input:     userInput,
	}

	finalStateRaw, err := g.Execute(context.Background(), state)
	if err != nil {
		t.Fatalf("graph execution failed: %v", err)
	}
	if invocations != 2 {
		t.Fatalf("expected 2 LLM invocations (agent -> tool -> agent), got %d", invocations)
	}
	if echo.Executions != 1 {
		t.Fatalf("expected the tool to execute exactly once, got %d", echo.Executions)
	}

	final := finalStateRaw.(*agora.ConversationState)
	history := final.History

	// The user input appears EXACTLY ONCE, at index 0.
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

	// Exact transcript shape: user, assistant(tool_calls), tool, assistant.
	wantRoles := []string{"user", "assistant", "tool", "assistant"}
	if len(history) != len(wantRoles) {
		t.Fatalf("History length = %d, want %d\nfull history: %+v", len(history), len(wantRoles), history)
	}
	for i, want := range wantRoles {
		if history[i].Role != want {
			t.Errorf("History[%d].Role = %q, want %q", i, history[i].Role, want)
		}
	}

	// The assistant tool_calls turn must be IMMEDIATELY followed by its tool
	// result — nothing wedged between them.
	if len(history[1].ToolCalls) == 0 {
		t.Error("History[1] should carry the assistant's tool_calls")
	}
	if history[2].Role != "tool" || history[2].ToolCallID != "call-1" {
		t.Errorf("History[2] should be the tool result for call-1, got role=%q tool_call_id=%q",
			history[2].Role, history[2].ToolCallID)
	}
	if history[3].Content != "final answer" {
		t.Errorf("History[3].Content = %q, want %q", history[3].Content, "final answer")
	}
}

// TestToolExecutor_SurvivesDeepCopy is AC3 (T3).
//
// tool_calls stored as []agora.ToolCall degrade to generic JSON shapes across
// DeepCopy (the JSON round-trip ParallelNode uses to fork state). The
// executor must still run the tools on the forked copy instead of dying with
// "invalid tool calls format in state".
func TestToolExecutor_SurvivesDeepCopy(t *testing.T) {
	echo := &echoTool{}
	registry := agora.NewToolRegistry()
	registry.Register(echo)

	state := &agora.ConversationState{
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

	finalState := result.State.(*agora.ConversationState)
	if len(finalState.History) != 1 {
		t.Fatalf("expected 1 tool result message in History, got %d", len(finalState.History))
	}
	toolMsg := finalState.History[0]
	if toolMsg.Role != "tool" || toolMsg.ToolCallID != "call-1" {
		t.Errorf("expected tool result for call-1, got role=%q tool_call_id=%q", toolMsg.Role, toolMsg.ToolCallID)
	}
}
