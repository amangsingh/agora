# Agora: The Industrial-Grade AI Framework for Go

[![Go Report Card](https://goreportcard.com/badge/github.com/amangsingh/agora)](https://goreportcard.com/report/github.com/amangsingh/agora)
[![Build Status](https://img.shields.io/badge/build-passing-brightgreen)](https://github.com/amangsingh/agora/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/amangsingh/agora.svg)](https://pkg.go.dev/github.com/amangsingh/agora)

**Agora** is a compiled, type-safe, and zero-dependency framework for building complex AI agents.

It is built for **Systems Engineers**, not Scripters. It rejects the fragility of interpreted "chains" in favor of strict, compiled **Graphs**.

## ⚡️ Why Agora?

| Feature | Python Frameworks (LangChain/CrewAI) | Agora (Go) |
| :--- | :--- | :--- |
| **Runtime** | Interpreted (Slow, Fragile) | **Compiled (Binary, Fast)** |
| **Concurrency** | GIL (Global Interpreter Lock) | **Goroutines (Native Parallelism)** |
| **Safety** | Runtime Errors (Surprise!) | **Compile-Time Checks** |
| **Deployment** | 4GB Docker Image (Pip Hell) | **15MB Static Binary** |
| **Security** | Opaque "Black Boxes" | **Zero Trust / Audit Ready** |

## 🚀 Quick Start

### Installation
```bash
go get github.com/amangsingh/agora
```

### The "Hello World" Agent

Unlike other frameworks, Agora requires you to define your architecture explicitly.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/llm"
	"github.com/amangsingh/agora/nodes"
)

func main() {
	ctx := context.Background()

	// 1. Initialize the Graph
	g := agora.NewGraph()
	g.MaxSteps = 10 // Safety Circuit Breaker

	// 2. Define Components
	// Note: You would likely use NewOpenAICompatibleLLM or similar here
	// model := llm.NewOpenAICompatibleLLM("http://localhost:11434/v1", "llama3", "ollama")
	
	// This example assumes a hypothetical constructor for brevity logic or mock.
	// Check the llm package for precise constructors.
	model := llm.NewOllamaLLM("http://localhost:11434/v1", "llama3")
	agent := nodes.SimpleAgentNode(model, "You are a helpful assistant.")

	// 3. Build the Architecture
	g.AddNode("agent", agent)
	g.SetEntry("agent")
	// g.AddEdge("agent", "END") // Or rely on empty NextNode to finish

	// 4. Execute
	// Initialize strict conversation state
	initialState := &agora.ConversationState{
		BaseState: agora.NewBaseState(),
		Input:     "Hello, Agora!",
	}
	
	finalState, err := g.Execute(ctx, initialState)
	if err != nil {
		log.Fatalf("Execution failed: %v", err)
	}
	
	// Type assert back to access conversation history
	finalConv := finalState.(*agora.ConversationState)
	if len(finalConv.History) > 0 {
		fmt.Println("Agent Reply:", finalConv.History[len(finalConv.History)-1].Content)
	}
}
```

## ⚠️ Migration Notice (v4.0)

Agora v4.0 introduces a **strict architectural rewrite**.

* **Global State** is removed.
* **Nodes** are now stateless functions.
* **MaxSteps** is mandatory.
Legacy v3 code is **incompatible**. Please consult the migration guide.

## 📜 License

MIT
