package compiler

// Step 9 (task qze8vx4easlbogyz): the compiler emits a RESIDENT — generated
// main constructs a Self FROM THE BLUEPRINT, starts R (the Hum), and serves.
//
// RED BASELINE (captured at 5157691 before this step landed):
//   - AC1/AC2/AC6/AC7/AC8: the generated module did not even BUILD as
//     emitted (go.mod required the unpublished agora v0.0.0 with no replace
//     directive), so no resident could be run; and the emitted main was a
//     run-once script (construct state → Execute → print → exit).
//   - AC3: generateMain discarded the blueprint (`_ = bp`, generator.go:161)
//     — every blueprint pressed the SAME main.go, byte for byte.
//   - AC5a: generateGraph hard-coded llm.NewOllamaLLM("http://localhost:11434/v1", …)
//     (generator.go:359) — the declared L member's provider/model/base_url
//     never reached the emitted graph.
//   - AC5b (SEC F1): blueprint strings were interpolated raw into quoted Go
//     literals — a quote/backslash/newline in a declaration broke (or
//     rewrote) the structure of the emitted source.
//
// These tests run the GENERATED BINARY where the criterion is behavioral:
// liveness, the anti-cron discriminator re-run inside the generated process,
// membrane authn, tool invocation, and dying well on SIGTERM.

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/amangsingh/agora/pkg/storage"
)

// residentBlueprint is the canonical Step 9 blueprint: an agent routing to a
// tool node, a distinctive L member (an unroutable base_url on purpose — a
// test resident must never reach a real model), a declared M field, a sqlite
// D bank, a rest ingress on an OS-assigned port, and R enabled.
const residentBlueprint = `
project: gen-resident
version: 0.1.0
self:
  id: resident-self
  name: The Resident
graph:
  entry: converse
  max_steps: 5
nodes:
  - name: converse
    type: agent
    model: fast
    instructions: "Converse."
  - name: tool_executor
    type: tool_node
    tools: ["file_reader", "http_client"]
edges:
  - from: converse
    to: tool_executor
  - from: tool_executor
    to: END
models:
  - name: fast
    provider: ollama
    model: llama3
    base_url: "http://127.0.0.1:1"
tools:
  - name: file_reader
    description: "Reads files."
state:
  - name: episodes_seen
    type: int
memory:
  kind: sqlite
  dsn: ./resident.db
endpoints:
  ingress:
    - name: api
      kind: rest
      addr: "127.0.0.1:0"
rhythm:
  enabled: true
`

// hermeticResidentEnv mirrors vetGeneratedModule's hermetic environment:
// module downloads are structurally impossible, so the generated module must
// build and run exactly as emitted, from local source and cache alone.
func hermeticResidentEnv() []string {
	return append(os.Environ(),
		"GOPROXY=off",
		"GOFLAGS=-mod=mod",
		"GOWORK=off",
		"GOSUMDB=off",
	)
}

// buildResident generates a project from the blueprint and compiles it AS
// EMITTED (no test-side go.mod surgery). The returned binary is the resident
// under test.
func buildResident(t *testing.T, blueprint string) (outDir, binPath string) {
	t.Helper()
	outDir = generateProject(t, blueprint)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go toolchain not found: %v", err)
	}
	binPath = filepath.Join(outDir, "resident-under-test")
	cmd := exec.Command(goBin, "build", "-o", binPath, ".")
	cmd.Dir = outDir
	cmd.Env = hermeticResidentEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated resident does not build as emitted:\n%s", out)
	}
	return outDir, binPath
}

