# DOC-00: AGORA TECH STACK & STYLE GUIDE v1.0

## 1. THE TECH STACK (Industrial-Grade)
* **Language:** Go 1.24+ (Latest Stable)
* **CLI Framework:** `cobra` (The standard for Go CLIs like kubectl/docker) + `viper` (Config).
* **Logging:** `log/slog` (Stdlib structured logging) OR `uber-go/zap` (If extreme perf is needed). *Decision: Use `slog` for zero-dependency purity.*
* **Testing:** Standard `testing` package + `stretchr/testify` (Assertions/Mocks).
* **Concurrency:** Native Goroutines + `golang.org/x/sync/errgroup` (Safe group execution).
* **LLM Client:** Custom `net/http` implementations (Zero 3rd party bloat).

## 2. THE STYLE GUIDE (The Uber-Go Derivative)
* **Error Handling:**
    * **Rule:** Errors are values. Panic is forbidden.
    * **Pattern:** `if err != nil { return fmt.Errorf("context: %w", err) }`
    * **Wrap:** Always wrap errors when bubbling up.
* **Interfaces:**
    * **Rule:** Accept Interfaces, Return Structs.
    * **Naming:** One method = `AgentDoer`. Two methods = `AgentExecutor`.
* **Concurrency:**
    * **Rule:** Zero-value Mutexes are valid.
    * **Rule:** Channels are for signaling, Mutexes are for state.
* **Project Layout:**
    * `/cmd`: Main entry points (`agora-cli`, `agora-server`).
    * `/pkg`: Public library code (The Runtime).
    * `/internal`: Private logic (Security, Resolvers).
    * `/examples`: Reference implementations.

## 3. THE "MOLT-PROOF" SECURITY STANDARD
* **No `os.Exit`:** The library must never kill the host process. Return error.
* **No `init()` side-effects:** Explicit initialization only.
* **Context Required:** Every blocking function signature must start with `ctx context.Context`.