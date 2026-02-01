package nodes

import (
	"context"

	"github.com/amangsingh/agora"
)

// SubGraphNode creates a NodeFunc that executes an entire sub-graph as a single step.
// This is the core mechanism for hierarchical agent composition.
func SubGraphNode(subGraph *agora.Graph) agora.NodeFunc {
	return func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		// 1. Execute the provided sub-graph, passing it the current state.
		// This is a blocking call; the parent graph waits for the sub-graph to finish.
		finalStateFromSubGraph, err := subGraph.Execute(ctx, s)
		if err != nil {
			// If the sub-graph fails, propagate the error up.
			return agora.NodeResult{State: s}, err
		}

		// 2. The execution was successful. The final state of the sub-graph
		// now becomes the new state of the parent graph.
		return agora.NodeResult{
			State: finalStateFromSubGraph,
		}, nil
	}
}
