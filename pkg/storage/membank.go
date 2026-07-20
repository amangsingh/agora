package storage

// membank.go makes *Repository a member of the Self's D bank: it implements
// the agora.MemoryStore contract over the Step 2 self-keyed engram schema.
// The framework type (agora.Self) speaks only the contract; the sqlite row
// shape (including its integer row id) stays a storage concern.

import (
	"fmt"
	"time"

	"github.com/amangsingh/agora"
)

// EnsureSelf implements agora.MemoryStore. An unknown self id is registered
// as a new self (the Step 2 identity model) whose memory starts empty —
// preserving the resolution semantics the server's recall path had. The
// create is a single idempotent statement, not get-then-create: concurrent
// first contact for the same new id must never lose on the primary key
// (SEC gate, advisory C). Selves created here carry no credential and
// therefore fail closed at verification; the credentialed path is
// EstablishSelf.
func (r *Repository) EnsureSelf(id string) error {
	_, err := r.db.Exec(
		`INSERT INTO selves (id, name, created_at) VALUES (?, ?, ?)
			ON CONFLICT(id) DO NOTHING`,
		id, id, time.Now())
	if err != nil {
		return fmt.Errorf("failed to ensure self %q: %w", id, err)
	}
	return nil
}

// RecallEngrams implements agora.MemoryStore, loading a self's stored engrams
// in insertion order. Nil-ness is preserved: a self with no engrams recalls
// nil, not an empty slice, so the amnesiac/warm distinction survives the
// contract boundary.
func (r *Repository) RecallEngrams(selfID string) ([]agora.Engram, error) {
	rows, err := r.GetEngrams(selfID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, nil
	}
	engrams := make([]agora.Engram, 0, len(rows))
	for _, e := range rows {
		engrams = append(engrams, agora.Engram{
			SelfID:               e.SelfID,
			Content:              e.Content,
			AffectiveCoefficient: e.AffectiveCoefficient,
			RelationalAnchor:     e.RelationalAnchor,
			CreatedAt:            e.CreatedAt,
		})
	}
	return engrams, nil
}

// RecallEngramsWindowed implements agora.WindowedRecaller: the newest n
// engrams of a self, chronological order, bounded at the query. Nil-ness is
// preserved exactly as in RecallEngrams.
func (r *Repository) RecallEngramsWindowed(selfID string, n int) ([]agora.Engram, error) {
	rows, err := r.GetEngramsWindowed(selfID, n)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, nil
	}
	engrams := make([]agora.Engram, 0, len(rows))
	for _, e := range rows {
		engrams = append(engrams, agora.Engram{
			SelfID:               e.SelfID,
			Content:              e.Content,
			AffectiveCoefficient: e.AffectiveCoefficient,
			RelationalAnchor:     e.RelationalAnchor,
			CreatedAt:            e.CreatedAt,
		})
	}
	return engrams, nil
}

// FormEngram implements agora.MemoryStore, persisting one framework-level
// engram through the soul-schema-enforcing write path.
func (r *Repository) FormEngram(e agora.Engram) error {
	return r.SaveEngram(Engram{
		SelfID:               e.SelfID,
		Content:              e.Content,
		AffectiveCoefficient: e.AffectiveCoefficient,
		RelationalAnchor:     e.RelationalAnchor,
		CreatedAt:            e.CreatedAt,
	})
}
