// in agora/agora.go

package agora

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type ChatMessage struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
	Role             string `json:"role"`
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
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
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

// State is the single, persistent object that is passed between all nodes
// in a graph. It carries the full context of the execution
type State struct {
	// Starting with a simple map for flexibility, allowing any node to read
	// or write data without changing the core struct.
	Values map[string]any
}

// NodeResult is what a node returns after it executes. It contains the
// updated state and instructions for the graph runner.
type NodeResult struct {
	State State
	NextNode string
	IsDone bool
}

// NodeFunc is the signature for any function that can act as a node
// in our graph. It's a function that takes a context and a state,
// and returns a noderesult and an error.
type NodeFunc func(ctx context.Context, s State) (NodeResult, error)

// Graph is a container for our entire agentic system. It holds all
// the nodes and defines the entry point.
type Graph struct {
	Nodes map[string]NodeFunc
	Entry string
}

// The Run function sends the user's ChatMessage to the model and returns the response
// in a proper ModelResponse parameter
func Run(ctx context.Context, endpointURL string, payload ModelRequest) (*ModelResponse, error) {
	// 1. convert the payload to JSON bytes
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse payload: %w", err)
	}

	// 2. create the http request.
	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("Failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer dummy-value") // hardcoding for now

	// 3. Run the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Failed to execute http request: %w", err)
	}

	// 4. check response status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Received non-200 status: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	// Create an empty response struct
	var finalResponse ModelResponse

	// 5. Decode the response and take action based on streamin/non-streaming
	if payload.Stream {
		isFirstChunk := true

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// Check if the stream is done, break the loop
			if line == "data: [DONE]" {
				break
			}

			// Continue if the string doesn't have the data prefix
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			// trim the prefix and get json data
			jsonData := strings.TrimPrefix(line, "data: ")

			var chunkResponse ModelResponse
			err := json.Unmarshal([]byte(jsonData), &chunkResponse)
			if err != nil {
				log.Printf("Could not unmarshal stream chunk: %v", err)
				continue
			}

			// Initialize our final response on first chunk
			if isFirstChunk {
				isFirstChunk = false
				finalResponse.Created = chunkResponse.Created
				finalResponse.Id = chunkResponse.Id
				finalResponse.Model = chunkResponse.Model
				finalResponse.Object = chunkResponse.Object
				finalResponse.SystemFingerprint = chunkResponse.SystemFingerprint
				finalResponse.Timings = chunkResponse.Timings
				finalResponse.Usage = chunkResponse.Usage
				finalResponse.Choices = make([]Choice, 1)
				finalResponse.Choices[0].Message.Role = "assistant"
			}

			// append delta from chunk to final response's
			if len(chunkResponse.Choices) > 0 {
				delta := chunkResponse.Choices[0].Delta
				finalResponse.Choices[0].Message.Content += delta.Content
				finalResponse.Choices[0].Message.ReasoningContent += delta.Content
				finalResponse.Choices[0].FinishReason = chunkResponse.Choices[0].FinishReason
			}

			// If the final chunk has timings, copy them over
			if chunkResponse.Timings.PredictedN > 0 {
				finalResponse.Timings = chunkResponse.Timings
			}
		}

		return &finalResponse, nil
	} else {
		err = json.NewDecoder(resp.Body).Decode(&finalResponse)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode response: %w", err)
		}

		// output the data for now
		return &finalResponse, nil
	}
}

// Execute runs the graph from its entry point until a node signals completion.
func (g *Graph) Execute(ctx context.Context, initialState State) (State, error) {
	// 1. Set the starting node from the graph's entry point. and set state
	currentNodeName := g.Entry
	state := initialState

	// 2. Start a loop that will run as long as we have a valid next node.
	for {
		// 3. Inside the loop, find the node function in the map.
		node, exists := g.Nodes[currentNodeName]
		if !exists {
			return state, fmt.Errorf("Could not find the node %s in the Graph!", currentNodeName)
		}

		// 4. Execute the node with the current state
		response, err := node(ctx, state)
		if err != nil {
			return state, fmt.Errorf("Trouble executing node: %w", err)
		}

		// 5. Update the state for the next iteration
		state = response.State

		// 6. Check if the node signaled that the graph is done.
		if response.IsDone {
			break
		}

		// 7. Update the current node name for the next iteration
		currentNodeName = response.NextNode
	}

	// 8. Return the final state
	return state, nil
}
