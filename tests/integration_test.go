package tests

import (
	"context"
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/nodes"
)

func TestIntegration_SimpleAgent(t *testing.T) {
	// 1. Setup Manual Mock LLM
	mockLLM := &MockLLM{
		InvokeFunc: func(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
			return agora.ModelResponse{
				Choices: []agora.Choice{
					{
						Message: agora.ChatMessage{
							Role:    "assistant",
							Content: "Hello, World!",
						},
					},
				},
			}, nil
		},
	}

	// 2. Setup Graph with SimpleAgentNode
	g := agora.NewGraph()
	g.SetEntry("agent")
	g.AddNode("agent", nodes.SimpleAgentNode(mockLLM, "You are a helpful bot."))

	// 3. Setup State
	// Use ConversationState to handle history automatically
	convState := &agora.ConversationState{
		BaseState: agora.NewBaseState(),
		History:   []agora.ChatMessage{},
		Input:     "Hi there",
	}

	// 4. Execute
	finalStateData, err := g.Execute(context.Background(), convState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 5. Verify
	// Check if output is set
	output := finalStateData.Get("output")
	if output != "Hello, World!" {
		t.Errorf("expected output 'Hello, World!', got %v", output)
	}

	// Check if history was updated (User input + AI response)
	// Cast back to ConversationState to inspect History
	finalConvState, ok := finalStateData.(*agora.ConversationState)
	if !ok {
		t.Fatal("expected ConversationState type assertion to succeed")
	}

	if len(finalConvState.History) != 2 {
		t.Fatalf("expected history len 2, got %d", len(finalConvState.History))
	}

	if role := finalConvState.History[0].Role; role != "user" {
		t.Errorf("expected first msg role 'user', got %s", role)
	}
	if role := finalConvState.History[1].Role; role != "assistant" {
		t.Errorf("expected second msg role 'assistant', got %s", role)
	}
	if content := finalConvState.History[1].Content; content != "Hello, World!" {
		t.Errorf("expected second msg content 'Hello, World!', got %s", content)
	}
}
