// in agora/tools.go

package agora

import (
	"context"
)

// Tool represents a function that can be called by an agent.
// It's a contract for any capability we want to add.
type Tool interface {
	// Definition returns the schema that the LLM needs to understand
	// how and when to call this tool.
	Definition() ToolDefinition

	// Execute runs the actual logic of the tool with the arguments
	// provided by the LLM.
	Execute(ctx context.Context, args map[string]interface{}) (any, error)
}

// ToolRegistry is a simple map to hold all available tools, keyed by their name.
type ToolRegistry map[string]Tool

// NewToolRegistry creates and empty registry.
func NewToolRegistry() ToolRegistry {
	return make(ToolRegistry)
}

// Register adds a new tool to the registry.
func (r ToolRegistry) Register(tool Tool) {
	// We get the name from the tool's own definition
	name := tool.Definition().Function.Name
	r[name] = tool
}

// RegisterAll adds multiple tools to the registry at once.
func (r ToolRegistry) RegisterAll(tools ...Tool) {
	for _, tool := range tools {
		r.Register(tool)
	}
}

// GetDefinitions extracts the definitions of all registered tools.
func (r ToolRegistry) GetDefinitions() []ToolDefinition {
	definitions := make([]ToolDefinition, 0, len(r))
	for _, tool := range r {
		definitions = append(definitions, tool.Definition())
	}
	return definitions
}
