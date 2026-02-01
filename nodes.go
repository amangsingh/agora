// in agora/nodes.go (a new file)

package agora

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// SimpleAgentNode is a factory that creates a NodeFunc for a basic agent.
// It takes an LLM implementation and uses it to respond to user input.
func SimpleAgentNode(llm LLM, instructions string) NodeFunc {
	return func(ctx context.Context, s State) (NodeResult, error) {
		// 1. Get the full conversation for the LLM call.
		// The user's ToChatHistory() implementation is responsible for
		// including the current input along with the past history.
		messagesForLLM, err := s.ToChatHistory()
		if err != nil {
			return NodeResult{}, fmt.Errorf("Could not get chat history: %w", err)
		}

		// 2. Prepend the system instructions.
		messagesForLLM = append([]ChatMessage{
			{Role: "system", Content: instructions},
		}, messagesForLLM...)

		request := ModelRequest{
			Messages: messagesForLLM,
		}

		// 3. Call the LLM
		response, err := llm.Chat(ctx, request)
		if err != nil {
			return NodeResult{}, fmt.Errorf("Failed to call LLM: %w", err)
		}

		// 4. Set the direct output for this turn.
		s.Set("output", response.Content)

		// 5. Append the AI's response to the state's history using the new contract.
		if err := s.AppendTurn(response); err != nil {
			return NodeResult{}, fmt.Errorf("Could not append turn to history: %w", err)
		}

		// 6. Return, letting the graph runner decide the next step.
		return NodeResult{
			State: s,
		}, nil
	}
}

// ToolAgentNode is a factory for an agent that can use tools.
func ToolAgentNode(llm LLM, instructions string, registry ToolRegistry) NodeFunc {
	return func(ctx context.Context, s State) (NodeResult, error) {
		// 1. get history
		messagesForLLM, err := s.ToChatHistory()
		if err != nil {
			return NodeResult{}, fmt.Errorf("Could not get chat history: %w", err)
		}

		// 2. Prepend the system instructions.
		messagesForLLM = append([]ChatMessage{
			{Role: "system", Content: instructions},
		}, messagesForLLM...)

		// 3. Create the ModelRequest
		request := ModelRequest{
			Messages:   messagesForLLM,
			Tools:      registry.GetDefinitions(),
			ToolChoice: "auto",
		}

		// 4. Call the LLM
		response, err := llm.Chat(ctx, request)
		if err != nil {
			return NodeResult{}, fmt.Errorf("Failed to call LLM: %w", err)
		}

		// debug print
		responseBytes, _ := json.MarshalIndent(response, "", "  ")
		fmt.Printf("LLM Response: %s\n", string(responseBytes))

		// 5. append thoughts to history
		if err := s.AppendTurn(response); err != nil {
			return NodeResult{}, fmt.Errorf("Could not append turn to history: %w", err)
		}

		// 6. check for tool calls and update state
		if len(response.ToolCalls) > 0 {
			s.Set("tool_calls", response.ToolCalls)
			s.Set("output", "")
		} else {
			s.Set("output", response.Content)
		}

		return NodeResult{State: s}, nil
	}
}

// ToolExecutorNode creates a NodeFunc that executes tool calls found in the state.
func ToolExecutorNode(registry ToolRegistry) NodeFunc {
	return func(ctx context.Context, s State) (NodeResult, error) {
		// 1. Check for tool calls in state.
		toolCallsData := s.Get("tool_calls")
		if toolCallsData == nil {
			// No tool calls to process, return early.
			return NodeResult{State: s}, nil
		}

		// 2. Safely cast the data to a slice of ToolCall.
		toolCalls, ok := toolCallsData.([]ToolCall)
		if !ok {
			return NodeResult{}, fmt.Errorf("Invalid tool calls format")
		}
		if len(toolCalls) == 0 {
			// No tool calls to process, return early.
			return NodeResult{State: s}, nil
		}

		// 3. Execute each tool call.
		for _, call := range toolCalls {
			tool, exists := registry[call.Function.Name]
			var resultStr string

			if !exists {
				resultStr = fmt.Sprintf("Error: Tool '%s' not found", call.Function.Name)
			} else {
				// if tool exists, execute it.
				result, err := tool.Execute(ctx, call.Function.Arguments)
				if err != nil {
					resultStr = fmt.Sprintf("Error executing tool '%s': %v", call.Function.Name, err)
				} else {
					// On success, marshal the result to a JSON
					resultBytes, jsonErr := json.Marshal(result)
					if jsonErr != nil {
						resultStr = fmt.Sprintf(`{"error": "failed to marshal tool result to JSON: %s"}`, jsonErr.Error())
					} else {
						resultStr = string(resultBytes)
					}
				}
			}

			// 4. Create a tool role message with the result.
			toolResponseMessage := ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    resultStr,
			}

			// 5. Append the result to the state's history for the agent to see.
			if err := s.AppendTurn(toolResponseMessage); err != nil {
				return NodeResult{}, fmt.Errorf("Could not append tool response to history: %w", err)
			}
		}

		// 6. Clear the processed tool calls from the state.
		s.Set("tool_calls", nil)

		// 7. Return the updated state.
		return NodeResult{State: s}, nil
	}
}

