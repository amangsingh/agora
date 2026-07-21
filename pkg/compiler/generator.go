package compiler

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"gopkg.in/yaml.v3"
)

// agoraModulePath is the framework module every emitted project depends on.
const agoraModulePath = "github.com/amangsingh/agora"

// goString renders a blueprint-declared string as a Go source literal. EVERY
// blueprint string that enters emitted source passes through here (SEC F1):
// quotes, backslashes, newlines and tabs arrive as DATA — declared content
// can never alter the structure of the generated code.
func goString(s string) string { return strconv.Quote(s) }

// Compile orchestrates the generation process.
func Compile(blueprintPath, outputDir string) error {
	// 1. Parse
	bp, err := ParseBlueprint(blueprintPath)
	if err != nil {
		return err
	}

	fmt.Printf("Compiling project '%s' version %s...\n", bp.Project, bp.Version)

	return GenerateProject(bp, outputDir)
}

// GenerateProject emits a project from an already-parsed blueprint. It is
// the press consumer of the declaration; ConstructSelf (construct.go) is the
// library consumer. Both take the SAME *Blueprint value — one truth, two
// consumers.
func GenerateProject(bp *Blueprint, outputDir string) error {
	// 2. Generate Main
	if err := generateMain(bp, outputDir); err != nil {
		return err
	}

	// 3. Generate Go Mod
	if err := generateGoMod(bp, outputDir); err != nil {
		return err
	}

	// 4. Generate Graph (Architecture)
	if err := generateGraph(bp, outputDir); err != nil {
		return err
	}

	// 5. Generate State
	if err := generateState(bp, outputDir); err != nil {
		return err
	}

	// 6. Generate Tests (DOC-02)
	if err := generateTests(bp, outputDir); err != nil {
		return err
	}

	return nil
}

// ... existing code ...

