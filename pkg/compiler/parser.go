package compiler

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// identifierRegex constrains every declared name that becomes a Go
// identifier or a bank address: no silent mangling, fail at parse instead.
var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// stateFieldTypes is the closed set of declarable M-bank field types and
// their Go emissions. An unknown type fails loudly at parse — this repo's
// node-type silent-drop history does not repeat at the blueprint layer.
var stateFieldTypes = map[string]string{
	"string": "string",
	"int":    "int",
	"float":  "float64",
	"bool":   "bool",
}

var (
	memoryKinds  = map[string]bool{"sqlite": true, "none": true}
	ingressKinds = map[string]bool{"rest": true, "cron": true, "listener": true}
	egressKinds  = map[string]bool{"writer": true}
	knowledgeKds = map[string]bool{"file": true}
)

// ParseBlueprint reads a YAML file and unmarshals it into a Blueprint.
// Decoding is STRICT (yaml KnownFields): a key the schema does not declare —
// an unknown letter, a typo — is a parse error, never a silent drop.
func ParseBlueprint(path string) (*Blueprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read blueprint file: %w", err)
	}
	return ParseBlueprintBytes(data)
}

// ParseBlueprintBytes parses an in-memory YAML declaration with the same
// strictness as ParseBlueprint. It exists for consumers that carry the
// declaration with them instead of on disk — the emitted resident (Step 9)
// embeds its blueprint verbatim and re-reads it through this path at wake-up.
func ParseBlueprintBytes(data []byte) (*Blueprint, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var bp Blueprint
	if err := dec.Decode(&bp); err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("failed to parse YAML: blueprint file is empty")
		}
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

	// Validate Nodes (N)
	nodeMap := make(map[string]bool)
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

	// Validate Edges (O)
	for _, e := range bp.Edges {
		if !nodeMap[e.From] && e.From != "START" { // Allow START/END if we decide to use them, though specs say Entry field.
			return fmt.Errorf("edge source '%s' does not exist", e.From)
		}
		if !nodeMap[e.To] && e.To != "END" {
			return fmt.Errorf("edge target '%s' does not exist", e.To)
		}
	}

	// Validate Entry (O)
	if !nodeMap[bp.Graph.Entry] {
		return fmt.Errorf("entry node '%s' does not exist", bp.Graph.Entry)
	}

	// Validate Models (L)
	modelNames := make(map[string]bool)
	for i, m := range bp.Models {
		if m.Name == "" {
			return fmt.Errorf("L bank: model at index %d declares no name", i)
		}
		if modelNames[m.Name] {
			return fmt.Errorf("L bank: duplicate model name '%s'", m.Name)
		}
		modelNames[m.Name] = true
	}

	// A node naming a model must address a declared L-bank member when the
	// L bank is declared at all (v1 blueprints without a models section keep
	// their raw model strings).
	if len(bp.Models) > 0 {
		for _, n := range bp.Nodes {
			if n.Model != "" && !modelNames[n.Model] {
				return fmt.Errorf("node '%s' names model '%s' which is not declared in the L bank", n.Name, n.Model)
			}
		}
	}

	// Validate Tools (T)
	toolNames := make(map[string]bool)
	for i, tl := range bp.Tools {
		if tl.Name == "" {
			return fmt.Errorf("T bank: tool at index %d declares no name", i)
		}
		if toolNames[tl.Name] {
			return fmt.Errorf("T bank: duplicate tool name '%s'", tl.Name)
		}
		toolNames[tl.Name] = true
	}

	// Validate System (S)
	sysNames := make(map[string]bool)
	for i, s := range bp.System {
		if s.Name == "" {
			return fmt.Errorf("S bank: system entry at index %d declares no name", i)
		}
		if sysNames[s.Name] {
			return fmt.Errorf("S bank: duplicate system entry name '%s'", s.Name)
		}
		sysNames[s.Name] = true
		if s.Content == "" {
			return fmt.Errorf("S bank: system entry '%s' declares no content", s.Name)
		}
	}

	// Validate State (M)
	stateNames := make(map[string]bool)
	for i, f := range bp.State {
		if f.Name == "" {
			return fmt.Errorf("M bank: state field at index %d declares no name", i)
		}
		if !identifierRegex.MatchString(f.Name) {
			return fmt.Errorf("M bank: state field name '%s' is invalid: must be a valid identifier (alphanumeric/underscore)", f.Name)
		}
		if stateNames[f.Name] {
			return fmt.Errorf("M bank: duplicate state field name '%s'", f.Name)
		}
		stateNames[f.Name] = true
		if _, ok := stateFieldTypes[f.Type]; !ok {
			return fmt.Errorf("M bank: state field '%s' declares unknown type '%s' (known: string, int, float, bool)", f.Name, f.Type)
		}
	}

	// Validate Knowledge (K)
	knowNames := make(map[string]bool)
	for i, k := range bp.Knowledge {
		if k.Name == "" {
			return fmt.Errorf("K bank: knowledge source at index %d declares no name", i)
		}
		if knowNames[k.Name] {
			return fmt.Errorf("K bank: duplicate knowledge source name '%s'", k.Name)
		}
		knowNames[k.Name] = true
		if k.Kind != "" && !knowledgeKds[k.Kind] {
			return fmt.Errorf("K bank: knowledge source '%s' declares unknown kind '%s' (known: file)", k.Name, k.Kind)
		}
		if k.URI == "" {
			return fmt.Errorf("K bank: knowledge source '%s' declares no uri", k.Name)
		}
	}

	// Validate Memory (D)
	if bp.Memory != nil {
		if !memoryKinds[bp.Memory.Kind] {
			return fmt.Errorf("D bank: memory declares unknown kind '%s' (known: sqlite, none)", bp.Memory.Kind)
		}
		if bp.Memory.Kind == "sqlite" && bp.Memory.DSN == "" {
			return fmt.Errorf("D bank: memory of kind 'sqlite' declares no dsn")
		}
	}

	// Validate Endpoints (E)
	ingressNames := make(map[string]bool)
	for i, in := range bp.Endpoints.Ingress {
		if in.Name == "" {
			return fmt.Errorf("E membrane: ingress at index %d declares no name", i)
		}
		if ingressNames[in.Name] {
			return fmt.Errorf("E membrane: duplicate ingress name '%s'", in.Name)
		}
		ingressNames[in.Name] = true
		if !ingressKinds[in.Kind] {
			return fmt.Errorf("E membrane: ingress '%s' declares unknown kind '%s' (known: rest, cron, listener)", in.Name, in.Kind)
		}
	}
	egressNames := make(map[string]bool)
	for i, eg := range bp.Endpoints.Egress {
		if eg.Name == "" {
			return fmt.Errorf("E membrane: egress at index %d declares no name", i)
		}
		if egressNames[eg.Name] {
			return fmt.Errorf("E membrane: duplicate egress name '%s'", eg.Name)
		}
		egressNames[eg.Name] = true
		if !egressKinds[eg.Kind] {
			return fmt.Errorf("E membrane: egress '%s' declares unknown kind '%s' (known: writer)", eg.Name, eg.Kind)
		}
	}

	return nil
}
