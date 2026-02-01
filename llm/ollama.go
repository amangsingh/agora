package llm

import (
	"context"
	"os"

	"github.com/amangsingh/agora"
)

// OllamaLLM is a concrete implementation of the LLM interface for Ollama.
// It effectively wraps the OpenAICompatibleLLM since Ollama provides an OAI-compat endpoint.
type OllamaLLM struct {
	Client *OpenAICompatibleLLM
}

// NewOllamaLLM creates a new instance of the Ollama LLM.
// It defaults to "http://localhost:11434/v1" if base URL is not provided.
// It reads the model name either from the arg or ENV "OLLAMA_MODEL".
func NewOllamaLLM(baseURL string, model string) *OllamaLLM {
	if baseURL == "" {
		baseURL = "http://localhost:11434/v1"
	}
	if model == "" {
		model = os.Getenv("OLLAMA_MODEL")
		if model == "" {
			model = "llama3" // Sane default
		}
	}
	return &OllamaLLM{
		Client: NewOpenAICompatibleLLM(baseURL, model, "ollama"),
	}
}

// Invoke implements the LLM interface.
func (l *OllamaLLM) Invoke(ctx context.Context, request agora.ModelRequest) (agora.ModelResponse, error) {
	return l.Client.Invoke(ctx, request)
}