// lineBuffer is a threadsafe sink for the resident's combined output.
type lineBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *lineBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *lineBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// waitFor blocks until the resident's output contains substr, failing the
// test on deadline. It returns the full output seen so far.
func (l *lineBuffer) waitFor(t *testing.T, substr string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s := l.String(); strings.Contains(s, substr) {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("resident output never contained %q within %s; output so far:\n%s", substr, timeout, l.String())
	return ""
}

// startResident launches the generated binary with no args and no input —
// the AC1 posture — and hands back the process plus its output stream.
func startResident(t *testing.T, binPath, dir string, extraEnv ...string) (*exec.Cmd, *lineBuffer) {
	t.Helper()
	out := &lineBuffer{}
	cmd := exec.Command(binPath)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Env = append(os.Environ(), extraEnv...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("generated resident failed to start: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	return cmd, out
}

// terminateResident sends SIGTERM and waits for exit, returning the exit
// code and whether the process exited within the deadline.
func terminateResident(t *testing.T, cmd *exec.Cmd, timeout time.Duration) (code int, exited bool) {
	t.Helper()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to signal resident: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		return cmd.ProcessState.ExitCode(), true
	case <-time.After(timeout):
		return -1, false
	}
}

// residentServingAddr extracts the actual ingress address from the log
// (the blueprint declares an OS-assigned port).
var servingAddrRe = regexp.MustCompile(`resident: serving ingress on (127\.0\.0\.1:\d+)`)

func residentServingAddr(t *testing.T, out *lineBuffer) string {
	t.Helper()
	logs := out.waitFor(t, "resident: serving ingress on", 30*time.Second)
	m := servingAddrRe.FindStringSubmatch(logs)
	if m == nil {
		t.Fatalf("could not extract serving address from resident output:\n%s", logs)
	}
	return m[1]
}

// TestResident_AC1_AC2_LivesAndHums (AC1 + AC2): the generated binary, no
// args and no input, stays alive past N seconds while the Hum observably
// advances — the named signal is the RESIDENT_PROBE log line, which carries
// the iteration counter and re-runs Step 5's anti-cron discriminator INSIDE
// the generated process: unpersisted in-memory working state survives >=3
// iterations, and the D store never held it.
func TestResident_AC1_AC2_LivesAndHums(t *testing.T) {
	outDir, bin := buildResident(t, residentBlueprint)
	dbPath := filepath.Join(outDir, "resident.db")
	cmd, out := startResident(t, bin, outDir,
		"AGORA_RESIDENT_PROBE=1",
		"AGORA_HUM_INTERVAL=20ms",
		"AGORA_DB="+dbPath,
	)

	residentServingAddr(t, out)
	started := time.Now()

	// AC2: the discriminator's verdict, from inside the generated process.
	logs := out.waitFor(t, "RESIDENT_PROBE anticron", 30*time.Second)
	if !strings.Contains(logs, "RESIDENT_PROBE anticron survived=true") {
		t.Errorf("AC2 FAIL: unpersisted in-memory working state did not survive inside the generated resident:\n%s", logs)
	}
	m := regexp.MustCompile(`iterations_now=(\d+)`).FindStringSubmatch(logs)
	if m == nil {
		t.Fatalf("probe line carries no iteration counter:\n%s", logs)
	}
	iterations, _ := strconv.Atoi(m[1])
	if iterations < 3 {
		t.Errorf("AC1 FAIL: Hum advanced only %d iterations (want >= 3)", iterations)
	}

	// AC1: alive past N seconds (N=2) with zero input.
	if wait := 2*time.Second - time.Since(started); wait > 0 {
		time.Sleep(wait)
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("AC1 FAIL: resident is not alive after 2s: %v\noutput:\n%s", err, out.String())
	}

	// Shut down, then prove the residue was genuinely unpersisted: D must
	// have no trace of it (survival via storage would be cron on a
	// technicality — hum_test.go's own bar).
	if code, exited := terminateResident(t, cmd, 10*time.Second); !exited || code != 0 {
		t.Fatalf("resident did not exit cleanly (exited=%t code=%d)", exited, code)
	}
	db, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("resident left no D store at %s: %v", dbPath, err)
	}
	if bytes.Contains(db, []byte("unpersisted-residue-resident")) {
		t.Errorf("AC2 FAIL: the probe residue leaked into the D store — the discriminator requires state that storage never had")
	}
}

// TestResident_AC6_ToolsInvocable (AC6): the declared tool nodes exist in
// the generated resident and their tools are invocable — behaviorally, in
// the running process (the probe invokes each declared tool and logs the
// result), not merely by compilation.
func TestResident_AC6_ToolsInvocable(t *testing.T) {
	outDir, bin := buildResident(t, residentBlueprint)
	_, out := startResident(t, bin, outDir,
		"AGORA_RESIDENT_PROBE=1",
		"AGORA_HUM_INTERVAL=20ms",
		"AGORA_DB="+filepath.Join(outDir, "resident.db"),
	)

	// Both the T-bank-declared tool and the node-referenced bare tool.
	for _, tool := range []string{"file_reader", "http_client"} {
		logs := out.waitFor(t, "RESIDENT_PROBE tool="+tool, 30*time.Second)
		if !strings.Contains(logs, "RESIDENT_PROBE tool="+tool+" invoked=true") {
			t.Errorf("AC6 FAIL: declared tool %q was not invocable in the running resident:\n%s", tool, logs)
		}
	}
}

// TestResident_AC6_GeneratedSuiteGreen (AC6 supplement): the generated
// module's own test suite — which exercises the tool node through
// nodes.ToolExecutorNode against the generated registry — passes as emitted.
func TestResident_AC6_GeneratedSuiteGreen(t *testing.T) {
	outDir := generateProject(t, residentBlueprint)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go toolchain not found: %v", err)
	}
	cmd := exec.Command(goBin, "test", "./...")
	cmd.Dir = outDir
	cmd.Env = hermeticResidentEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated module's own test suite fails:\n%s", out)
	}
}

