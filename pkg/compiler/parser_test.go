package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBlueprint_Valid(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	yamlContent := `
project: test-project
version: 1.0.0
graph:
  entry: agent
  max_steps: 10
nodes:
  - name: agent
    type: agent
    model: llama3
    instructions: "test instructions"
edges:
  - from: agent
    to: END
`
	path := filepath.Join(tmpDir, "valid.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Execute
	bp, err := ParseBlueprint(path)

	// Verify
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if bp.Project != "test-project" {
		t.Errorf("expected project 'test-project', got '%s'", bp.Project)
	}
	if bp.Graph.MaxSteps != 10 {
		t.Errorf("expected max_steps 10, got %d", bp.Graph.MaxSteps)
	}
	if len(bp.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(bp.Nodes))
	}
}

func TestParseBlueprint_Invalid(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	// Missing "entry" in graph config
	yamlContent := `
project: invalid-project
graph:
  max_steps: 10
nodes:
  - name: agent
    type: agent
`
	path := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Execute
	_, err := ParseBlueprint(path)

	// Verify
	if err == nil {
		t.Fatal("expected validation error (missing entry), got nil")
	}
}

func TestParseBlueprint_InvalidName(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	// Invalid node name "my-agent" (contains dash)
	yamlContent := `
project: invalid-name
graph:
  entry: agent
  max_steps: 10
nodes:
  - name: my-agent
    type: agent
`
	path := filepath.Join(tmpDir, "invalid_name.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Execute
	_, err := ParseBlueprint(path)

	// Verify
	if err == nil {
		t.Fatal("expected validation error (invalid node name), got nil")
	}
}
