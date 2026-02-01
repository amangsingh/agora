package storage

import (
	"database/sql"
	"fmt"
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

// NewRepository initializes the SQLite database.
func NewRepository(dbPath string) (*Repository, error) {
	db, err := sql.Open("sqlite3", dbPath)
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
	}

	for _, q := range queries {
		if _, err := r.db.Exec(q); err != nil {
			return fmt.Errorf("executing query %q: %w", q, err)
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
