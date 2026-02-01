package llm

import (
	"context"
	"fmt"

	"github.com/amangsingh/agora"
)

// GoogleStudioLLM is a placeholder implementation for Google AI Studio.
// This enforces the interface contract but currently returns a 'Not Implemented' error.
// TODO: Implement actual Google AI Studio API call.
type GoogleStudioLLM struct {
	APIKey string
	Model  string
}

// NewGoogleStudioLLM creates a new instance.
func NewGoogleStudioLLM(apiKey, model string) *GoogleStudioLLM {
	return &GoogleStudioLLM{
		APIKey: apiKey,
		Model:  model,
	}
}

// Invoke implements the LLM interface.
func (l *GoogleStudioLLM) Invoke(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
	return agora.ModelResponse{}, fmt.Errorf("GoogleStudioLLM is not yet implemented")
}
