package llm

import (
	"context"

	"github.com/amangsingh/agora"
)

// LLM defines the interface for Large Language Model providers.
// It follows the AGORA strict standard: Invoke(ctx, request) -> (ModelResponse, error).
type LLM interface {
	Invoke(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error)
}