// SubGraphNode creates a NodeFunc that executes an entire sub-graph as a single step.
// This is the core mechanism for hierarchical agent composition.
func SubGraphNode(subGraph *Graph) NodeFunc {
	return func(ctx context.Context, s State) (NodeResult, error) {
		// 1. Execute the provided sub-graph, passing it the current state.
		// This is a blocking call; the parent graph waits for the sub-graph to finish.
		finalStateFromSubGraph, err := subGraph.Execute(ctx, s)
		if err != nil {
			// If the sub-graph fails, propagate the error up.
			return NodeResult{}, err
		}

		// 2. The execution was successful. The final state of the sub-graph
		// now becomes the new state of the parent graph.
		// For now, we do a simple replacement. V2 could have more complex "merge" strategies.
		return NodeResult{
			State: finalStateFromSubGraph,
		}, nil
	}
}

// ParallelNode executes multiple NodeFuncs in parallel.
//
// It works by creating a deep copy of the state for each parallel branch,
// ensuring complete isolation. After all branches have completed, it uses a
// user-provided `mergeFunc` to combine the results from all branches back
// into a single, final state.
//
// This is the core mechanism for concurrent agent execution. The individual
// nodes to run can be SimpleAgentNodes, ToolAgentNodes, or even SubGraphNodes,
// allowing for incredibly complex parallel workflows.
func ParallelNode(nodesToRun []NodeFunc, mergeFunc func(originalState State, resultingStates []State) State) NodeFunc {
	return func(ctx context.Context, s State) (NodeResult, error) {
		var wg sync.WaitGroup
		// A channel to collect the final state from each successful goroutine.
		resultsChan := make(chan State, len(nodesToRun))
		// A channel to collect any errors.
		errChan := make(chan error, len(nodesToRun))

		// Fan-out: Launch a goroutine for each node to be run in parallel.
		for _, nodeFunc := range nodesToRun {
			wg.Add(1)

			// CRITICAL: Create a deep copy of the state for each goroutine.
			// This is the heart of the "State Isolation" strategy.
			stateCopy, err := s.DeepCopy()
			if err != nil {
				// If we can't even copy the state, we can't proceed.
				return NodeResult{}, fmt.Errorf("failed to deep copy state for parallel execution: %w", err)
			}

			go func(nf NodeFunc, st State) {
				defer wg.Done()
				// Execute the node with its private copy of the state.
				// This node can be a single step or an entire sub-graph.
				result, err := nf(ctx, st)
				if err != nil {
					errChan <- err
					return
				}
				// On success, send the final state of this branch to the results channel.
				resultsChan <- result.State
			}(nodeFunc, stateCopy)
		}

		// Wait for all the parallel branches to complete.
		wg.Wait()
		close(resultsChan)
		close(errChan)

		// Check if any of the branches returned an error.
		if len(errChan) > 0 {
			// For now, we return the first error we find.
			// V2 could collect and return all errors.
			return NodeResult{}, <-errChan
		}

		// Fan-in: Collect all the resulting states from the successful branches.
		var resultingStates []State
		for res := range resultsChan {
			resultingStates = append(resultingStates, res)
		}

		// Merge: Use the user-provided function to merge the results.
		// The `s` here is the original, pre-parallel state.
		mergedState := mergeFunc(s, resultingStates)

		// Return the single, unified state. The graph continues from here.
		return NodeResult{State: mergedState}, nil
	}
}
