// Package agora provides an industrial-grade, concurrent, and stateful agentic framework for Go.
//
// Agora is designed for systems engineers who require strict type safety, compilation checks,
// and zero-dependency architectures. Unlike Python-based frameworks (LangChain), Agora
// compiles your agentic logic into a single, highly performant binary.
//
// Core Components:
//
//   - Graph: The orchestration engine. It manages the flow of execution between nodes.
//     It enforces a "Circuit Breaker" (MaxSteps) to prevent infinite loops (The Molt Vector).
//
//   - State: The thread-safe memory of the agent. It is immutable during parallel execution
//     (via DeepCopy) to prevent race conditions.
//
//   - Nodes: The functional units of logic. A NodeFunc receives the current state and
//     returns a NodeResult indicating the next step.
//
// Usage:
//
//	g := agora.NewGraph()
//	g.AddNode("agent", myAgentNode)
//	g.AddNode("tools", myToolNode)
//	g.SetEntry("agent")
//	g.AddEdge("tools", "agent")
//	g.AddConditionalEdge("agent", deciderFunc)
//
//	finalState, err := g.Execute(ctx, initialState)
package agora
