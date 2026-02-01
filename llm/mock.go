package llm

import (
	"context"

	"github.com/amangsingh/agora"
)

// MockLLM is a mock implementation of the LLM interface for testing.
// It uses a manual function hook pattern to avoid external dependencies.
type MockLLM struct {
	InvokeFunc func(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error)
}

// Invoke implements the LLM interface.
func (m *MockLLM) Invoke(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
	if m.InvokeFunc != nil {
		return m.InvokeFunc(ctx, request)
	}
	// Default empty return
	return agora.ModelResponse{}, nil
}
