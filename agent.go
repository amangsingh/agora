// in agent.go

package agora

import (
	"context"
	"fmt"
)

// SimpleAgentNode is a factory that creates a NodeFunc for a basic agent.
// It takes an LLM implementation and uses it to respond to user input.
func SimpleAgentNode(llm LLM, instructions string) NodeFunc {
	return func(ctx context.Context, s State) (NodeResult, error) {
		// 1. get the user input from the state
		userInput, ok := s.Values["input"].(string)
		if !ok {
			return NodeResult{}, fmt.Errorf("Input not found or not a string in state")
		}

		// 2. Prepare the messages for the LLM
		messages := []ChatMessage{
			{Role: "system", Content: instructions},
			{Role: "user", Content: userInput},
		}

		// 3. Call the LLM
		responseMessage, err := llm.Chat(ctx, messages)
		if err != nil {
			return NodeResult{}, fmt.Errorf("Failed to call LLM: %w", err)
		}

		// 4. Update the state with the response
		s.Values["output"] = responseMessage.Content

		// 5. Return the result, indicating we are done for this turn
		return NodeResult{
			State: s,
			NextNode: "",
			IsDone: true,
		}, nil
	}
}
