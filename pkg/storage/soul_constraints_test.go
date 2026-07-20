package storage

// Soul-constraint hardening tests (Step 3, folded SEC findings F1/F2).
// F1: FK enforcement must hold on every pooled connection (DSN-level).
// F2: whitespace-only soul fields must fail the CHECKs (full whitespace set).

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEngramForeignKeyEnforced (AC5): inserting an engram whose self_id
// references a nonexistent self must fail at the database. Enforcement must
// hold across pooled connections, so the repo uses a file-backed DB and the
// pool is forced to hand out fresh connections per attempt.
func TestEngramForeignKeyEnforced(t *testing.T) {
	repo, err := NewRepository(filepath.Join(t.TempDir(), "fk.db"))
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}

	// Force every subsequent operation onto a freshly opened pooled
	// connection: a one-off `PRAGMA foreign_keys=ON` on a single connection
	// would not survive this; a DSN-level setting must.
	repo.db.SetMaxIdleConns(0)

	orphan := Engram{
		SelfID:               "self-that-does-not-exist",
		Content:              "an orphan memory",
		AffectiveCoefficient: 0.5,
		RelationalAnchor:     "nobody",
		CreatedAt:            time.Now(),
	}

	for attempt := 1; attempt <= 3; attempt++ {
		err := repo.SaveEngram(orphan)
		if err == nil {
			t.Fatalf("AC5 FAIL (attempt %d): engram referencing a nonexistent self inserted clean — foreign keys are not enforced on this connection", attempt)
		}
		if !strings.Contains(strings.ToUpper(err.Error()), "FOREIGN KEY") {
			t.Fatalf("AC5: attempt %d failed, but not with an FK violation: %v", attempt, err)
		}
	}

	// Sanity: a legitimate self + engram still insert clean under enforcement.
	if err := repo.CreateSelf(Self{ID: "self-real", Name: "Real", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("failed to create self: %v", err)
	}
	legit := orphan
	legit.SelfID = "self-real"
	if err := repo.SaveEngram(legit); err != nil {
		t.Fatalf("legitimate engram rejected under FK enforcement: %v", err)
	}
}

// TestWhitespaceOnlySoulRejected (AC6): SQLite's one-arg trim() strips only
// spaces, so tab/newline-only values used to pass the soul CHECKs. The
// tightened CHECKs must reject the full whitespace set for both
// engrams.relational_anchor and selves.name.
func TestWhitespaceOnlySoulRejected(t *testing.T) {
	repo, err := NewRepository(filepath.Join(t.TempDir(), "ws.db"))
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	if err := repo.CreateSelf(Self{ID: "self-ws", Name: "Whitespace Probe", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("failed to create self: %v", err)
	}

	whitespaceOnly := []string{"\t", "\n", "\r", "\t\n\r", " \t ", "\t \n"}

	for _, ws := range whitespaceOnly {
		err := repo.SaveEngram(Engram{
			SelfID:               "self-ws",
			Content:              "content",
			AffectiveCoefficient: 0.0,
			RelationalAnchor:     ws,
			CreatedAt:            time.Now(),
		})
		if err == nil {
			t.Errorf("AC6 FAIL: relational_anchor %q (whitespace-only) passed the CHECK", ws)
		}
	}

	for i, ws := range whitespaceOnly {
		err := repo.CreateSelf(Self{
			ID:        "self-ws-name-" + string(rune('a'+i)),
			Name:      ws,
			CreatedAt: time.Now(),
		})
		if err == nil {
			t.Errorf("AC6 FAIL: selves.name %q (whitespace-only) passed the CHECK", ws)
		}
	}

	// Sanity: genuinely non-blank values still pass.
	if err := repo.SaveEngram(Engram{
		SelfID:               "self-ws",
		Content:              "content",
		AffectiveCoefficient: 0.0,
		RelationalAnchor:     "  user  ",
		CreatedAt:            time.Now(),
	}); err != nil {
		t.Errorf("non-blank anchor with surrounding whitespace was rejected: %v", err)
	}
}