// TestResident_AC7_MembraneAuthn (AC7): the emitted membrane carries the SEC
// gate posture — a request addressing an EXISTING self without (or with a
// wrong) credential is rejected, asserted against the running generated
// binary. Recall stays bounded by construction: the emitted main constructs
// the Self through ConstructSelf with no RecallWindow override, so the D
// read path is MostRecentN over DefaultRecallWindow (recall.go), never
// unbounded.
func TestResident_AC7_MembraneAuthn(t *testing.T) {
	outDir, bin := buildResident(t, residentBlueprint)
	_, out := startResident(t, bin, outDir,
		"AGORA_HUM_INTERVAL=20ms",
		"AGORA_DB="+filepath.Join(outDir, "resident.db"),
	)
	addr := residentServingAddr(t, out)
	base := "http://" + addr
	client := &http.Client{Timeout: 10 * time.Second}

	// Establish a self: creation and credential minting are one act.
	resp, err := client.Post(base+"/selves", "application/json",
		strings.NewReader(`{"self_id":"guarded-self"}`))
	if err != nil {
		t.Fatalf("POST /selves failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /selves: got status %d, want 201", resp.StatusCode)
	}
	var established struct {
		SelfID     string `json:"self_id"`
		Credential string `json:"credential"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&established); err != nil {
		t.Fatalf("POST /selves: undecodable response: %v", err)
	}
	if established.Credential == "" {
		t.Fatal("POST /selves minted no credential")
	}

	// Unauthenticated addressing of the existing self: rejected.
	for _, tc := range []struct {
		name       string
		credential string
	}{
		{name: "missing credential", credential: ""},
		{name: "wrong credential", credential: "not-the-credential"},
	} {
		req, err := http.NewRequest(http.MethodPost, base+"/run",
			strings.NewReader(`{"input":"hello","self_id":"guarded-self"}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if tc.credential != "" {
			req.Header.Set("X-Agora-Self-Credential", tc.credential)
		}
		got, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s: POST /run failed: %v", tc.name, err)
		}
		got.Body.Close()
		if got.StatusCode != http.StatusUnauthorized {
			t.Errorf("AC7 FAIL (%s): addressing an existing self got status %d, want 401", tc.name, got.StatusCode)
		}
	}
}

