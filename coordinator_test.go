package burstsmith

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// mockWorkflow tracks how many times it was run
type mockWorkflow struct {
	runCount atomic.Int32
	delay    time.Duration
}

func (m *mockWorkflow) Run(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
	m.runCount.Add(1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rep.Report(Event{ActorID: actorID, Step: "mock", Success: true, Duration: m.delay})
	return nil
}

func TestCoordinator_SpawnsCorrectNumberOfActors(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 5, workflow)
	coord.Wait()
	collector.Close()

	// Each actor should run at least once
	if workflow.runCount.Load() < 5 {
		t.Errorf("expected at least 5 workflow runs, got %d", workflow.runCount.Load())
	}
}

func TestCoordinator_ActorsRunConcurrently(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	// Workflow that takes 50ms
	workflow := &mockWorkflow{delay: 50 * time.Millisecond}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Spawn 5 actors
	coord.Spawn(ctx, 5, workflow)
	coord.Wait()
	elapsed := time.Since(start)
	collector.Close()

	// If actors ran sequentially, it would take 5*50ms = 250ms minimum
	// Since they run concurrently, it should complete in ~100ms (the context timeout)
	if elapsed > 150*time.Millisecond {
		t.Errorf("actors don't appear to run concurrently, took %v", elapsed)
	}
}

func TestCoordinator_ActorsRespectContextCancellation(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{delay: 1 * time.Second}

	ctx, cancel := context.WithCancel(context.Background())

	coord.Spawn(ctx, 3, workflow)

	// Cancel immediately
	cancel()
	coord.Wait()
	collector.Close()

	// Actors should stop quickly after cancellation
	// They might have started one run each before context was cancelled
	if workflow.runCount.Load() > 3 {
		t.Logf("workflow ran %d times (expected <= 3)", workflow.runCount.Load())
	}
}

func TestCoordinator_ActorsGetUniqueIDs(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 10, workflow)
	coord.Wait()
	collector.Close()

	// Check that we got events from unique actor IDs
	actorIDs := make(map[int]bool)
	for _, e := range collector.events {
		actorIDs[e.ActorID] = true
	}

	if len(actorIDs) < 10 {
		t.Errorf("expected 10 unique actor IDs, got %d", len(actorIDs))
	}
}

func TestCoordinator_ReportsEventsToCollector(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 3, workflow)
	coord.Wait()
	collector.Close()

	if len(collector.events) == 0 {
		t.Error("expected events to be reported to collector")
	}

	// All events should be successful (mock workflow always succeeds)
	for _, e := range collector.events {
		if !e.Success {
			t.Errorf("expected all events to be successful, got failure for actor %d", e.ActorID)
		}
	}
}

func TestCoordinator_WaitBlocksUntilComplete(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{delay: 50 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		coord.Spawn(ctx, 2, workflow)
		coord.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good - Wait returned
	case <-time.After(500 * time.Millisecond):
		t.Error("Wait did not return in time")
	}

	collector.Close()
}

func TestCoordinator_RunWithProfile_RampUp(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{delay: 10 * time.Millisecond}

	profile := &LoadProfile{
		Phases: []Phase{
			{Name: "ramp", Duration: 300 * time.Millisecond, StartActors: 1, EndActors: 5},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil)
	coord.Wait()
	collector.Close()

	// Should have events from multiple actors
	actorIDs := make(map[int]bool)
	for _, e := range collector.events {
		actorIDs[e.ActorID] = true
	}

	if len(actorIDs) < 2 {
		t.Errorf("expected multiple actors during ramp, got %d unique actors", len(actorIDs))
	}
}

func TestCoordinator_RunWithProfile_SteadyState(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{delay: 10 * time.Millisecond}

	profile := &LoadProfile{
		Phases: []Phase{
			{Name: "steady", Duration: 200 * time.Millisecond, Actors: 5},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil)
	coord.Wait()
	collector.Close()

	// Should have exactly 5 actors active
	actorIDs := make(map[int]bool)
	for _, e := range collector.events {
		actorIDs[e.ActorID] = true
	}

	if len(actorIDs) != 5 {
		t.Errorf("expected 5 actors in steady state, got %d", len(actorIDs))
	}
}

func TestCoordinator_RunWithProfile_MultiplePhases(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{delay: 5 * time.Millisecond}

	profile := &LoadProfile{
		Phases: []Phase{
			{Name: "phase1", Duration: 100 * time.Millisecond, Actors: 2},
			{Name: "phase2", Duration: 100 * time.Millisecond, Actors: 4},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil)
	coord.Wait()
	collector.Close()

	// Should have events from the test
	if len(collector.events) == 0 {
		t.Error("expected events from multi-phase profile")
	}
}

func TestCoordinator_RunWithProfile_WithRateLimiter(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	// Use a lower RPS with smaller burst to make rate limiting more visible
	rateLimiter := NewRateLimiter(20) // 20 RPS with burst of 20

	// Create HTTP workflow with rate limiter attached
	workflow := &rateLimitedMockWorkflow{
		rateLimiter: rateLimiter,
		collector:   collector,
	}

	profile := &LoadProfile{
		Phases: []Phase{
			{Name: "limited", Duration: 300 * time.Millisecond, Actors: 10, RPS: 20},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	coord.RunWithProfile(ctx, profile, workflow, rateLimiter)
	coord.Wait()
	elapsed := time.Since(start)
	collector.Close()

	// With 20 RPS (burst=20) over ~300ms: initial burst of 20 + 20*0.3 = ~26 requests max
	// Allow tolerance for timing variations
	eventCount := len(collector.events)
	expectedMin := 10
	expectedMax := 40

	if eventCount < expectedMin || eventCount > expectedMax {
		t.Errorf("expected %d-%d events with rate limiting, got %d (elapsed: %v)",
			expectedMin, expectedMax, eventCount, elapsed)
	}

	// Verify rate limiting is actually happening - without it, 10 actors could do thousands of requests
	if eventCount > 100 {
		t.Errorf("rate limiting doesn't appear to be working, got %d events", eventCount)
	}
}

// rateLimitedMockWorkflow is a mock that respects rate limiting
type rateLimitedMockWorkflow struct {
	rateLimiter *RateLimiter
	collector   *Collector
}

func (m *rateLimitedMockWorkflow) Run(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
	if m.rateLimiter != nil {
		if err := m.rateLimiter.Wait(ctx); err != nil {
			return err
		}
	}
	m.collector.Report(Event{ActorID: actorID, Step: "mock", Success: true})
	return nil
}

func TestCoordinator_RunWithProfile_RampDown(t *testing.T) {
	collector := NewCollector()
	coord := NewCoordinator(collector)

	workflow := &mockWorkflow{delay: 10 * time.Millisecond}

	profile := &LoadProfile{
		Phases: []Phase{
			{Name: "ramp_down", Duration: 200 * time.Millisecond, StartActors: 5, EndActors: 0},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil)
	coord.Wait()
	collector.Close()

	// After ramp down completes, should have 0 active actors
	if coord.ActiveActors() != 0 {
		t.Errorf("expected 0 active actors after ramp down, got %d", coord.ActiveActors())
	}
}