func generateTests(bp *Blueprint, outDir string) error {
	// 1. Generate Mock LLM
	mockTmpl := `package main

import (
	"context"

	"github.com/amangsingh/agora"
)

// MockLLM is a test helper that returns static responses.
type MockLLM struct {
	Response string
}

func (m *MockLLM) Invoke(ctx context.Context, req agora.ModelRequest) (agora.ModelResponse, error) {
	return agora.ModelResponse{
		Choices: []agora.Choice{
			{
				Message: agora.ChatMessage{
					Role:    "assistant",
					Content: m.Response,
				},
			},
		},
	}, nil
}
`
	if err := SafeWriteFile(outDir, "mock_llm.go", []byte(mockTmpl)); err != nil {
		return err
	}

	// 2. Generate Graph Test — shaped by the blueprint, and TRUTHFUL for its
	// shape: a purely-agent graph gets the mocked linear execution test; any
	// graph carrying tool nodes gets the declared-tools behavioral test
	// (mock-swapped linear execution does not describe a graph that routes
	// through tool nodes, and an emitted test that fails when run is a lie).
	hasAgents, hasToolNodes, err := splitNodeKinds(bp)
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString(`package main

import (
	"context"
	"testing"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/nodes"
)
`)

	if hasAgents && !hasToolNodes {
		b.WriteString(`
func TestGraphExecution(t *testing.T) {
	// 1. Setup
	g := NewGraph()
	ctx := context.Background()

	// 2. Swap real LLMs with mocks: the generated graph's agent nodes are
	// replaced in the map, so execution never reaches a model transport.
	mock := &MockLLM{Response: "Test Response"}
`)
		for _, n := range bp.Nodes {
			if n.Type != "agent" {
				continue
			}
			fmt.Fprintf(&b, "\tg.AddNode(%s, nodes.SimpleAgentNode(mock, \"System Prompt\"))\n", goString(n.Name))
		}
		b.WriteString(`
	// 3. Execute
	initialState := &ConversationState{
		BaseState: agora.NewBaseState(),
		Input:     "Test Input",
	}

	finalState, err := g.Execute(ctx, initialState)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}

	// 4. Verify
	fs := finalState.(*ConversationState)
	if len(fs.History) == 0 {
		t.Error("Expected history but got none")
	} else {
		last := fs.History[len(fs.History)-1]
		if last.Content != "Test Response" {
			t.Errorf("Expected 'Test Response', got '%s'", last.Content)
		}
	}
}
`)
	}

	if hasToolNodes {
		quotedNames := make([]string, 0, len(declaredToolUnion(bp)))
		for _, td := range declaredToolUnion(bp) {
			quotedNames = append(quotedNames, goString(td.Name))
		}
		b.WriteString(`
func TestDeclaredToolsInvocable(t *testing.T) {
	ctx := context.Background()
	reg := DeclaredTools()
`)
		fmt.Fprintf(&b, "\tfor _, name := range []string{%s} {\n", strings.Join(quotedNames, ", "))
		b.WriteString(`		tool, ok := reg[name]
		if !ok {
			t.Fatalf("declared tool %q missing from the registry", name)
		}
		if _, err := tool.Execute(ctx, map[string]interface{}{"probe": "press"}); err != nil {
			t.Fatalf("declared tool %q failed to execute: %v", name, err)
		}
	}
`)

		// Behavioral: run a planted call through the tool NODE itself, using
		// the first tool of the first tool node that names one.
		var probeTool string
		for _, n := range bp.Nodes {
			if n.Type == "tool_node" && len(n.Tools) > 0 {
				probeTool = n.Tools[0]
				break
			}
		}
		if probeTool != "" {
			fmt.Fprintf(&b, `
	// Behavioral: the tool node executes a planted call and appends the
	// tool's response turn — invocation, not just registration.
	exec := nodes.ToolExecutorNode(reg)
	st := &ConversationState{BaseState: agora.NewBaseState()}
	call := agora.ToolCall{ID: "probe-1", Type: "function"}
	call.Function.Name = %s
	call.Function.Arguments = map[string]interface{}{"probe": "press"}
	st.Set("tool_calls", []agora.ToolCall{call})
	res, err := exec(ctx, st)
	if err != nil {
		t.Fatalf("tool node execution failed: %%v", err)
	}
	fs := res.State.(*ConversationState)
	if len(fs.History) == 0 || fs.History[len(fs.History)-1].Role != "tool" {
		t.Fatal("tool node did not append a tool response turn")
	}
}
`, goString(probeTool))
		} else {
			b.WriteString("}\n")
		}
	}

	return SafeWriteFile(outDir, "graph_test.go", []byte(b.String()))
}

