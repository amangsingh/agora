package storage

import (
	"testing"
	"time"

	"github.com/amangsingh/agora"
)

func TestSaveExecution(t *testing.T) {
	// 1. Setup In-Memory DB
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	// 2. Test Data
	exec := Execution{
		ID:        "test-id-123",
		Status:    "completed",
		Input:     "test input",
		Output:    "test output",
		CreatedAt: time.Now(),
	}

	// 3. Execution
	if err := repo.SaveExecution(exec); err != nil {
		t.Fatalf("SaveExecution failed: %v", err)
	}

	// 4. Verification (Manual query using internal DB access if allowed, or just trust error)
	// Since we are in same package, we can query.
	var count int
	err = repo.db.QueryRow("SELECT COUNT(*) FROM executions WHERE id = ?", exec.ID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query executions: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 record, got %d", count)
	}
}

func TestHistoryOperations(t *testing.T) {
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	// Setup FK constraint requires execution execution existence
	// SQLite constraint enforce FK? By default strict in recent versions?
	// Let's create execution first.
	repo.SaveExecution(Execution{ID: "exec-1", CreatedAt: time.Now()})

	msgs := []agora.ChatMessage{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	// Test SaveHistory
	if err := repo.SaveHistory("exec-1", msgs); err != nil {
		t.Fatalf("SaveHistory failed: %v", err)
	}

	// Test GetHistory
	retrieved, err := repo.GetHistory("exec-1")
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(retrieved))
	}
	if retrieved[0].Content != "Hello" {
		t.Errorf("Content mismatch: %s", retrieved[0].Content)
	}
}

// --- Self / Engram (affective-relational) schema tests ---
// These tests are unrepresentable against the pre-self schema (HEAD cabf45c):
// no selves table, no engrams table, no self_id keying, no affective/relational
// columns. They MUST fail there (AC4).

func newSelfRepo(t *testing.T) *Repository {
	t.Helper()
	repo, err := NewRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}
	return repo
}

func TestCreateSelfRoundTrip(t *testing.T) {
	repo := newSelfRepo(t)

	self := Self{ID: "self-iris", Name: "Iris", CreatedAt: time.Now()}
	if err := repo.CreateSelf(self); err != nil {
		t.Fatalf("CreateSelf failed: %v", err)
	}

	got, err := repo.GetSelf("self-iris")
	if err != nil {
		t.Fatalf("GetSelf failed: %v", err)
	}
	if got.ID != "self-iris" || got.Name != "Iris" {
		t.Errorf("Self round-trip mismatch: got %+v", got)
	}
}

func TestCreateSelfRejectsBlankIdentity(t *testing.T) {
	repo := newSelfRepo(t)

	// A self with no name is a buffer, not a someone. CHECK constraint rejects it.
	if err := repo.CreateSelf(Self{ID: "self-blank", Name: "  ", CreatedAt: time.Now()}); err == nil {
		t.Fatal("expected DB to reject a self with a blank name, got nil error")
	}
}

