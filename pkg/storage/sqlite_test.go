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
