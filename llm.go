// in llm.go

package agora

import "context"

type LLM interface {
	// For now we keep it simple. It takes a message and returns a single message
	// streaming to be added later
	Chat(ctx context.Context, messages []ChatMessage) (ChatMessage, error)
}
