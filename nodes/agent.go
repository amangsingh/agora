package nodes

import (
	"context"
	"fmt"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/llm"
)

// SimpleAgentNode factory creates a node that acts as a basic conversational agent.
// It uses the LLM interface to generate a response based on history + instructions.
func SimpleAgentNode(l llm.LLM, instructions string) agora.NodeFunc {
	return func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		// 1. Get the full conversation for the LLM call.
		messagesForLLM, err := s.ToChatHistory()
		if err != nil {
			return agora.NodeResult{State: s}, fmt.Errorf("could not get chat history: %w", err)
		}

		// 2. Prepend the system instructions.
		// Note: We create a new slice to avoid mutating the underlying state history if it was returned directly.
		fullMessages := append([]agora.ChatMessage{
			{Role: "system", Content: instructions},
		}, messagesForLLM...)

		request := agora.ModelRequest{
			Messages: fullMessages,
		}

		// 3. Call the LLM using the new Invoke signature.
		response, err := l.Invoke(ctx, request)
		if err != nil {
			return agora.NodeResult{State: s}, fmt.Errorf("failed to invoke LLM: %w", err)
		}

		// 4. Extract the content.
		// We expect at least one choice.
		if len(response.Choices) == 0 {
			return agora.NodeResult{State: s}, fmt.Errorf("LLM returned no choices")
		}
		assistantMessage := response.Choices[0].Message

		// 5. Update State
		// Set direct output
		s.Set("output", assistantMessage.Content)

		// Append turn to history
		if err := s.AppendTurn(assistantMessage); err != nil {
			return agora.NodeResult{State: s}, fmt.Errorf("could not append turn to history: %w", err)
		}

		// 6. Return result
		// By default, SimpleAgentNode doesn't decide the next node (leaves it empty) or signal done.
		// It just does its job and returns the updated state.
		return agora.NodeResult{
			State: s,
		}, nil
	}
}
