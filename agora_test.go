package agora_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/amangsingh/agora"
)

// Helper to get a valid state object
func newTestState() *agora.ConversationState {
	return &agora.ConversationState{
		BaseState: agora.NewBaseState(),
		History:   []agora.ChatMessage{},
		Input:     "test input",
	}
}

// TestGraph_Execute_MaxSteps verifies that the graph stops and returns an error
// when the step count exceeds MaxSteps.
func TestGraph_Execute_MaxSteps(t *testing.T) {
	g := agora.NewGraph()
	g.MaxSteps = 3
	g.SetEntry("start")

	// Create a node that just loops back to itself
	g.AddNode("start", func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		return agora.NodeResult{
			State:    s,
			NextNode: "start", // Infinite loop
		}, nil
	})

	state := newTestState()
	_, err := g.Execute(context.Background(), state)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, agora.ErrMaxStepsExceeded) {
		t.Errorf("expected ErrMaxStepsExceeded, got %v", err)
	}
}

// TestGraph_Execute_ContextCancel verifies that the graph stops immediately
// if the context is cancelled.
func TestGraph_Execute_ContextCancel(t *testing.T) {
	g := agora.NewGraph()
	g.SetEntry("start")

	g.AddNode("start", func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		// Wait a bit to allow context cancellation to happen if it hasn't already
		select {
		case <-ctx.Done():
			return agora.NodeResult{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
			return agora.NodeResult{
				State:    s,
				NextNode: "start",
			}, nil
		}
	})

	// Cancel strictly before execution for deterministic test
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	state := newTestState()
	_, err := g.Execute(ctx, state)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error could be context.Canceled directly or wrapped
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestGraph_Execute_Success verifies a simple 2-node linear flow.
func TestGraph_Execute_Success(t *testing.T) {
	g := agora.NewGraph()
	g.SetEntry("step1")

	g.AddNode("step1", func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		s.Set("step1", true)
		return agora.NodeResult{
			State:    s,
			NextNode: "step2",
		}, nil
	})

	g.AddNode("step2", func(ctx context.Context, s agora.State) (agora.NodeResult, error) {
		s.Set("step2", true)
		return agora.NodeResult{
			State:  s,
			IsDone: true, // Finish here
		}, nil
	})

	state := newTestState()
	finalState, err := g.Execute(context.Background(), state)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val := finalState.Get("step1"); val != true {
		t.Errorf("expected step1=true, got %v", val)
	}
	if val := finalState.Get("step2"); val != true {
		t.Errorf("expected step2=true, got %v", val)
	}
}
