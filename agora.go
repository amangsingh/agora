// in agora/agora.go

package agora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrMaxStepsExceeded is returned when the graph execution exceeds the defined MaxSteps.
var ErrMaxStepsExceeded = errors.New("execution exceeded max steps")

// ToolCall represents the LLM's request to call a specific tool.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	} `json:"function"`
}

// ChatMessage defines the universal format for conversational turns
type ChatMessage struct {
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content"`
	Role             string     `json:"role"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

type Choice struct {
	FinishReason string      `json:"finish_reason"`
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message,omitempty"`
	Delta        ChatMessage `json:"delta,omitempty"`
}

type Timings struct {
	CacheN             int     `json:"cache_n"`
	PredictedMs        float32 `json:"predicted_ms"`
	PredictedN         int     `json:"predicted_n"`
	PredictedPerSecond float32 `json:"predicted_per_second"`
	PromptMs           float32 `json:"prompt_ms"`
	PromptN            int     `json:"prompt_n"`
	PromptPerSecond    float32 `json:"prompt_per_second"`
	PromptPerTokenMs   float32 `json:"prompt_per_token_ms"`
}

type Usage struct {
	CompletionTokens int `json:"completion_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ModelRequest struct {
	Model      string           `json:"model"`
	Messages   []ChatMessage    `json:"messages"`
	Stream     bool             `json:"stream,omitempty"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"`
}

type ModelResponse struct {
	Choices           []Choice `json:"choices"`
	Created           int      `json:"created"`
	Id                string   `json:"id"`
	Model             string   `json:"model"`
	Object            string   `json:"object"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Timings           Timings  `json:"timings"`
	Usage             Usage    `json:"usage"`
}

// NodeResult is what a node returns after it executes. It contains the
// updated state and instructions for the graph runner.
type NodeResult struct {
	State    State
	NextNode string // Targeted jump to a specific node
	IsDone   bool   // Logic to signal strictly that we are done
}

// NodeFunc is the signature for any function that can act as a node
// in our graph. It's a function that takes a context and a state,
// and returns a noderesult and an error.
type NodeFunc func(ctx context.Context, s State) (NodeResult, error)

// Graph is a container for our entire agentic system. It holds all
// the nodes and defines the entry point.
type Graph struct {
	Nodes            map[string]NodeFunc
	Edges            map[string]string
	ConditionalEdges map[string]func(s State) string
	Entry            string
	MaxSteps         int // Circuit breaker defaults to 25
}

// ToolDefinition defines the structure for a tool that can be used by agents.
type ToolDefinition struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function represents the actual function declaration for the tool
type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// A constructor to get a graph
func NewGraph() *Graph {
	return &Graph{
		Nodes:            make(map[string]NodeFunc),
		Edges:            make(map[string]string),
		ConditionalEdges: make(map[string]func(s State) string),
		MaxSteps:         25, // Default as per Spec
	}
}

// --- Builder Methods ---
// Add Node
func (g *Graph) AddNode(name string, node NodeFunc) {
	g.Nodes[name] = node
}

// Add an edge
func (g *Graph) AddEdge(source, target string) {
	g.Edges[source] = target
}

// Setting a conditional edge
func (g *Graph) SetConditionalEdge(sourceNode string, logic func(s State) string) {
	g.ConditionalEdges[sourceNode] = logic
}

// Set entry point
func (g *Graph) SetEntry(name string) {
	g.Entry = name
}

// The Run function sends the user's ChatMessage to the model and returns the response
// in a proper ModelResponse parameter
func Run(ctx context.Context, endpointURL string, payload ModelRequest, token string) (*ModelResponse, error) {
	// 1. convert the payload to JSON bytes
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	// 2. create the http request.
	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token) // hardcoding for now

	// 3. Run the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute http request: %w", err)
	}
	defer resp.Body.Close()

	// 4. check response status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("received non-200 status: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	// Create an empty response struct
	var finalResponse ModelResponse

	// 5. Decode the response and take action based on streamin/non-streaming
	err = json.NewDecoder(resp.Body).Decode(&finalResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// output the data for now
	return &finalResponse, nil
}

// Execute runs the graph from its entry point until a node signals completion.
func (g *Graph) Execute(ctx context.Context, initialState State) (State, error) {
	currentNodeName := g.Entry
	state := initialState
	steps := 0

	for {
		// 1. Strict Context Check
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		default:
		}

		// 2. Strict MaxSteps Check
		if steps >= g.MaxSteps {
			return state, ErrMaxStepsExceeded
		}
		steps++

		// 3. Check for End of Execution via END magic string or empty
		if currentNodeName == "END" || currentNodeName == "" {
			return state, nil
		}

		// 4. Get and Execute Node
		node, exists := g.Nodes[currentNodeName]
		if !exists {
			return state, fmt.Errorf("node %s not found", currentNodeName)
		}

		response, err := node(ctx, state)
		if err != nil {
			return state, fmt.Errorf("error executing node %s: %w", currentNodeName, err)
		}

		// Update state
		state = response.State

		// 5. Navigation Logic
		// Priority 1: If NodeResult says IsDone, we stop immediately.
		if response.IsDone {
			return state, nil
		}

		// Priority 2: If NodeResult provides a specific NextNode, we go there.
		if response.NextNode != "" {
			currentNodeName = response.NextNode
			continue
		}

		// Priority 3: Conditional Edges
		if logic, exists := g.ConditionalEdges[currentNodeName]; exists {
			currentNodeName = logic(state)
			continue
		}

		// Priority 4: Static Edges
		if nextNode, exists := g.Edges[currentNodeName]; exists {
			currentNodeName = nextNode
			continue
		}

		// Priority 5: No path found implies implicit termination
		break
	}

	return state, nil
}