// residentMainTemplate is the emitted main.go: a RESIDENT, not a script.
// The @BLUEPRINT_YAML@ placeholder is replaced with the project's own
// declaration as a quoted Go literal — the only blueprint-derived content in
// this file, and it enters as DATA. Everything below the const is static.
const residentMainTemplate = `package main

// Generated by the agora press (Step 9): this program is a RESIDENT, not a
// script. main constructs a Self FROM THE BLUEPRINT, starts R (the Hum), and
// serves its membrane until told to die — and then it dies well.

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/pkg/compiler"
	"github.com/amangsingh/agora/pkg/server"
	"github.com/amangsingh/agora/pkg/storage"
)

// blueprintYAML is the declaration this resident was pressed from, embedded
// verbatim. The resident re-reads it at wake-up through the SAME strict
// parser and constructor a library consumer uses (ParseBlueprintBytes ->
// ConstructSelf): one truth, two consumers, and the press is the second.
const blueprintYAML = @BLUEPRINT_YAML@

func main() {
	os.Exit(run())
}

func run() int {
	bp, err := compiler.ParseBlueprintBytes([]byte(blueprintYAML))
	if err != nil {
		log.Printf("resident: embedded blueprint does not parse: %v", err)
		return 1
	}

	// D bank: the declared engram store; the execution log shares it.
	dsn := os.Getenv("AGORA_DB")
	if dsn == "" {
		if bp.Memory != nil && bp.Memory.Kind == "sqlite" && bp.Memory.DSN != "" {
			dsn = bp.Memory.DSN
		} else {
			dsn = bp.Project + ".db"
		}
	}
	repo, err := storage.NewRepository(dsn)
	if err != nil {
		log.Printf("resident: D bank at %q failed to open: %v", dsn, err)
		return 1
	}

	// 1: the Self, constructed from the blueprint through the one-truth
	// path. Recall stays bounded by construction: no RecallWindow override
	// means MostRecentN over the framework default window (SEC advisory B).
	self, err := compiler.ConstructSelf(bp, compiler.SelfOptions{
		ModelFactory: declaredModelClient,
		Memory:       repo,
	})
	if err != nil {
		log.Printf("resident: Self construction failed: %v", err)
		return 1
	}
	log.Printf("resident: self %q constructed; banks live for process lifetime", self.ID())

	// Arm the death signal BEFORE the Hum starts writing D and before the
	// membrane advertises readiness. If the handler were installed after those
	// (as it once was), a SIGTERM arriving in that window would hit the Go
	// default disposition and hard-kill the process mid-Hum — a potentially
	// half-written D and no clean shutdown (AC8). Ordering here is load-bearing.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// R: the Hum. A declared rhythm (or the resident default, when the
	// blueprint declares none) starts here; an explicit enabled: false
	// stays quiet — the declaration is honored either way.
	var hum *agora.Hum
	if bp.Rhythm == nil || bp.Rhythm.Enabled {
		hum, err = self.StartHum(agora.HumConfig{Interval: humInterval()})
		if err != nil {
			log.Printf("resident: Hum failed to start: %v", err)
			return 1
		}
		log.Printf("resident: self %q is humming", self.ID())
	}

	// E (ingress): the membrane is pkg/server's — self authn enforced on
	// /run before anything is logged, resolved or recalled (SEC advisory A),
	// and self creation fused with credential minting on /selves.
	handler := &server.AgentHandler{Repo: repo, Self: self}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", handler.HandleRun)
	mux.HandleFunc("POST /selves", handler.HandleEstablishSelf)
	mux.HandleFunc("GET /history", handler.HandleGetHistory)

	ln, err := net.Listen("tcp", ingressAddr(bp))
	if err != nil {
		log.Printf("resident: ingress listen failed: %v", err)
		return 1
	}
	srv := &http.Server{Handler: mux}
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("resident: ingress serve failed: %v", serveErr)
		}
	}()
	log.Printf("resident: serving ingress on %s", ln.Addr())

	// Declared-but-later membrane members are named on the log, never
	// silently dropped: non-rest ingress kinds and all egress wiring land
	// in later steps (egress only behind the SEC gate).
	for _, in := range bp.Endpoints.Ingress {
		if in.Kind != "rest" {
			log.Printf("resident: ingress %q (kind %s) is declared; serving it lands in a later step", in.Name, in.Kind)
		}
	}
	for _, eg := range bp.Endpoints.Egress {
		log.Printf("resident: egress %q (kind %s) is declared; reaching out lands with Step 6 behind the SEC gate", eg.Name, eg.Kind)
	}

	if os.Getenv("AGORA_RESIDENT_PROBE") != "" {
		go residentProbe(hum)
	}

	// Live until told otherwise; then die well. The handler was armed above,
	// before the Hum and the membrane came up.
	<-ctx.Done()

	// Dying well, in order: drain the membrane first so in-flight episodes
	// finish their D writes, then stop the Hum — Stop joins its goroutine
	// before returning, so nothing is left running and nothing is left
	// half-written.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("resident: ingress drain: %v", err)
	}
	if err := self.Close(); err != nil {
		log.Printf("resident: hum stop: %v", err)
	}
	log.Printf("resident: hum stopped cleanly; dying well")
	return 0
}

// declaredModelClient adapts one declared L-bank member to the graph's own
// provider constructor (newDeclaredModel, graph.go) — the same L resolution
// whether the model is reached through the Self's banks or the graph.
func declaredModelClient(m compiler.ModelDecl) agora.Model {
	model := m.Model
	if model == "" {
		model = m.Name
	}
	return newDeclaredModel(m.Provider, model, m.BaseURL)
}

// ingressAddr resolves where the membrane listens: environment override,
// then the first declared rest ingress, then the resident default.
func ingressAddr(bp *compiler.Blueprint) string {
	if addr := os.Getenv("AGORA_ADDR"); addr != "" {
		return addr
	}
	for _, in := range bp.Endpoints.Ingress {
		if in.Kind == "rest" && in.Addr != "" {
			return in.Addr
		}
	}
	return ":8080"
}

// humInterval resolves the Hum cadence: environment override, else zero,
// which HumConfig documents as the framework default.
func humInterval() time.Duration {
	if v := os.Getenv("AGORA_HUM_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
		log.Printf("resident: ignoring invalid AGORA_HUM_INTERVAL %q", v)
	}
	return 0
}

// residentProbe re-runs Step 5's anti-cron discriminator INSIDE this
// generated process — plant unpersisted in-memory residue, let >=3
// iterations pass, report whether it survived — then invokes every declared
// tool once. Both verdicts land on the log. Enabled only by
// AGORA_RESIDENT_PROBE; a resident left alone probes nothing.
func residentProbe(hum *agora.Hum) {
	if hum != nil {
		const key = "hum.residue"
		const residue = "unpersisted-residue-resident-9d2c" // never written to D, by design
		start := hum.Iterations()
		hum.SetWorking(key, residue)
		deadline := time.Now().Add(30 * time.Second)
		for hum.Iterations() < start+3 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		got := hum.Working(key)
		log.Printf("RESIDENT_PROBE anticron survived=%t iterations_start=%d iterations_now=%d",
			got == residue, start, hum.Iterations())
	}
	for name, tool := range DeclaredTools() {
		result, err := tool.Execute(context.Background(), map[string]interface{}{"probe": "resident"})
		log.Printf("RESIDENT_PROBE tool=%s invoked=%t result=%v err=%v", name, err == nil, result, err)
	}
}
`

