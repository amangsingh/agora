package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/llm"
)

// ToolAgentNode is a factory for an agent that can use tools.
// It generates a NodeFunc that calls the LLM with tool definitions.
func ToolAgentNode(l llm.LLM, instructions string, registry agora.ToolRegistry) agora.NodeFunc {
	return func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		// 1. Get history
		messagesForLLM, err := s.ToChatHistory()
		if err != nil {
			return agora.NodeResult{State: s}, fmt.Errorf("could not get chat history: %w", err)
		}

		// 2. Prepend the system instructions.
		fullMessages := append([]agora.ChatMessage{
			{Role: "system", Content: instructions},
		}, messagesForLLM...)

		// 3. Create the ModelRequest with Tools
		request := agora.ModelRequest{
			Messages:   fullMessages,
			Tools:      registry.GetDefinitions(),
			ToolChoice: "auto",
		}

		// 4. Call the LLM
		response, err := l.Invoke(ctx, request)
		if err != nil {
			return agora.NodeResult{State: s}, fmt.Errorf("failed to invoke LLM: %w", err)
		}

		// 5. Append thoughts/response to history
		// We expect at least one choice.
		if len(response.Choices) == 0 {
			return agora.NodeResult{State: s}, fmt.Errorf("LLM returned no choices")
		}
		assistantMessage := response.Choices[0].Message

		if err := s.AppendTurn(assistantMessage); err != nil {
			return agora.NodeResult{State: s}, fmt.Errorf("could not append turn to history: %w", err)
		}

		// 6. Check for tool calls and update state
		if len(assistantMessage.ToolCalls) > 0 {
			// If tools were called, we store them in state to be executed by ToolExecutorNode
			s.Set("tool_calls", assistantMessage.ToolCalls)
			// Clear any previous output since we are in a tool calling loop
			s.Set("output", "")
		} else {
			// Normal response
			s.Set("output", assistantMessage.Content)
		}

		return agora.NodeResult{State: s}, nil
	}
}

// coerceToolCalls returns the typed tool calls stored in state.
//
// tool_calls are stored as []agora.ToolCall, but State.DeepCopy is a JSON
// round-trip (see ConversationState.DeepCopy), so on a forked state the
// value arrives as []interface{} of maps. Re-encoding through JSON restores
// the typed slice; anything that cannot decode into []agora.ToolCall is
// genuinely malformed and reported as an error.
func coerceToolCalls(data any) ([]agora.ToolCall, error) {
	if calls, ok := data.([]agora.ToolCall); ok {
		return calls, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("cannot re-encode tool calls for coercion: %w", err)
	}
	var calls []agora.ToolCall
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil, fmt.Errorf("cannot decode tool calls: %w", err)
	}
	return calls, nil
}

// ToolExecutorNode creates a NodeFunc that executes tool calls found in the state.
func ToolExecutorNode(registry agora.ToolRegistry) agora.NodeFunc {
	return func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		// 1. Check for tool calls in state.
		toolCallsData := s.Get("tool_calls")
		if toolCallsData == nil {
			// No tool calls to process, return early.
			return agora.NodeResult{State: s}, nil
		}

		// 2. Coerce the data back to a slice of ToolCall. A direct type
		// assertion covers the in-memory path; after a DeepCopy (JSON
		// round-trip, e.g. a ParallelNode fork) the stored slice degrades to
		// generic JSON shapes, so a forked state must be re-cast rather than
		// rejected — tool-calling and parallelism are not mutually exclusive.
		toolCalls, err := coerceToolCalls(toolCallsData)
		if err != nil {
			return agora.NodeResult{State: s}, fmt.Errorf("invalid tool calls format in state: %w", err)
		}

		if len(toolCalls) == 0 {
			return agora.NodeResult{State: s}, nil
		}

		// 3. Execute each tool call.
		for _, call := range toolCalls {
			tool, exists := registry[call.Function.Name]
			var resultStr string

			if !exists {
				resultStr = fmt.Sprintf("Error: Tool '%s' not found", call.Function.Name)
			} else {
				// Execute the tool
				result, err := tool.Execute(ctx, call.Function.Arguments)
				if err != nil {
					resultStr = fmt.Sprintf("Error executing tool '%s': %v", call.Function.Name, err)
				} else {
					// Marshal success result
					resultBytes, jsonErr := json.Marshal(result)
					if jsonErr != nil {
						resultStr = fmt.Sprintf(`{"error": "failed to marshal tool result to JSON: %s"}`, jsonErr.Error())
					} else {
						resultStr = string(resultBytes)
					}
				}
			}

			// 4. Create a tool role message with the result.
			toolResponseMessage := agora.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    resultStr,
			}

			// 5. Append the result to the state's history.
			if err := s.AppendTurn(toolResponseMessage); err != nil {
				return agora.NodeResult{State: s}, fmt.Errorf("could not append tool response to history: %w", err)
			}
		}

		// 6. Clear the processed tool calls from the state.
		s.Set("tool_calls", nil)

		// 7. Return the updated state.
		return agora.NodeResult{State: s}, nil
	}
}
