package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // Import sqlite3 driver

	"github.com/amangsingh/agora"
)

// Repository handles data persistence.
type Repository struct {
	db *sql.DB
}

// Execution represents a run of an agent.
type Execution struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"` // "running", "completed", "failed"
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	CreatedAt time.Time `json:"created_at"`
}

// Self is a stable identity that memory is keyed on. It is deliberately
// minimal: just enough identity to make the mind a someone rather than a
// buffer. The full Self-as-owning-object is a later step.
type Self struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Engram is a single unit of mind-memory. It belongs to a Self — not to an
// execution — so it outlives any single run and is shared across runs of the
// same self. Every engram is born with an affective coefficient and a
// relational anchor at memory-formation time; the schema itself forbids a
// write without them ("if it isn't in the schema, it isn't in the soul").
type Engram struct {
	ID                   int64     `json:"id"`
	SelfID               string    `json:"self_id"`
	Content              string    `json:"content"`
	AffectiveCoefficient float64   `json:"affective_coefficient"` // valence/intensity in [-1.0, 1.0]
	RelationalAnchor     string    `json:"relational_anchor"`     // who/what this memory is bound to; never blank
	CreatedAt            time.Time `json:"created_at"`
}

// NewRepository initializes the SQLite database.
//
// Foreign-key enforcement is switched on in the DSN, not via a PRAGMA:
// SQLite defaults FK enforcement OFF per connection, and database/sql pools
// connections, so a one-off `PRAGMA foreign_keys=ON` would bind to a single
// pooled connection and silently miss the rest. The DSN parameter makes the
// driver apply it to every connection it opens (F1).
func NewRepository(dbPath string) (*Repository, error) {
	dsn := dbPath + "?_foreign_keys=on"
	if strings.Contains(dbPath, "?") {
		dsn = dbPath + "&_foreign_keys=on"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	repo := &Repository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return repo, nil
}

func (r *Repository) migrate() error {
	queries := []string{
		// Run-log tables: one row per execution, messages keyed on the run.
		// Still consumed by the server handler; recall re-wiring is a later step.
		`CREATE TABLE IF NOT EXISTS executions (
			id TEXT PRIMARY KEY,
			status TEXT,
			input TEXT,
			output TEXT,
			created_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			execution_id TEXT,
			role TEXT,
			content TEXT,
			FOREIGN KEY(execution_id) REFERENCES executions(id)
		);`,
		// Mind-memory tables: memory is keyed on a self, not a run.
		// A self must have a non-blank identity to be a someone.
		// The CHECKs trim the full whitespace set (space, tab, LF, CR), not
		// just spaces: SQLite's one-arg trim() strips only 0x20, which would
		// let a tab/newline-only "identity" pass as a someone (F2).
		// NOTE: CREATE TABLE IF NOT EXISTS does not retrofit the tightened
		// CHECKs onto pre-existing dev databases; acceptable for dev-stage data.
		// credential_hash is the SEC gate's authn boundary (advisory A): the
		// sha256 of the per-self credential minted at establishment. NULL
		// marks a legacy or library-created self that never established a
		// credential — verification fails closed for those.
		`CREATE TABLE IF NOT EXISTS selves (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
				CHECK(length(trim(name, ' ' || char(9) || char(10) || char(13))) > 0),
			created_at DATETIME NOT NULL,
			credential_hash TEXT
		);`,
		// The soul constraint lives in the schema, not in application code:
		// NOT NULL rejects an omitted/NULL field, and the CHECK clauses reject
		// the degenerate values (blank anchor, out-of-range coefficient) that
		// a forgetful caller's zero values would otherwise smuggle past it.
		`CREATE TABLE IF NOT EXISTS engrams (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			self_id TEXT NOT NULL REFERENCES selves(id),
			content TEXT NOT NULL,
			affective_coefficient REAL NOT NULL
				CHECK(affective_coefficient BETWEEN -1.0 AND 1.0),
			relational_anchor TEXT NOT NULL
				CHECK(length(trim(relational_anchor, ' ' || char(9) || char(10) || char(13))) > 0),
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_engrams_self_id ON engrams(self_id);`,
	}

	for _, q := range queries {
		if _, err := r.db.Exec(q); err != nil {
			return fmt.Errorf("executing query %q: %w", q, err)
		}
	}

	// Dev-stage schema catch-up (A4 posture, Step 3 precedent): CREATE TABLE
	// IF NOT EXISTS does not retrofit credential_hash onto a pre-existing dev
	// database, so add the column when it is missing. Not a migration
	// ceremony — one additive column, no data rewrite.
	hasCredentialHash := false
	rows, err := r.db.Query(`PRAGMA table_info(selves)`)
	if err != nil {
		return fmt.Errorf("inspecting selves schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("scanning selves schema: %w", err)
		}
		if name == "credential_hash" {
			hasCredentialHash = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspecting selves schema: %w", err)
	}
	if !hasCredentialHash {
		if _, err := r.db.Exec(`ALTER TABLE selves ADD COLUMN credential_hash TEXT`); err != nil {
			return fmt.Errorf("adding selves.credential_hash: %w", err)
		}
	}
	return nil
}

// SaveExecution saves the execution state.
func (r *Repository) SaveExecution(exec Execution) error {
	query := `INSERT INTO executions (id, status, input, output, created_at) VALUES (?, ?, ?, ?, ?)`
	// Use Parameterized Query for security (Injection Scan)
	_, err := r.db.Exec(query, exec.ID, exec.Status, exec.Input, exec.Output, exec.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert execution: %w", err)
	}
	return nil
}

// SaveHistory saves chat history for an execution using a transaction.
func (r *Repository) SaveHistory(executionID string, messages []agora.ChatMessage) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO messages (execution_id, role, content) VALUES (?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, msg := range messages {
		if _, err := stmt.Exec(executionID, msg.Role, msg.Content); err != nil {
			tx.Rollback()
			return err
		}
	}
	// ... existing methods ...
	return tx.Commit()
}

// GetHistory retrieves chat history for an execution.
func (r *Repository) GetHistory(executionID string) ([]agora.ChatMessage, error) {
	query := `SELECT role, content FROM messages WHERE execution_id = ? ORDER BY id ASC`
	rows, err := r.db.Query(query, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []agora.ChatMessage
	for rows.Next() {
		var msg agora.ChatMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		history = append(history, msg)
	}
	return history, rows.Err()
}

// AppendMessage saves a single message.
func (r *Repository) AppendMessage(executionID, role, content string) error {
	query := `INSERT INTO messages (execution_id, role, content) VALUES (?, ?, ?)`
	_, err := r.db.Exec(query, executionID, role, content)
	return err
}

// Helper to update output/status
func (r *Repository) UpdateExecution(id, status, output string) error {
	query := `UPDATE executions SET status = ?, output = ? WHERE id = ?`
	_, err := r.db.Exec(query, status, output, id)
	return err
}

// dummyCredentialHash is compared against when the addressed self has no
// stored credential (unknown id, or a legacy row that never established
// one), so the compare happens — in constant time, against a same-shaped
// value — on every verification path.
var dummyCredentialHash = credentialHash("agora-dummy-credential-for-constant-time-compare")

// credentialHash is the stored form of a credential: hex sha256.
func credentialHash(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

// EstablishSelf implements agora.SelfEstablisher: create the self and mint
// its credential in ONE act, so no server-established self ever exists
// without one. The insert is idempotent under concurrent first contact
// (advisory C): the loser of the race — like any caller addressing a taken
// id — receives agora.ErrSelfUnavailable, never a constraint failure.
// The plaintext credential is returned exactly once and only its sha256 is
// stored.
func (r *Repository) EstablishSelf(id, name string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to mint credential: %w", err)
	}
	credential := hex.EncodeToString(raw)

	res, err := r.db.Exec(
		`INSERT INTO selves (id, name, created_at, credential_hash) VALUES (?, ?, ?, ?)
			ON CONFLICT(id) DO NOTHING`,
		id, name, time.Now(), credentialHash(credential))
	if err != nil {
		return "", fmt.Errorf("failed to establish self: %w", err)
	}
	won, err := res.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("failed to establish self: %w", err)
	}
	if won == 0 {
		return "", agora.ErrSelfUnavailable
	}
	return credential, nil
}

// VerifySelfCredential implements agora.CredentialVerifier. Every failure
// shape — unknown self, legacy self without a credential, wrong credential,
// even a read error — collapses into the one agora.ErrSelfAuthentication
// (fail closed, no existence oracle), and the hash compare is constant-time
// on every path.
func (r *Repository) VerifySelfCredential(selfID, credential string) error {
	var stored sql.NullString
	err := r.db.QueryRow(`SELECT credential_hash FROM selves WHERE id = ?`, selfID).Scan(&stored)
	known := err == nil && stored.Valid && stored.String != ""

	target := dummyCredentialHash
	if known {
		target = stored.String
	}
	match := subtle.ConstantTimeCompare([]byte(credentialHash(credential)), []byte(target)) == 1

	if known && match {
		return nil
	}
	return agora.ErrSelfAuthentication
}

// GetEngramsWindowed retrieves the newest n engrams of a self, returned in
// chronological order (oldest of the window first). The bound lives in the
// query — the rest of the store is never loaded, and never touched: this is
// a read shape, not a compaction (SEC gate advisory B, architect addendum).
// The full engram row travels: relational_anchor and affective_coefficient
// are not stripped on the windowed path.
func (r *Repository) GetEngramsWindowed(selfID string, n int) ([]Engram, error) {
	if n <= 0 {
		return nil, fmt.Errorf("recall window must be positive, got %d", n)
	}
	query := `SELECT id, self_id, content, affective_coefficient, relational_anchor, created_at
		FROM engrams WHERE self_id = ? ORDER BY id DESC LIMIT ?`
	rows, err := r.db.Query(query, selfID, n)
	if err != nil {
		return nil, fmt.Errorf("failed to query windowed engrams for self %q: %w", selfID, err)
	}
	defer rows.Close()

	var engrams []Engram
	for rows.Next() {
		var e Engram
		if err := rows.Scan(&e.ID, &e.SelfID, &e.Content,
			&e.AffectiveCoefficient, &e.RelationalAnchor, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan engram: %w", err)
		}
		engrams = append(engrams, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The query walked newest-first to apply the LIMIT; recall speaks
	// chronological order.
	for i, j := 0, len(engrams)-1; i < j; i, j = i+1, j-1 {
		engrams[i], engrams[j] = engrams[j], engrams[i]
	}
	return engrams, nil
}

// CreateSelf registers a stable self identity that memory can be keyed on.
func (r *Repository) CreateSelf(self Self) error {
	query := `INSERT INTO selves (id, name, created_at) VALUES (?, ?, ?)`
	if _, err := r.db.Exec(query, self.ID, self.Name, self.CreatedAt); err != nil {
		return fmt.Errorf("failed to insert self: %w", err)
	}
	return nil
}

// GetSelf retrieves a self identity by id.
func (r *Repository) GetSelf(id string) (Self, error) {
	query := `SELECT id, name, created_at FROM selves WHERE id = ?`
	var self Self
	if err := r.db.QueryRow(query, id).Scan(&self.ID, &self.Name, &self.CreatedAt); err != nil {
		return Self{}, fmt.Errorf("failed to get self %q: %w", id, err)
	}
	return self, nil
}

// SaveEngram writes a single unit of mind-memory for a self. The affective
// coefficient and relational anchor are mandatory at the schema level; a
// write missing either fails at the database constraint, not here.
func (r *Repository) SaveEngram(engram Engram) error {
	query := `INSERT INTO engrams (self_id, content, affective_coefficient, relational_anchor, created_at)
		VALUES (?, ?, ?, ?, ?)`
	if _, err := r.db.Exec(query,
		engram.SelfID, engram.Content, engram.AffectiveCoefficient,
		engram.RelationalAnchor, engram.CreatedAt); err != nil {
		return fmt.Errorf("failed to insert engram: %w", err)
	}
	return nil
}

// GetEngrams retrieves all engrams belonging to a self, oldest first. Memory
// is keyed on self_id alone, so engrams formed during different executions of
// the same self are all visible here, and no other self's engrams ever are.
func (r *Repository) GetEngrams(selfID string) ([]Engram, error) {
	query := `SELECT id, self_id, content, affective_coefficient, relational_anchor, created_at
		FROM engrams WHERE self_id = ? ORDER BY id ASC`
	rows, err := r.db.Query(query, selfID)
	if err != nil {
		return nil, fmt.Errorf("failed to query engrams for self %q: %w", selfID, err)
	}
	defer rows.Close()

	var engrams []Engram
	for rows.Next() {
		var e Engram
		if err := rows.Scan(&e.ID, &e.SelfID, &e.Content,
			&e.AffectiveCoefficient, &e.RelationalAnchor, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan engram: %w", err)
		}
		engrams = append(engrams, e)
	}
	return engrams, rows.Err()
}
