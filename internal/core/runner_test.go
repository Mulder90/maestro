package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

// mockWorkflow is a simple workflow for testing
type mockWorkflow struct {
	runFunc func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error
}

func (m *mockWorkflow) Run(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
	if m.runFunc != nil {
		return m.runFunc(ctx, actorID, coord, rep)
	}
	return nil
}

// mockReporter collects events for testing
type mockReporter struct {
	events []Event
}

func (m *mockReporter) Report(e Event) {
	m.events = append(m.events, e)
}

func TestRunner_MaxIterations(t *testing.T) {
	var callCount int
	workflow := &mockWorkflow{
		runFunc: func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
			callCount++
			rep.Report(Event{Step: "mock", Success: true})
			return nil
		},
	}

	reporter := &mockReporter{}
	runner := NewRunner(workflow, reporter, nil, 1, RunnerConfig{
		MaxIterations: 3,
	})

	ctx := context.Background()
	for {
		err := runner.RunIteration(ctx)
		if errors.Is(err, ErrMaxIterationsReached) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// Exactly 3 iterations executed
	if runner.Iteration() != 3 {
		t.Errorf("expected 3 iterations, got %d", runner.Iteration())
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
	if len(reporter.events) != 3 {
		t.Errorf("expected 3 events, got %d", len(reporter.events))
	}
}

func TestRunner_WarmupExcludesMetrics(t *testing.T) {
	reporter := &mockReporter{}
	workflow := &mockWorkflow{
		runFunc: func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
			rep.Report(Event{Step: "mock", Success: true})
			return nil
		},
	}

	runner := NewRunner(workflow, reporter, nil, 1, RunnerConfig{
		MaxIterations: 5,
		WarmupIters:   2,
	})

	ctx := context.Background()
	for {
		err := runner.RunIteration(ctx)
		if errors.Is(err, ErrMaxIterationsReached) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// 5 total iterations, but only 3 reported (after warmup)
	if runner.Iteration() != 5 {
		t.Errorf("expected 5 iterations, got %d", runner.Iteration())
	}
	if len(reporter.events) != 3 {
		t.Errorf("expected 3 events (excluding warmup), got %d", len(reporter.events))
	}
}

func TestRunner_IsWarmup(t *testing.T) {
	workflow := &mockWorkflow{}
	reporter := &mockReporter{}

	runner := NewRunner(workflow, reporter, nil, 1, RunnerConfig{
		MaxIterations: 5,
		WarmupIters:   2,
	})

	ctx := context.Background()

	// Before any iterations
	if !runner.IsWarmup() {
		t.Error("expected IsWarmup() to be true before warmup completes")
	}

	// After first iteration (still in warmup)
	runner.RunIteration(ctx)
	if !runner.IsWarmup() {
		t.Error("expected IsWarmup() to be true during warmup (iteration 1)")
	}

	// After second iteration (warmup complete)
	runner.RunIteration(ctx)
	if runner.IsWarmup() {
		t.Error("expected IsWarmup() to be false after warmup completes (iteration 2)")
	}
}

func TestRunner_Iteration(t *testing.T) {
	workflow := &mockWorkflow{}
	reporter := &mockReporter{}

	runner := NewRunner(workflow, reporter, nil, 1, RunnerConfig{
		MaxIterations: 3,
	})

	ctx := context.Background()

	if runner.Iteration() != 0 {
		t.Errorf("expected iteration 0 before any runs, got %d", runner.Iteration())
	}

	runner.RunIteration(ctx)
	if runner.Iteration() != 1 {
		t.Errorf("expected iteration 1 after first run, got %d", runner.Iteration())
	}

	runner.RunIteration(ctx)
	if runner.Iteration() != 2 {
		t.Errorf("expected iteration 2 after second run, got %d", runner.Iteration())
	}

	runner.RunIteration(ctx)
	if runner.Iteration() != 3 {
		t.Errorf("expected iteration 3 after third run, got %d", runner.Iteration())
	}
}

func TestRunner_UnlimitedIterations(t *testing.T) {
	var callCount atomic.Int32
	workflow := &mockWorkflow{
		runFunc: func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
			callCount.Add(1)
			return nil
		},
	}

	runner := NewRunner(workflow, &mockReporter{}, nil, 1, RunnerConfig{
		MaxIterations: 0, // unlimited
	})

	ctx := context.Background()

	// Run many iterations - should never return ErrMaxIterationsReached
	for i := 0; i < 100; i++ {
		err := runner.RunIteration(ctx)
		if errors.Is(err, ErrMaxIterationsReached) {
			t.Fatal("unexpected ErrMaxIterationsReached with unlimited iterations")
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	if callCount.Load() != 100 {
		t.Errorf("expected 100 calls, got %d", callCount.Load())
	}
}

func TestRunner_WorkflowError(t *testing.T) {
	expectedErr := errors.New("workflow error")
	workflow := &mockWorkflow{
		runFunc: func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
			return expectedErr
		},
	}

	runner := NewRunner(workflow, &mockReporter{}, nil, 1, RunnerConfig{
		MaxIterations: 5,
	})

	ctx := context.Background()
	err := runner.RunIteration(ctx)

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected workflow error, got %v", err)
	}

	// Iteration should still increment even on error
	if runner.Iteration() != 1 {
		t.Errorf("expected iteration 1 after error, got %d", runner.Iteration())
	}
}

func TestRunner_ActorIDPassedToWorkflow(t *testing.T) {
	var receivedActorID int
	workflow := &mockWorkflow{
		runFunc: func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
			receivedActorID = actorID
			return nil
		},
	}

	expectedActorID := 42
	runner := NewRunner(workflow, &mockReporter{}, nil, expectedActorID, RunnerConfig{})

	ctx := context.Background()
	runner.RunIteration(ctx)

	if receivedActorID != expectedActorID {
		t.Errorf("expected actorID %d, got %d", expectedActorID, receivedActorID)
	}
}

func TestNullReporter(t *testing.T) {
	// NullReporter should not panic when Report is called
	NullReporter.Report(Event{Step: "test", Success: true})
	// If we get here without panic, the test passes
}

func TestRunner_ContextCancellation(t *testing.T) {
	workflow := &mockWorkflow{
		runFunc: func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		},
	}

	runner := NewRunner(workflow, &mockReporter{}, nil, 1, RunnerConfig{
		MaxIterations: 100,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := runner.RunIteration(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunner_WarmupWithZeroIterations(t *testing.T) {
	reporter := &mockReporter{}
	workflow := &mockWorkflow{
		runFunc: func(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
			rep.Report(Event{Step: "mock", Success: true})
			return nil
		},
	}

	runner := NewRunner(workflow, reporter, nil, 1, RunnerConfig{
		MaxIterations: 3,
		WarmupIters:   0, // No warmup
	})

	ctx := context.Background()
	for {
		err := runner.RunIteration(ctx)
		if errors.Is(err, ErrMaxIterationsReached) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	// All 3 iterations should be reported (no warmup)
	if len(reporter.events) != 3 {
		t.Errorf("expected 3 events with no warmup, got %d", len(reporter.events))
	}
}
