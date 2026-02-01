# DOC-02: The Compiler Specification v1.1

## 1\. Project Philosophy

* The Compiler's directive is to be **invisible, infallible, and trustworthy**. It must transmute visual design into a high-performance, testable, and human-readable Go project.

---

## 2\. Core Components & Process Flow

The `agora-cli compile` command orchestrates a three-stage pipeline:

### Stage 1: The Pre-Processor (The Blueprint Inspector)

* **Input:** A bundle of JSON files from the Foundry (`graph.json`, `state.json`, etc.).  
* **Action:** Rigorously validates the entire blueprint for logical consistency. Fails fast with clear errors if any component is missing or misconfigured.

### Stage 2: The Generator (The Code & Test Factory)

* **Input:** The validated blueprint.  
* **Action:** Programmatically writes a complete, idiomatic Go project to a temporary directory with a clean, modular structure.  
* **Project Structure Generated:**

/agora-project  
├── main.go               // The main entry point based on the chosen Trigger  
├── go.mod                // Generated go module file  
├── graph.go              // Contains the generated code to build the Graph struct  
├── state.go              // The user-defined State struct  
├── /tools/               // Directory for tool definitions  
│   ├── tool\_one.go  
│   └── tool\_two.go  
├── /functions/           // Directory for custom Go functions used by tools  
│   ├── func\_one.go  
│   └── func\_two.go  
├── /skills/              // Directory for instructional Recipes  
│   ├── index.md          // An index of available skills  
│   ├── skill\_one.md  
│   └── skill\_two.md  
└── /subgraphs/           // Directory for reusable Appliances  
│   └── subgraph\_one.json

* **Automated Test Generation:** This is the critical step for trustworthiness. The Generator will also create `_test.go` files.  
1. **`graph_test.go`:** Generates tests to ensure the graph builds correctly and nodes are connected as expected.  
2. **`tools_test.go`:** Generates basic tests to ensure tool functions can be called.  
3. **LLM Mocking:** Crucially, for any test requiring an LLM, the generator will create and use a **`MockLLM`**. This mock implementation of our `LLM` interface will return static, predictable `ModelResponse` data, allowing for fast, deterministic, and free testing of the agent's logic without making any real API calls.

### Stage 3: The Builder (The Forge)

* **Input:** The generated and tested Go project.  
* **Action:** Invokes the appropriate build toolchain based on the chosen **Export Target** (Binary, Dockerfile, Kubernetes Manifests).

---

