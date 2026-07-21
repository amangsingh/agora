package compiler

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestCompile_OutputCompiles is the repo's eye: it proves the generated
// project actually COMPILES, instead of merely existing on disk. File
// existence (os.Stat, as asserted above) is not evidence of a working
// compiler — the generator can, and today does, emit an unbuildable module
// while the suite reports green.
//
// Mechanism: every generated .go file is run through go/parser (syntax
// gate), then `go vet` runs over the generated module as a whole (package
// loading + type checking). The vet step is hermetic by construction:
// GOPROXY=off makes network module fetches structurally impossible, so the
// result is deterministic on any machine where this repo's own tests run.
// If the generated module can only be built by downloading something, that
// is a failure this test is designed to surface, not an inconvenience to
// route around.
//
// Three arms, all vetting the module EXACTLY AS EMITTED (Step 9 made the
// emission self-resolving, so no arm needs test-side go.mod surgery anymore):
//
//   - ModuleResolution: historically red because the generated go.mod
//     required github.com/amangsingh/agora v0.0.0 — a version published
//     nowhere — with no replace directive. Step 9's generateGoMod
//     materializes the replace at generation time; this arm keeps vetting
//     the module as emitted so the resolution can never regress.
//
//   - ToolNodeGraph: a blueprint shaped like the readme's tool_node example.
//     Historically red because generateGraph's agent-only guard emitted no
//     code for tool nodes while still emitting their imports and inbound
//     edges (unused imports / unused declarations). Step 9 emits tool nodes
//     wired from the T bank; this arm keeps that emission honest.
//
//   - AgentGraph: agent-only blueprint — the happy-path emission must
//     type-check end to end.
//
// This test failed while the generator defects were present (red at a42a428
// on the first two arms). It is the standing gate that keeps them from
// drifting back now that Step 9 closed them.
func TestCompile_OutputCompiles(t *testing.T) {
	// Shaped after the documented example in readme.md (tool_node section):
	// an agent routing to a tool node with a feedback loop.
	const mixedBlueprint = `
project: gen-compile-mixed
version: 0.1.0
graph:
  entry: start_node
  max_steps: 5
nodes:
  - name: start_node
    type: agent
    model: llama3
    instructions: "Route the user to the correct tool."
  - name: tool_executor
    type: tool_node
    tools: ["file_reader", "http_client"]
edges:
  - from: start_node
    to: tool_executor
  - from: tool_executor
    to: start_node
`

	const toolOnlyBlueprint = `
project: gen-compile-tool
version: 0.1.0
graph:
  entry: tool_executor
  max_steps: 5
nodes:
  - name: tool_executor
    type: tool_node
    tools: ["file_reader"]
edges:
  - from: tool_executor
    to: END
`

	const agentOnlyBlueprint = `
project: gen-compile-agent
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

	cases := []struct {
		name      string
		blueprint string
	}{
		{name: "ModuleResolution", blueprint: mixedBlueprint},
		{name: "ToolNodeGraph", blueprint: toolOnlyBlueprint},
		{name: "AgentGraph", blueprint: agentOnlyBlueprint},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outDir := generateProject(t, tc.blueprint)
			parseGeneratedGoFiles(t, outDir)
			vetGeneratedModule(t, outDir)
		})
	}
}

// generateProject writes the blueprint to a temp dir, runs Compile, and
// returns the output directory containing the generated module.
func generateProject(t *testing.T, blueprint string) string {
	t.Helper()
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "build")
	blueprintPath := filepath.Join(tmpDir, "agora.yaml")
	if err := os.WriteFile(blueprintPath, []byte(blueprint), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Compile(blueprintPath, outDir); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	return outDir
}

// parseGeneratedGoFiles runs every generated .go file through go/parser and
// fails the test on any syntax error. Existence is never asserted: an output
// directory with zero parseable Go files is itself a failure.
func parseGeneratedGoFiles(t *testing.T, outDir string) {
	t.Helper()
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("failed to read output dir: %v", err)
	}
	fset := token.NewFileSet()
	parsed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if _, err := parser.ParseFile(fset, filepath.Join(outDir, e.Name()), nil, parser.AllErrors); err != nil {
			t.Errorf("generated file %s does not parse:\n%v", e.Name(), err)
		}
		parsed++
	}
	if parsed == 0 {
		t.Fatal("generation produced no .go files to check")
	}
}

// vetGeneratedModule compiles and vets the generated module and fails the
// test on any error — module resolution, type checking, or vet analysis.
//
// Two passes, both fully offline:
//   - `go build ./...` — the compiler proper. Unlike vet (which halts at
//     the first type error during package loading), build reports every
//     type error per package, so the failure output names each defect in
//     the emitted code rather than only the first one encountered.
//   - `go vet ./...` — includes _test.go files in the generated module and
//     adds vet's static analyzers on top of compilation.
//
// GOPROXY=off forbids network access entirely — module downloads are
// structurally impossible, not merely avoided — so both passes are
// deterministic on any machine where this repo's own tests run: the
// generated module must resolve and compile from local source and the
// local module cache alone (the same cache this repo's build requires).
func vetGeneratedModule(t *testing.T, outDir string) {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go toolchain not found: %v", err)
	}
	hermeticEnv := append(os.Environ(),
		"GOPROXY=off",      // hermetic: module downloads are impossible, not just avoided
		"GOFLAGS=-mod=mod", // allow go to record resolved requirements in the temp module
		"GOWORK=off",       // insulate from any ambient workspace file
		"GOSUMDB=off",      // no checksum-db lookups (network) for cache-resolved modules
	)
	for _, pass := range [][]string{
		{"build", "./..."},
		{"vet", "./..."},
	} {
		cmd := exec.Command(goBin, pass...)
		cmd.Dir = outDir
		cmd.Env = hermeticEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("generated project fails `go %s` — the emitted module does not compile:\n%s", pass[0], out)
		}
	}
}