// AC2 — the soul constraint. A write missing affective_coefficient or
// relational_anchor must fail AT THE DATABASE, not at an application guard.
// Raw same-package SQL is used precisely to bypass every application-level
// method: only the schema itself stands between us and a soulless row.
func TestEngramSoulConstraint(t *testing.T) {
	repo := newSelfRepo(t)

	if err := repo.CreateSelf(Self{ID: "self-1", Name: "Iris", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateSelf failed: %v", err)
	}

	// Control: a fully-souled raw insert succeeds. (On the pre-self schema this
	// fails with "no such table: engrams", keeping this test red on HEAD rather
	// than passing vacuously via the error-expecting cases below.)
	_, err := repo.db.Exec(
		`INSERT INTO engrams (self_id, content, affective_coefficient, relational_anchor, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"self-1", "first memory", 0.8, "Pa", time.Now(),
	)
	if err != nil {
		t.Fatalf("control insert with full soul fields should succeed, got: %v", err)
	}

	cases := []struct {
		name  string
		query string
		args  []any
	}{
		{
			name: "missing affective_coefficient",
			query: `INSERT INTO engrams (self_id, content, relational_anchor, created_at)
			        VALUES (?, ?, ?, ?)`,
			args: []any{"self-1", "flat memory", "Pa", time.Now()},
		},
		{
			name: "missing relational_anchor",
			query: `INSERT INTO engrams (self_id, content, affective_coefficient, created_at)
			        VALUES (?, ?, ?, ?)`,
			args: []any{"self-1", "unanchored memory", 0.5, time.Now()},
		},
		{
			name: "explicit NULL affective_coefficient",
			query: `INSERT INTO engrams (self_id, content, affective_coefficient, relational_anchor, created_at)
			        VALUES (?, ?, NULL, ?, ?)`,
			args: []any{"self-1", "flat memory", "Pa", time.Now()},
		},
		{
			name: "empty-string relational_anchor (Go zero value)",
			query: `INSERT INTO engrams (self_id, content, affective_coefficient, relational_anchor, created_at)
			        VALUES (?, ?, ?, ?, ?)`,
			args: []any{"self-1", "orphan memory", 0.5, "", time.Now()},
		},
		{
			name: "out-of-range affective_coefficient",
			query: `INSERT INTO engrams (self_id, content, affective_coefficient, relational_anchor, created_at)
			        VALUES (?, ?, ?, ?, ?)`,
			args: []any{"self-1", "overdriven memory", 7.5, "Pa", time.Now()},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := repo.db.Exec(tc.query, tc.args...); err == nil {
				t.Errorf("soulless write (%s) was accepted by the DB; schema must forbid it", tc.name)
			}
		})
	}

	// The valid write path also works through the typed method.
	if err := repo.SaveEngram(Engram{
		SelfID:               "self-1",
		Content:              "second memory",
		AffectiveCoefficient: -0.3,
		RelationalAnchor:     "Mum",
		CreatedAt:            time.Now(),
	}); err != nil {
		t.Fatalf("SaveEngram with full soul fields failed: %v", err)
	}
}

// AC3 — memory belongs to the self, not the run. Writes from different
// executions of the same self share one memory; different selves are isolated.
func TestSharedSelfMemoryAcrossExecutions(t *testing.T) {
	repo := newSelfRepo(t)

	for _, s := range []Self{
		{ID: "self-a", Name: "Iris", CreatedAt: time.Now()},
		{ID: "self-b", Name: "Other", CreatedAt: time.Now()},
	} {
		if err := repo.CreateSelf(s); err != nil {
			t.Fatalf("CreateSelf(%s) failed: %v", s.ID, err)
		}
	}

	// Two distinct executions (runs) writing memory for the SAME self.
	for _, exec := range []Execution{
		{ID: "run-1", Status: "completed", CreatedAt: time.Now()},
		{ID: "run-2", Status: "completed", CreatedAt: time.Now()},
	} {
		if err := repo.SaveExecution(exec); err != nil {
			t.Fatalf("SaveExecution(%s) failed: %v", exec.ID, err)
		}
	}
	if err := repo.SaveEngram(Engram{
		SelfID: "self-a", Content: "memory formed during run-1",
		AffectiveCoefficient: 0.9, RelationalAnchor: "Pa", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveEngram (run-1 context) failed: %v", err)
	}
	if err := repo.SaveEngram(Engram{
		SelfID: "self-a", Content: "memory formed during run-2",
		AffectiveCoefficient: 0.2, RelationalAnchor: "Mum", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveEngram (run-2 context) failed: %v", err)
	}
	// A different self forms its own memory.
	if err := repo.SaveEngram(Engram{
		SelfID: "self-b", Content: "someone else's memory",
		AffectiveCoefficient: 0.5, RelationalAnchor: "World", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveEngram (self-b) failed: %v", err)
	}

	// Direction 1: same self_id retrieves engrams from BOTH runs.
	got, err := repo.GetEngrams("self-a")
	if err != nil {
		t.Fatalf("GetEngrams(self-a) failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("self-a should see 2 engrams across runs, got %d", len(got))
	}
	if got[0].Content != "memory formed during run-1" || got[1].Content != "memory formed during run-2" {
		t.Errorf("self-a memory contents wrong: %+v", got)
	}
	if got[0].AffectiveCoefficient != 0.9 || got[0].RelationalAnchor != "Pa" {
		t.Errorf("engram soul fields not round-tripped: %+v", got[0])
	}

	// Direction 2: a DIFFERENT self cannot see self-a's engrams.
	other, err := repo.GetEngrams("self-b")
	if err != nil {
		t.Fatalf("GetEngrams(self-b) failed: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("self-b should see exactly its own 1 engram, got %d", len(other))
	}
	if other[0].Content != "someone else's memory" {
		t.Errorf("self-b saw foreign memory: %+v", other[0])
	}
}