// generateMain emits the resident's main.go. It CONSUMES the blueprint: the
// declaration is embedded verbatim (as a quoted literal — data, not code)
// and the emitted program re-reads it through ParseBlueprintBytes and
// constructs its Self through ConstructSelf. (Step 9 killed the `_ = bp`
// defect here — the emitted main used to be a fixed run-once script.)
func generateMain(bp *Blueprint, outDir string) error {
	declaration, err := yaml.Marshal(bp)
	if err != nil {
		return fmt.Errorf("re-marshalling blueprint for emission: %w", err)
	}
	src := strings.Replace(residentMainTemplate, "@BLUEPRINT_YAML@", goString(string(declaration)), 1)
	return SafeWriteFile(outDir, "main.go", []byte(src))
}

// generateGoMod emits a module that builds and runs AS EMITTED. The require
// names agora v0.0.0 — a version published nowhere — so the framework rides
// along by source: the replace directive is materialized at generation time
// from wherever the press itself found the framework (Step 9; supersedes the
// old ship-without-resolution gap).
func generateGoMod(bp *Blueprint, outDir string) error {
	frameworkDir, err := frameworkSourceDir()
	if err != nil {
		return fmt.Errorf("go.mod emission: %w", err)
	}
	replacePath := frameworkDir
	if strings.ContainsAny(replacePath, " \t\"'`") {
		replacePath = strconv.Quote(replacePath)
	}
	tmpl := fmt.Sprintf(`module %s

go 1.25

require %s v0.0.0

replace %s => %s
`, bp.Project, agoraModulePath, agoraModulePath, replacePath)
	return SafeWriteFile(outDir, "go.mod", []byte(tmpl))
}

var (
	frameworkDirOnce sync.Once
	frameworkDir     string
	frameworkDirErr  error
)

