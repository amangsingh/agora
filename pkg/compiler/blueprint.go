package compiler

// Blueprint represents the unmarshalled YAML configuration.
type Blueprint struct {
	Project string      `yaml:"project"`
	Version string      `yaml:"version"`
	Graph   GraphConfig `yaml:"graph"`
	Nodes   []NodeGen   `yaml:"nodes"`
	Edges   []EdgeGen   `yaml:"edges"`
}

type GraphConfig struct {
	Entry    string `yaml:"entry"`
	MaxSteps int    `yaml:"max_steps"`
}

type NodeGen struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"` // "agent", "tool", "subgraph"
	Model        string   `yaml:"model,omitempty"`
	Instructions string   `yaml:"instructions,omitempty"` // For agents
	Tools        []string `yaml:"tools,omitempty"`        // List of tool names
}

type EdgeGen struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}
