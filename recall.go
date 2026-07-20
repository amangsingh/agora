// in agora/recall.go

package agora

import "fmt"

// This file is the SEC gate's answer to advisory B (unbounded engram load),
// shaped by the architect's addendum: the recall window is ONE RECALL
// STRATEGY over the engram store — a bounded query shape behind a nameable
// seam on the D bank — never a schema commitment and never destructive
// truncation. The Step-2 engram schema is latent edge structure
// (relational_anchor = edge label, affective_coefficient = edge weight), so
// the windowed read path carries both untouched, and most-recent-N can later
// be swapped for salience- or edge-shaped recall by replacing the strategy,
// with no schema migration.

// DefaultRecallWindow bounds the engram load of an episode when the
// blueprint declares no window of its own. Recall is never unbounded by
// default.
const DefaultRecallWindow = 50

// WindowedRecaller is the optional D-bank capability for windowing AT QUERY
// TIME: return the newest n engrams of a self in chronological order,
// without loading the rest. *storage.Repository implements it; stores
// without it are windowed in memory after a full recall.
type WindowedRecaller interface {
	RecallEngramsWindowed(selfID string, n int) ([]Engram, error)
}

// RecallStrategy is the nameable seam on the D bank: HOW a self's stored
// engrams become an episode's warm start. Strategies bound their reads —
// the unbounded load is exactly what the SEC gate retired.
type RecallStrategy interface {
	// Name identifies the strategy (diagnostics, declarations).
	Name() string
	// Recall loads the engrams an episode warms up from, oldest first.
	Recall(d MemoryStore, selfID string) ([]Engram, error)
}

// MostRecentN is the default recall strategy: the newest N engrams of the
// self, presented in chronological order. N of zero (or negative) means
// DefaultRecallWindow.
type MostRecentN struct {
	N int
}

func (s MostRecentN) window() int {
	if s.N > 0 {
		return s.N
	}
	return DefaultRecallWindow
}

// Name implements RecallStrategy.
func (s MostRecentN) Name() string {
	return fmt.Sprintf("most-recent-%d", s.window())
}

// Recall implements RecallStrategy. A store with the WindowedRecaller
// capability bounds the read at the query; any other store is bounded here,
// after the fact — the payload stays windowed either way. Nil-ness is
// preserved (a self with no engrams recalls nil), and the window never
// writes: the store keeps everything it had.
func (s MostRecentN) Recall(d MemoryStore, selfID string) ([]Engram, error) {
	n := s.window()
	if wr, ok := d.(WindowedRecaller); ok {
		return wr.RecallEngramsWindowed(selfID, n)
	}
	all, err := d.RecallEngrams(selfID)
	if err != nil || len(all) <= n {
		return all, err
	}
	return all[len(all)-n:], nil
}
