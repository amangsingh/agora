// in agora/state.go

package agora

import (
	"encoding/json"
	"fmt"

	"github.com/mitchellh/mapstructure"
)

// State is the fundamental contract for any object that wishes to be
// managed by the Agora graph. It must know how to act as a generic key-value
// store and how to represent itself as a chat history
type State interface {
	Get(key string) any
	Set(key string, value any)

	// ToChatHistory translates the *entire current state* into the list of
	// messages needed for an LLM call. The user is responsible for
	// including both past history and the current input in this list.
	ToChatHistory() ([]ChatMessage, error)

	// AppendTurn takes the result of an LLM call and updates the
	// state's history. The user is responsible for the implementation,
	// whether that's appending to a slice, writing to a database, etc.
	AppendTurn(output ChatMessage) error

	// Giving the ability to replicate itself
	DeepCopy() (State, error)
}

// BaseState provides a default, embeddable implementation of the state interface.
// It uses a map internally for dynamic storage and relies on the user to
// implement the domain-specific ToChatHistory method.
type BaseState struct {
	Values map[string]any
}

// NewBaseState creates an initialized BaseState. A user's custom struct
// must call this to initialize its embedded BaseState.
func NewBaseState() BaseState {
	return BaseState{Values: make(map[string]any)}
}

// Get retrieves a value from the internal state map.
func (s *BaseState) Get(key string) any {
	return s.Values[key]
}

// Set places a value into the internal state map
func (s *BaseState) Set(key string, value any) {
	s.Values[key] = value
}

// Sync uses reflection to decode the fields of a user's struct (data)
// into the internal map. This is the magic that allows direct field access
// on the user's side and generic map access on the framework's side.
func (s *BaseState) Sync(data any) error {
	return mapstructure.Decode(data, &s.Values)
}

// Decode does the reverse of Sync, populating the fields of a user's struct
// (target) from the values in the internal map.
func (s *BaseState) Decode(target any) error {
	return mapstructure.Decode(s.Values, target)
}

// ConversationState is a helper struct that provides a default, embeddable
// implementation for common chat-based agents. A user can embed this
// in their own state struct to get standard history management for free.
type ConversationState struct {
	BaseState `mapstructure:",squash"`
	History   []ChatMessage `mapstructure:"history"`
	Input     string        `mapstructure:"input"`
}

// ToChatHistory fulfills the State interface, providing the full list of
// messages for an LLM call by combining the past History with the current Input.
// This function creates a temporary slice and does NOT mutate the state.
func (s *ConversationState) ToChatHistory() ([]ChatMessage, error) {
	return append(s.History, ChatMessage{Role: "user", Content: s.Input}), nil
}

// AppendTurn fulfills the State interface, persisting the completed turn
// into the History. This function MUTATES the state's History slice.
// It is responsible for saving both the user's prompt and the AI's response.
func (s *ConversationState) AppendTurn(output ChatMessage) error {
	s.History = append(s.History, ChatMessage{Role: "user", Content: s.Input}, output)
	return nil
}

// This method fulfills the DeepCopy contract for ConversationState.
func (s *ConversationState) DeepCopy() (State, error) {
	// By marshaling and unmarshaling the concrete type, we create a
	// perfect, deep copy of the entire struct, including all its fields.
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ConversationState for deep copy: %w", err)
	}

	var newState ConversationState
	err = json.Unmarshal(bytes, &newState)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal into new ConversationState: %w", err)
	}

	// We return a pointer to the new struct, which satisfies the State interface.
	return &newState, nil
}
