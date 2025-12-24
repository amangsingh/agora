// in llm_openai.go

package agora

import (
	"context"
	"fmt"
)

// OpenAICompatibleLLM is a concrete implementation of the LLM interface
// for any backend that mimics the OpenAI Chat Completions API.
type OpenAICompatibleLLM struct {
	BaseURL   string
	ModelName string
}

// NewOpenAICompatibleLLM creates a new instance of the LLM.
func NewOpenAICompatibleLLM(baseURL, model string) *OpenAICompatibleLLM {
	return &OpenAICompatibleLLM{
		BaseURL:   baseURL,
		ModelName: model,
	}
}

// Chat implements the LLM interface.
func (l *OpenAICompatibleLLM) Chat(ctx context.Context, messages []ChatMessage) (ChatMessage, error) {
	// 1. Create the ModelRequest payload using l.ModelName and the incoming messages.
	payload := ModelRequest{
		Model:    l.ModelName,
		Messages: messages,
	}

	// 2. Create the openAI like completions url
	completionsURL := l.BaseURL + "/chat/completions"

	// 3. Call the agora.Run function we built earlier, passing it the payload
	// For now we'll call it in non-streaming mode.
	response, err := Run(ctx, completionsURL, payload)

	// 4. Handle the error from agora.Run
	if err != nil {
		return ChatMessage{}, fmt.Errorf("Trouble executing model call: %w", err)
	}

	// 5. If successful, extract the first ChatMessage from the response's Choices.
	responseMessage := response.Choices[0].Message

	// 6. Return the extracted ChatMessage and a nil error
	return responseMessage, nil
}