// frameworkSourceDir locates the agora framework source tree the emitted
// replace directive points at. Resolution is local-only (no network): first
// the module context the press runs in (`go list -m` answers from the main
// module or the local cache), then the press's own source tree.
func frameworkSourceDir() (string, error) {
	frameworkDirOnce.Do(func() {
		frameworkDir, frameworkDirErr = resolveFrameworkSourceDir()
	})
	return frameworkDir, frameworkDirErr
}

func resolveFrameworkSourceDir() (string, error) {
	if goBin, err := exec.LookPath("go"); err == nil {
		if out, err := exec.Command(goBin, "list", "-m", "-f", "{{.Dir}}", agoraModulePath).Output(); err == nil {
			if dir := strings.TrimSpace(string(out)); dir != "" && isAgoraModuleRoot(dir) {
				return dir, nil
			}
		}
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		dir := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		if isAgoraModuleRoot(dir) {
			return dir, nil
		}
	}
	return "", fmt.Errorf("cannot locate the %s source tree to materialize the emitted replace directive: run the press from a module that requires it, or from the framework source tree", agoraModulePath)
}

// isAgoraModuleRoot verifies a directory actually holds the framework
// module — the replace target is checked, never assumed.
func isAgoraModuleRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	return err == nil && strings.Contains(string(data), "module "+agoraModulePath)
}