// TestResident_AC8_DiesWell (AC8): SIGTERM stops the resident cleanly — exit
// code 0 within the deadline, the clean-shutdown line on the log (the Hum's
// goroutine is joined by Stop before that line prints), and a D store that
// reopens and migrates without error (no half-written D).
func TestResident_AC8_DiesWell(t *testing.T) {
	outDir, bin := buildResident(t, residentBlueprint)
	dbPath := filepath.Join(outDir, "resident.db")
	cmd, out := startResident(t, bin, outDir,
		"AGORA_HUM_INTERVAL=20ms",
		"AGORA_DB="+dbPath,
	)
	residentServingAddr(t, out)

	code, exited := terminateResident(t, cmd, 10*time.Second)
	if !exited {
		t.Fatalf("AC8 FAIL: resident did not exit within 10s of SIGTERM; output:\n%s", out.String())
	}
	if code != 0 {
		t.Errorf("AC8 FAIL: resident exited with code %d after SIGTERM, want 0; output:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "resident: hum stopped cleanly; dying well") {
		t.Errorf("AC8 FAIL: no clean-shutdown line on the log; output:\n%s", out.String())
	}
	// No half-written D: the store must reopen and migrate cleanly.
	if _, err := storage.NewRepository(dbPath); err != nil {
		t.Errorf("AC8 FAIL: D store does not reopen cleanly after SIGTERM: %v", err)
	}
}

// TestGenerateMain_AC3_BlueprintDerived (AC3): two blueprints differing in a
// load-bearing declaration (the L member's base_url — where the mind's
// compute lives) emit observably different residents. RED pre-change:
// generateMain discarded bp, so every blueprint pressed an identical main.go.
func TestGenerateMain_AC3_BlueprintDerived(t *testing.T) {
	const otherURL = "http://127.0.0.2:1"
	variant := strings.Replace(residentBlueprint, "http://127.0.0.1:1", otherURL, 1)

	dirA := generateProject(t, residentBlueprint)
	dirB := generateProject(t, variant)

	mainA, err := os.ReadFile(filepath.Join(dirA, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	mainB, err := os.ReadFile(filepath.Join(dirB, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(mainA, mainB) {
		t.Errorf("AC3 FAIL: two blueprints differing in a load-bearing L declaration emitted byte-identical main.go — the emitted main does not derive from the blueprint")
	}

	graphA, err := os.ReadFile(filepath.Join(dirA, "graph.go"))
	if err != nil {
		t.Fatal(err)
	}
	graphB, err := os.ReadFile(filepath.Join(dirB, "graph.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(graphA), "http://127.0.0.1:1") {
		t.Errorf("AC3 FAIL: emitted graph does not carry blueprint A's declared base_url")
	}
	if !strings.Contains(string(graphB), otherURL) {
		t.Errorf("AC3 FAIL: emitted graph does not carry blueprint B's declared base_url")
	}
}

// TestGeneratedGraph_AC5a_CarriesDeclaredLBank (AC5a): the emitted graph
// resolves a node's model through the DECLARED L member — provider, model,
// base_url — not the hard-coded localhost Ollama default. RED pre-change at
// generator.go:359.
func TestGeneratedGraph_AC5a_CarriesDeclaredLBank(t *testing.T) {
	outDir := generateProject(t, residentBlueprint)
	graph, err := os.ReadFile(filepath.Join(outDir, "graph.go"))
	if err != nil {
		t.Fatal(err)
	}
	want := `newDeclaredModel("ollama", "llama3", "http://127.0.0.1:1")`
	if !strings.Contains(string(graph), want) {
		t.Errorf("AC5a FAIL: emitted graph does not construct the node's model from the declared L member.\nwant fragment: %s\ngot graph.go:\n%s", want, graph)
	}
	if strings.Contains(string(graph), `llm.NewOllamaLLM("http://localhost:11434/v1"`) {
		t.Errorf("AC5a FAIL: emitted graph still hard-codes the localhost Ollama default at the node site")
	}
}

// nastyInstructions carries every F1 escape class at once: double quotes,
// backslashes, a newline, and a tab.
const nastyInstructions = "She said \"hello\" \\ twice\nand meant it\ttruly"

// escapeBlueprint declares strings that MUST arrive in the emitted source as
// data. Built as a struct (not YAML) so the test bytes are unambiguous.
func escapeBlueprint() *Blueprint {
	return &Blueprint{
		Project: "gen-escape",
		Version: "0.1.0",
		Graph:   GraphConfig{Entry: "converse", MaxSteps: 3},
		Nodes: []NodeGen{
			{Name: "converse", Type: "agent", Model: "fast", Instructions: nastyInstructions},
			{Name: "tool_executor", Type: "tool_node", Tools: []string{"file_reader"}},
		},
		Edges: []EdgeGen{
			{From: "converse", To: "tool_executor"},
			{From: "tool_executor", To: "END"},
		},
		Models: []ModelDecl{
			{Name: "fast", Provider: "ollama", Model: "llama3", BaseURL: "http://127.0.0.1:1"},
		},
		Tools: []ToolDecl{
			{Name: "file_reader", Description: "backslash \\ and \"quote\" and\nnewline"},
		},
	}
}

// astStringLiterals returns every string literal in a Go source file,
// unquoted — the emitted file's DATA, as the compiler would see it.
func astStringLiterals(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("emitted file %s does not parse: %v", path, err)
	}
	var literals []string
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				literals = append(literals, s)
			}
		}
		return true
	})
	return literals
}

// TestGeneratedSource_AC5b_EscapesBlueprintStrings (AC5b / SEC F1):
// blueprint strings containing quotes, backslashes, newlines and tabs are
// emitted as DATA — the generated project still parses and compiles, and
// every string round-trips intact through the emitted source. RED
// pre-change: raw interpolation into quoted literals broke the emitted
// syntax (generator.go:359-360).
func TestGeneratedSource_AC5b_EscapesBlueprintStrings(t *testing.T) {
	bp := escapeBlueprint()
	outDir := filepath.Join(t.TempDir(), "build")
	if err := GenerateProject(bp, outDir); err != nil {
		t.Fatalf("GenerateProject failed on escape-class strings: %v", err)
	}

	// Structure survives: every emitted file parses; the module compiles.
	parseGeneratedGoFiles(t, outDir)
	vetGeneratedModule(t, outDir)

	// Data survives: the nasty strings round-trip byte-exactly.
	graphLiterals := astStringLiterals(t, filepath.Join(outDir, "graph.go"))
	assertLiteral := func(want, where string) {
		t.Helper()
		for _, got := range graphLiterals {
			if got == want {
				return
			}
		}
		t.Errorf("AC5b FAIL: %s did not round-trip through the emitted source as data.\nwant literal: %q", where, want)
	}
	assertLiteral(nastyInstructions, "node instructions")
	assertLiteral("backslash \\ and \"quote\" and\nnewline", "tool description")

	// And through the embedded declaration: the resident's own copy of the
	// blueprint re-parses to the same declared strings.
	mainLiterals := astStringLiterals(t, filepath.Join(outDir, "main.go"))
	var reparsed *Blueprint
	for _, lit := range mainLiterals {
		if !strings.Contains(lit, "gen-escape") {
			continue
		}
		got, err := ParseBlueprintBytes([]byte(lit))
		if err == nil {
			reparsed = got
			break
		}
	}
	if reparsed == nil {
		t.Fatal("AC5b FAIL: emitted main carries no re-parseable embedded blueprint")
	}
	if got := reparsed.Nodes[0].Instructions; got != nastyInstructions {
		t.Errorf("AC5b FAIL: embedded declaration mangled the instructions.\nwant %q\ngot  %q", nastyInstructions, got)
	}
}
