package nodes

import (
	"context"
	"fmt"
	"sync"

	"github.com/amangsingh/agora"
)

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
func ParallelNode(nodesToRun []agora.NodeFunc, mergeFunc func(originalState agora.State, resultingStates []agora.State) agora.State) agora.NodeFunc {
	return func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		var wg sync.WaitGroup
		// A channel to collect the final state from each successful goroutine.
		resultsChan := make(chan agora.State, len(nodesToRun))
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
				return agora.NodeResult{}, fmt.Errorf("failed to deep copy state for parallel execution: %w", err)
			}

			go func(nf agora.NodeFunc, st agora.State) {
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
			return agora.NodeResult{}, <-errChan
		}

		// Fan-in: Collect all the resulting states from the successful branches.
		var resultingStates []agora.State
		for res := range resultsChan {
			resultingStates = append(resultingStates, res)
		}

		// Merge: Use the user-provided function to merge the results.
		// The `s` here is the original, pre-parallel state.
		mergedState := mergeFunc(s, resultingStates)

		// Return the single, unified state. The graph continues from here.
		return agora.NodeResult{State: mergedState}, nil
	}
}