// goFieldName converts a declared snake_case state-field name into the
// exported Go field the generated struct carries (resonance_index ->
// ResonanceIndex). Names are parse-validated identifiers, so this never
// mangles silently.
func goFieldName(name string) string {
	parts := strings.Split(name, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

// goFieldType maps a declared M-bank field type to its Go emission. The
// parser has already rejected unknown types; reaching the default here
// means generateState was fed an unvalidated blueprint, which must fail
// loudly, not emit a guess.
func goFieldType(declared string) (string, error) {
	if goType, ok := stateFieldTypes[declared]; ok {
		return goType, nil
	}
	return "", fmt.Errorf("M bank: state field type '%s' has no Go emission (known: string, int, float, bool)", declared)
}

// generateState emits the generated project's state.go. It CONSUMES the
// blueprint: every declared M-bank field becomes a typed field of the
// generated ConversationState. (Step 8 killed the `_ = bp` defect here —
// the emitted state used to be a fixed template regardless of declaration.)
func generateState(bp *Blueprint, outDir string) error {
	type stateFieldGen struct {
		GoName  string
		GoType  string
		YAMLKey string
	}
	fields := make([]stateFieldGen, 0, len(bp.State))
	for _, f := range bp.State {
		goType, err := goFieldType(f.Type)
		if err != nil {
			return fmt.Errorf("state field '%s': %w", f.Name, err)
		}
		fields = append(fields, stateFieldGen{
			GoName:  goFieldName(f.Name),
			GoType:  goType,
			YAMLKey: f.Name,
		})
	}

	const structTmpl = `package main

import (
	"encoding/json"
	"fmt"
	"github.com/amangsingh/agora"
)

// ConversationState carries the transcript plus the M-bank fields this
// project's blueprint declares.
type ConversationState struct {
	agora.BaseState ` + "`mapstructure:\",squash\"`" + `
	History   []agora.ChatMessage ` + "`mapstructure:\"history\"`" + `
	Input     string        ` + "`mapstructure:\"input\"`" + `
{{- range .Fields}}
	{{.GoName}} {{.GoType}} ` + "`mapstructure:\"{{.YAMLKey}}\" json:\"{{.YAMLKey}}\"`" + ` // declared state field: {{.YAMLKey}}
{{- end}}
}`

	t, err := template.New("state-struct").Parse(structTmpl)
	if err != nil {
		return fmt.Errorf("failed to parse state template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct{ Fields []stateFieldGen }{fields}); err != nil {
		return fmt.Errorf("failed to execute state template: %w", err)
	}

	tmpl := buf.String() + `

// ToChatHistory returns a freshly allocated slice combining History with the
// pending Input (when not yet consumed). It shares no backing storage with
// History, so caller mutations can never corrupt the state.
func (s *ConversationState) ToChatHistory() ([]agora.ChatMessage, error) {
	messages := make([]agora.ChatMessage, 0, len(s.History)+1)
	messages = append(messages, s.History...)
	if s.Input != "" {
		messages = append(messages, agora.ChatMessage{Role: "user", Content: s.Input})
	}
	return messages, nil
}

// AppendTurn consumes the pending user Input exactly once, then appends the
// output. Subsequent turns append only their own output — the user turn is
// never re-injected into the transcript.
func (s *ConversationState) AppendTurn(output agora.ChatMessage) error {
	if s.Input != "" {
		s.History = append(s.History, agora.ChatMessage{Role: "user", Content: s.Input})
		s.Input = ""
	}
	s.History = append(s.History, output)
	return nil
}

func (s *ConversationState) DeepCopy() (agora.State, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal for DeepCopy: %w", err)
	}
	var newState ConversationState
	if err := json.Unmarshal(bytes, &newState); err != nil {
		return nil, fmt.Errorf("failed to unmarshal for DeepCopy: %w", err)
	}
	return &newState, nil
}
`
	return SafeWriteFile(outDir, "state.go", []byte(tmpl))
}

// splitNodeKinds classifies the N bank for emission. An unknown node type is
// a loud generation failure — this repo's node-type silent-drop history
// (agent-only guards leaving dead imports behind) does not repeat.
func splitNodeKinds(bp *Blueprint) (hasAgents, hasToolNodes bool, err error) {
	for _, n := range bp.Nodes {
		switch n.Type {
		case "agent":
			hasAgents = true
		case "tool_node":
			hasToolNodes = true
		default:
			return false, false, fmt.Errorf("N bank: node '%s' declares type '%s' which has no emission (known: agent, tool_node) — nothing is silently dropped", n.Name, n.Type)
		}
	}
	return hasAgents, hasToolNodes, nil
}

// declaredToolUnion is the generated project's full tool set, in stable
// order: every T-bank declaration first, then any tool a node references
// that the T bank does not declare (carried as a bare declaration).
func declaredToolUnion(bp *Blueprint) []ToolDecl {
	seen := make(map[string]bool, len(bp.Tools))
	union := make([]ToolDecl, 0, len(bp.Tools))
	for _, td := range bp.Tools {
		seen[td.Name] = true
		union = append(union, td)
	}
	for _, n := range bp.Nodes {
		for _, name := range n.Tools {
			if !seen[name] {
				seen[name] = true
				union = append(union, ToolDecl{Name: name})
			}
		}
	}
	return union
}

// generateGraph emits the generated project's graph.go. It CONSUMES the
// blueprint: agent nodes resolve their model through the DECLARED L-bank
// member (provider, model, base_url — Step 9 killed the hard-coded localhost
// default), tool nodes are wired from the T bank, and every declared string
// enters the source through goString — as data, never as code (SEC F1).
func generateGraph(bp *Blueprint, outDir string) error {
	if _, _, err := splitNodeKinds(bp); err != nil {
		return err
	}

	modelDecls := make(map[string]ModelDecl, len(bp.Models))
	for _, m := range bp.Models {
		modelDecls[m.Name] = m
	}

	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\n\t\"github.com/amangsingh/agora\"\n")
	b.WriteString("\t\"github.com/amangsingh/agora/llm\"\n")
	b.WriteString("\t\"github.com/amangsingh/agora/nodes\"\n")
	b.WriteString(")\n\n")

	fmt.Fprintf(&b, "func NewGraph() *agora.Graph {\n\tg := agora.NewGraph()\n\tg.MaxSteps = %d\n\tg.SetEntry(%s)\n\n", bp.Graph.MaxSteps, goString(bp.Graph.Entry))
	for _, n := range bp.Nodes {
		// n.Name and n.Type are parse-validated tokens (identifier regex /
		// the closed kind set above), safe in a comment.
		fmt.Fprintf(&b, "\t// Node: %s (%s)\n", n.Name, n.Type)
		switch n.Type {
		case "agent":
			decl, declared := modelDecls[n.Model]
			if !declared {
				// v1 blueprint (no L bank): the raw model string rides as
				// the provider-side id on the default provider.
				decl = ModelDecl{Name: n.Model, Model: n.Model}
			}
			modelID := decl.Model
			if modelID == "" {
				modelID = decl.Name
			}
			fmt.Fprintf(&b, "\tmodel_%s := newDeclaredModel(%s, %s, %s)\n", n.Name, goString(decl.Provider), goString(modelID), goString(decl.BaseURL))
			fmt.Fprintf(&b, "\tg.AddNode(%s, nodes.SimpleAgentNode(model_%s, %s))\n\n", goString(n.Name), n.Name, goString(n.Instructions))
		case "tool_node":
			quoted := make([]string, 0, len(n.Tools))
			for _, toolName := range n.Tools {
				quoted = append(quoted, goString(toolName))
			}
			fmt.Fprintf(&b, "\tg.AddNode(%s, nodes.ToolExecutorNode(registryFor(%s)))\n\n", goString(n.Name), strings.Join(quoted, ", "))
		}
	}

	b.WriteString("\t// --- Edges ---\n")
	for _, e := range bp.Edges {
		if e.To == "END" {
			// Termination is implicit: Graph.Execute ends when no edge
			// leads onward.
			continue
		}
		fmt.Fprintf(&b, "\tg.AddEdge(%s, %s)\n", goString(e.From), goString(e.To))
	}
	b.WriteString("\n\treturn g\n}\n\n")

	// The T bank, made live: declarations become invocable registry members.
	b.WriteString(`// DeclaredTool is a T-bank declaration made invocable: Execute acknowledges
// the invocation and echoes the arguments back as data. Binding richer
// behavior to a declared tool is later work — but a declared tool is never a
// dead entry.
type DeclaredTool struct {
	Name        string
	Description string
}

func (t DeclaredTool) Definition() agora.ToolDefinition {
	return agora.ToolDefinition{
		Type:     "function",
		Function: agora.Function{Name: t.Name, Description: t.Description},
	}
}

func (t DeclaredTool) Execute(_ context.Context, args map[string]interface{}) (any, error) {
	return map[string]interface{}{"tool": t.Name, "args": args, "declared": true}, nil
}

// DeclaredTools returns the generated project's full T bank as a live
// registry: every declared tool, invocable. Tool nodes draw their working
// sets from it by name.
func DeclaredTools() agora.ToolRegistry {
	reg := agora.NewToolRegistry()
`)
	for _, td := range declaredToolUnion(bp) {
		fmt.Fprintf(&b, "\treg.Register(DeclaredTool{Name: %s, Description: %s})\n", goString(td.Name), goString(td.Description))
	}
	b.WriteString(`	return reg
}

// registryFor selects a tool node's working set from the T bank by name.
func registryFor(names ...string) agora.ToolRegistry {
	all := DeclaredTools()
	reg := agora.NewToolRegistry()
	for _, name := range names {
		if tool, ok := all[name]; ok {
			reg.Register(tool)
		}
	}
	return reg
}
`)

	b.WriteString(`
// newDeclaredModel constructs the client for one declared L-bank member.
// Provider selection follows the declaration; credentials come from the
// environment, never from the blueprint. Agent nodes and the resident's
// ModelFactory (main.go) both resolve models through this one function.
func newDeclaredModel(provider, model, baseURL string) llm.LLM {
	switch provider {
	case "openai":
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return llm.NewOpenAICompatibleLLM(baseURL, model, os.Getenv("AGORA_MODEL_TOKEN"))
	case "google":
		return llm.NewGoogleStudioLLM(os.Getenv("AGORA_MODEL_TOKEN"), model)
	default:
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		return llm.NewOllamaLLM(baseURL, model)
	}
}
`)

	return SafeWriteFile(outDir, "graph.go", []byte(b.String()))
}
