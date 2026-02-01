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

---

## 🛠️ The Compiler (CLI)

Agora v4.0 introduces the **Compiler** (`agora-cli`), a toolchain that transforms YAML blueprints into industrial-grade Go code.

### Installation

```bash
go install github.com/amangsingh/agora/cmd/agora-cli@latest
```

### Zero to Agent: The Workflow

1.  **Initialize**: Scaffold a new project.
    ```bash
    agora-cli init my-agent
    cd my-agent
    ```

2.  **Blueprint**: Edit the `agora.yaml` file to define your architecture.
    ```yaml
    project: my-agent
    version: 1.0.0

    graph:
      entry: research_agent
      max_steps: 10

    nodes:
      - name: research_agent
        type: agent
        model: llama3
        instructions: "You are a senior researcher. Summarize the user's input."

    edges:
      - from: research_agent
        to: END
    ```

3.  **Compile**: Generate the Go source code.
    ```bash
    agora-cli generate
    ```

4.  **Run**: Execute your binary agent.
    ```bash
    go mod tidy
    go run .
    ```

### The Blueprint Schema (`agora.yaml`)

The blueprint is the source of truth for your agent's topology.

```yaml
# Project Metadata
project: secure-agent
version: 0.1.0

# Runtime Constraints
graph:
  entry: start_node
  max_steps: 25  # Circuit breaker for infinite loops

# Component Definitions
nodes:
  - name: start_node
    type: agent
    model: gpt-4o
    instructions: "Route the user to the correct tool."

  - name: tool_executor
    type: tool_node
    tools: ["file_reader", "http_client"]

# Control Flow (The Graph)
edges:
  - from: start_node
    to: tool_executor
  - from: tool_executor
    to: start_node  # Feedback loop
```

---

## 📚 The Runtime (Library)

If you prefer to write Go code manually, Agora provides a clean, zero-dependency library.

### Quick Start

```bash
go get github.com/amangsingh/agora
```

### Manual Implementation Example

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
	model := llm.NewOllamaLLM("http://localhost:11434/v1", "llama3")
	agent := nodes.SimpleAgentNode(model, "You are a helpful assistant.")

	// 3. Build the Architecture
	g.AddNode("agent", agent)
	g.SetEntry("agent")

	// 4. Execute
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


---

## 🏛️ The Sovereign Server (API)

For production deployment, Agora provides a specialized REST API server (`agora-server`).

> [!WARNING]
> **Security Notice**: This server is designed to be exposed. **Do NOT disable Authentication.**

### Configuration
The server is configured via Environment Variables:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | Listening Port | `8080` |
| `AGORA_DB` | SQLite Database Path | `./agora.db` |
| `AGORA_AUTH_TOKEN` | **REQUIRED** Bearer Token | *(None)* |

### Deployment Guide

```bash
# 1. Build
go build -o agora-server cmd/agora-server/main.go

# 2. Configure & Run
export AGORA_AUTH_TOKEN="super-secret-key-change-me"
./agora-server
```

### API Reference

#### 1. Execute Agent
`POST /run`

Executes the agent graph with the provided input.

**Headers:**
`Authorization: Bearer <AGORA_AUTH_TOKEN>`

**Payload:**
```json
{
  "input": "Summarize the latest logs.",
  "model": "llama3" 
}
```

**Response:**
```json
{
  "execution_id": "a1b2c3d4",
  "status": "completed",
  "output": "Here is the summary..."
}
```

#### 2. Get History
`GET /history?execution_id=<id>`

Retrieves the chat logs for a specific execution.

---

## ⚠️ Migration Notice (v4.0)

Agora v4.0 introduces a **strict architectural rewrite**.

* **Global State** is removed.
* **Nodes** are now stateless functions.
* **MaxSteps** is mandatory.
* **Compiler** is the recommended way to start new projects.

Legacy v3 code is **incompatible**.

## 📜 License

MIT
