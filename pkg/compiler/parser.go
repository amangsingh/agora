package compiler

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// ParseBlueprint reads a YAML file and unmarshals it into a Blueprint struct.
// It also performs basic validation.
func ParseBlueprint(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read blueprint file: %w", err)
	}

	var bp Blueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := validate(&bp); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &bp, nil
}

func validate(bp *Blueprint) error {
	if bp.Project == "" {
		return fmt.Errorf("project name is required")
	}
	if bp.Graph.Entry == "" {
		return fmt.Errorf("graph entry node is required")
	}

	// Validate Nodes
	nodeMap := make(map[string]bool)
	identifierRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	for _, n := range bp.Nodes {
		if n.Name == "" {
			return fmt.Errorf("node name cannot be empty")
		}
		if !identifierRegex.MatchString(n.Name) {
			return fmt.Errorf("node name '%s' is invalid: must be a valid Go identifier (alphanumeric/underscore)", n.Name)
		}
		if nodeMap[n.Name] {
			return fmt.Errorf("duplicate node name: %s", n.Name)
		}
		nodeMap[n.Name] = true
	}

	// Validate Edges
	for _, e := range bp.Edges {
		if !nodeMap[e.From] && e.From != "START" { // Allow START/END if we decide to use them, though specs say Entry field.
			return fmt.Errorf("edge source '%s' does not exist", e.From)
		}
		if !nodeMap[e.To] && e.To != "END" {
			return fmt.Errorf("edge target '%s' does not exist", e.To)
		}
	}

	// Validate Entry
	if !nodeMap[bp.Graph.Entry] {
		return fmt.Errorf("entry node '%s' does not exist", bp.Graph.Entry)
	}

	return nil
}
