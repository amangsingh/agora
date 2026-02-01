package llm

import (
	"context"
	"fmt"

	"github.com/amangsingh/agora"
)

// OpenAICompatibleLLM is a concrete implementation of the LLM interface
// for any backend that mimics the OpenAI Chat Completions API.
type OpenAICompatibleLLM struct {
	BaseURL   string
	ModelName string
	Token     string
}

// NewOpenAICompatibleLLM creates a new instance of the LLM.
func NewOpenAICompatibleLLM(baseURL, model, token string) *OpenAICompatibleLLM {
	return &OpenAICompatibleLLM{
		BaseURL:   baseURL,
		ModelName: model,
		Token:     token,
	}
}

// Invoke implements the LLM interface by calling the OpenAI Chat Completions API.
func (l *OpenAICompatibleLLM) Invoke(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
	// 1. Get the model name
	request.Model = l.ModelName

	// 2. Create the openAI like completions url
	completionsURL := l.BaseURL + "/chat/completions"

	// 3. Call the agora.Run function, passing it the payload
	// Note: We are using the public 'Run' helper from the agora package.
	responsePtr, err := agora.Run(ctx, completionsURL, request, l.Token)
	if err != nil {
		return agora.ModelResponse{}, fmt.Errorf("trouble executing model call: %w", err)
	}

	if responsePtr == nil {
		return agora.ModelResponse{}, fmt.Errorf("received nil response from core runtime")
	}

	return *responsePtr, nil
}
