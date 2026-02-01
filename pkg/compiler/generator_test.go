package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompile_GeneratesFiles(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "build")

	yamlContent := `
project: gen-test
version: 0.1.0
graph:
  entry: agent
  max_steps: 5
nodes:
  - name: agent
    type: agent
    model: llama3
edges:
  - from: agent
    to: END
`
	blueprintPath := filepath.Join(tmpDir, "agora.yaml")
	if err := os.WriteFile(blueprintPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Execute
	err := Compile(blueprintPath, outDir)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	// Verify Files Existance
	expectedFiles := []string{
		"main.go",
		"go.mod",
		"graph.go",
		"state.go",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(outDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected generated file %s not found", f)
		}
	}
}
