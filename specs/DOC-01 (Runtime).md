# DOC-01: The Agora Runtime Specification v1.1

## 1\. Project Philosophy & Manifesto

* **Problem:** The AI revolution is leaving production-focused Go developers behind, forcing them to use Python tools that are ill-suited for high-performance, concurrent, and reliable systems.  
* **Promise:** Agora is the industrial-grade, open-source agentic framework for Go. It allows architects and systems engineers to build, run, and deploy complex, stateful multi-agent systems with the performance, safety, and concurrency native to Go. We are not the first mover; we are the last, most robust mover.

---

## 2\. Core Data Structures (\`agora/\` package)

* **`Graph` struct:** The primary container for an agentic system.

    type Graph struct {  
        Nodes            map\[string\]NodeFunc  
        Edges            map\[string\]string // Maps a node to the \*next\* node for simple transitions  
        ConditionalEdges map\[string\]func(s State) string // Maps a node to a function that dynamically chooses the next node  
        Entry            string  
        MaxSteps         int // A circuit-breaker to prevent infinite loops. Defaults to 25\.  
    }

* **`State` interface:** The "soul" or "memory" of the graph. It is passed between every node.

    type State interface {  
        // Implementations will vary, but must provide methods for history and data management.  
    }

* **`NodeFunc` type:** The signature for any function that can act as a node in the graph.

    `type NodeFunc func(ctx context.Context, s State) (NodeResult, error)`

* **`NodeResult` struct:** The output of a `NodeFunc`, instructing the `Execute` loop on how to proceed.

    type NodeResult struct {  
        State    State  
        NextNode string // The name of the next node to execute. An empty string can signify the end.  
        IsDone   bool   // An explicit flag to halt execution.  
    }

---

## 3\. Execution & Control Flow (agora/ package)

* **`Graph.Execute()` method:** The primary entry point for running a graph.

    `func (g *Graph) Execute(ctx context.Context, initialState State) (State, error)`

* **Execution Loop Logic:**  
1. The loop starts at the `Graph.Entry` node.  
2. It MUST check `ctx.Done()` and the `MaxSteps` circuit breaker on every iteration.  
3. It executes the current node's `NodeFunc`.  
4. It updates the `State` with the `NodeResult.State`.  
5. It determines the next node by checking `ConditionalEdges` first, then `Edges`.  
6. The loop terminates if `NodeResult.IsDone` is true or if the next node name is empty.

---

## 4\. Core Interfaces & Standard Library

### 4.1. The LLM Interface (llm/ package)

The generic, flexible interface for any language model provider.

type ModelRequest struct {  
    Messages \[\]ChatMessage  
    Tools    \[\]ToolDefinition // Optional tool definitions for the model  
    // Other future params: Temperature, ResponseFormat, etc.  
}

type ModelResponse struct {  
    Content   string  
    ToolCalls \[\]ToolCall // Structured tool calls requested by the model  
}

type LLM interface {  
    Invoke(ctx context.Context, request ModelRequest) (ModelResponse, error)  
}

### 4.2. Required LLM Implementations (llm/ package)

The framework must ship with the following clients implementing the `LLM` interface:

* **`OllamaLLM`:** For connecting to a local Ollama server.  
* **`GoogleStudioLLM`:** For connecting to Google AI Studio.  
* **`OpenAICompatibleLLM`:** For connecting to any OpenAI-compliant API.

### 4.3. The Standard Node Library (nodes/ package)

A set of pre-built, reusable `NodeFunc` factories for common agentic patterns.

* **`SimpleAgentNode(llm LLM, systemPrompt string) NodeFunc`**  
  * **Purpose:** A basic "thinking" node for conversational tasks.  
  * **Logic:** Constructs a `ModelRequest`, calls `llm.Invoke()`, and appends the text response to the state.

* **`ToolAgentNode(llm LLM, systemPrompt string, tools []ToolDefinition) NodeFunc`**  
  * **Purpose:** The core "reasoning" node that decides when to use tools.  
  * **Logic:** Sends tools in the `ModelRequest`. If the `ModelResponse` contains `ToolCalls`, it adds them to the state for the `ToolExecutorNode` to handle.

* **`ToolExecutorNode(registry ToolRegistry) NodeFunc`**  
  * **Purpose:** The "action" node that executes tool calls.  
  * **Logic:** Scans the state for pending `ToolCalls`, executes the corresponding functions from the registry, and appends the results to the state.

* **`SubGraphNode(subGraph *Graph) NodeFunc`**  
  * **Purpose:** The "composability" node for hierarchical agent design.  
  * **Logic:** Executes an entire `subGraph` as a single step within a parent graph.

---

## 5\. Definition of Done for Runtime v1.1

* The Agora library can be imported to programmatically build and execute a graph.  
* The graph can reliably demonstrate a full ReAct (Reason-Act) loop using the `ToolAgentNode` and `ToolExecutorNode`.  
* The system can be composed hierarchically using the `SubGraphNode`.  
* Execution can be interrupted via context.  
* `Context` and is protected from infinite loops by `MaxSteps`.  
* A developer has a choice of at least three different LLM backends (Ollama, Google, OpenAI-compatible) through the generic `LLM` interface.  
* The codebase is organized into logical packages (`agora`, `llm`, `nodes`) and all public APIs are documented with GoDoc.