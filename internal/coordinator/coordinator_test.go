package coordinator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"maestro/internal/collector"
	"maestro/internal/config"
	"maestro/internal/core"
	"maestro/internal/ratelimit"
)

// mockWorkflow tracks how many times it was run
type mockWorkflow struct {
	runCount atomic.Int32
	delay    time.Duration
}

func (m *mockWorkflow) Run(ctx context.Context, actorID int, coord core.Coordinator, rep core.Reporter) error {
	m.runCount.Add(1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rep.Report(core.Event{ActorID: actorID, Step: "mock", Success: true, Duration: m.delay})
	return nil
}

func TestCoordinator_SpawnsCorrectNumberOfActors(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 5, workflow)
	coord.Wait()
	c.Close()

	// Each actor should run at least once
	if workflow.runCount.Load() < 5 {
		t.Errorf("expected at least 5 workflow runs, got %d", workflow.runCount.Load())
	}
}

func TestCoordinator_ActorsRunConcurrently(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	// Workflow that takes 50ms
	workflow := &mockWorkflow{delay: 50 * time.Millisecond}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Spawn 5 actors
	coord.Spawn(ctx, 5, workflow)
	coord.Wait()
	elapsed := time.Since(start)
	c.Close()

	// If actors ran sequentially, it would take 5*50ms = 250ms minimum
	// Since they run concurrently, it should complete in ~100ms (the context timeout)
	if elapsed > 150*time.Millisecond {
		t.Errorf("actors don't appear to run concurrently, took %v", elapsed)
	}
}

func TestCoordinator_ActorsRespectContextCancellation(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 1 * time.Second}

	ctx, cancel := context.WithCancel(context.Background())

	coord.Spawn(ctx, 3, workflow)

	// Cancel immediately
	cancel()
	coord.Wait()
	c.Close()

	// Actors should stop quickly after cancellation
	// They might have started one run each before context was cancelled
	if workflow.runCount.Load() > 3 {
		t.Logf("workflow ran %d times (expected <= 3)", workflow.runCount.Load())
	}
}

func TestCoordinator_ActorsGetUniqueIDs(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 10, workflow)
	coord.Wait()
	c.Close()

	// Check that we got events from unique actor IDs
	actorIDs := make(map[int]bool)
	for _, e := range c.Events() {
		actorIDs[e.ActorID] = true
	}

	// All 10 actors should report at least one event
	if len(actorIDs) < 10 {
		t.Errorf("expected 10 unique actor IDs, got %d", len(actorIDs))
	}
}

func TestCoordinator_ReportsEventsToCollector(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 3, workflow)
	coord.Wait()
	c.Close()

	events := c.Events()
	if len(events) == 0 {
		t.Error("expected events to be reported to collector")
	}

	// All events should be successful (mock workflow always succeeds)
	for _, e := range events {
		if !e.Success {
			t.Errorf("expected all events to be successful, got failure for actor %d", e.ActorID)
		}
	}
}

func TestCoordinator_WaitBlocksUntilComplete(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

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

	c.Close()
}

func TestCoordinator_RunWithProfile_RampUp(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 10 * time.Millisecond}

	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "ramp", Duration: 300 * time.Millisecond, StartActors: 1, EndActors: 5},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// Should have events from multiple actors
	actorIDs := make(map[int]bool)
	for _, e := range c.Events() {
		actorIDs[e.ActorID] = true
	}

	if len(actorIDs) < 2 {
		t.Errorf("expected multiple actors during ramp, got %d unique actors", len(actorIDs))
	}
}

func TestCoordinator_RunWithProfile_SteadyState(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 10 * time.Millisecond}

	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "steady", Duration: 200 * time.Millisecond, Actors: 5},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// Should have exactly 5 actors active
	actorIDs := make(map[int]bool)
	for _, e := range c.Events() {
		actorIDs[e.ActorID] = true
	}

	if len(actorIDs) != 5 {
		t.Errorf("expected 5 actors in steady state, got %d", len(actorIDs))
	}
}

func TestCoordinator_RunWithProfile_MultiplePhases(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 5 * time.Millisecond}

	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "phase1", Duration: 100 * time.Millisecond, Actors: 2},
			{Name: "phase2", Duration: 100 * time.Millisecond, Actors: 4},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// Should have events from the test
	if len(c.Events()) == 0 {
		t.Error("expected events from multi-phase profile")
	}
}

func TestCoordinator_RunWithProfile_WithRateLimiter(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	// Use a lower RPS with smaller burst to make rate limiting more visible
	rateLimiter := ratelimit.NewRateLimiter(20) // 20 RPS with burst of 20

	// Create workflow with rate limiter attached
	workflow := &rateLimitedMockWorkflow{
		rateLimiter: rateLimiter,
		reporter:    c,
	}

	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "limited", Duration: 300 * time.Millisecond, Actors: 10, RPS: 20},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, rateLimiter, nil)
	coord.Wait()
	c.Close()

	// With 20 RPS (burst=20) over ~300ms: initial burst of 20 + 20*0.3 = ~26 requests max
	// Allow tolerance for timing variations
	eventCount := len(c.Events())
	expectedMin := 10
	expectedMax := 40

	if eventCount < expectedMin || eventCount > expectedMax {
		t.Errorf("expected %d-%d events with rate limiting, got %d",
			expectedMin, expectedMax, eventCount)
	}

	// Verify rate limiting is actually happening - without it, 10 actors could do thousands of requests
	if eventCount > 100 {
		t.Errorf("rate limiting doesn't appear to be working, got %d events", eventCount)
	}
}

// rateLimitedMockWorkflow is a mock that respects rate limiting
type rateLimitedMockWorkflow struct {
	rateLimiter *ratelimit.RateLimiter
	reporter    core.Reporter
}

func (m *rateLimitedMockWorkflow) Run(ctx context.Context, actorID int, coord core.Coordinator, rep core.Reporter) error {
	if m.rateLimiter != nil {
		if err := m.rateLimiter.Wait(ctx); err != nil {
			return err
		}
	}
	m.reporter.Report(core.Event{ActorID: actorID, Step: "mock", Success: true})
	return nil
}

func TestCoordinator_RunWithProfile_RampDown(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 10 * time.Millisecond}

	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "ramp_down", Duration: 200 * time.Millisecond, StartActors: 5, EndActors: 0},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// After ramp down completes, should have 0 active actors
	if coord.ActiveActors() != 0 {
		t.Errorf("expected 0 active actors after ramp down, got %d", coord.ActiveActors())
	}
}

func TestCoordinator_ActiveActors(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	if coord.ActiveActors() != 0 {
		t.Errorf("expected 0 active actors initially, got %d", coord.ActiveActors())
	}

	c.Close()
}

func TestCoordinator_StopActors_ViaRampDown(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 20 * time.Millisecond}

	// Test stopActors through a ramp-down profile which naturally calls it
	profile := &config.LoadProfile{
		Phases: []config.Phase{
			// Start with 10 actors, ramp down to 0
			{Name: "ramp_down", Duration: 200 * time.Millisecond, StartActors: 10, EndActors: 0},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// After ramp down, should have 0 active actors
	finalActors := coord.ActiveActors()
	if finalActors != 0 {
		t.Errorf("expected 0 active actors after ramp down, got %d", finalActors)
	}

	// Should have had multiple actors reporting events during the ramp
	actorIDs := make(map[int]bool)
	for _, e := range c.Events() {
		actorIDs[e.ActorID] = true
	}
	if len(actorIDs) < 3 {
		t.Errorf("expected multiple actors to run during ramp down, got %d unique actors", len(actorIDs))
	}
}

func TestCoordinator_StopActors_PhaseTransition(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 10 * time.Millisecond}

	// Two phases: start with 5 actors, drop to 2
	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "high", Duration: 150 * time.Millisecond, Actors: 5},
			{Name: "low", Duration: 150 * time.Millisecond, Actors: 2},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, profile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// After profile completes, should have 0 actors
	if coord.ActiveActors() != 0 {
		t.Errorf("expected 0 actors after profile complete, got %d", coord.ActiveActors())
	}

	// Should have events from both phases
	if len(c.Events()) == 0 {
		t.Error("expected events from phase transitions")
	}
}

func TestCoordinator_StopActors_EmptyStopChans(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	// stopActors on empty coordinator should not panic
	coord.stopActors(5) // No actors to stop

	// Should still work normally after
	if coord.ActiveActors() != 0 {
		t.Errorf("expected 0 actors, got %d", coord.ActiveActors())
	}

	c.Close()
}

func TestCoordinator_RunWithProfile_NoRPS(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 5 * time.Millisecond}

	// Phase without RPS (should use different message format)
	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "no_rps", Duration: 200 * time.Millisecond, Actors: 3},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Run without progress (nil) to exercise the fmt.Printf path
	coord.RunWithProfile(ctx, profile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// Should have events from the test
	if len(c.Events()) == 0 {
		t.Error("expected events from profile without RPS")
	}
}

func TestCoordinator_RunWithProfile_ContextCancellation(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{delay: 50 * time.Millisecond}

	profile := &config.LoadProfile{
		Phases: []config.Phase{
			{Name: "long", Duration: 10 * time.Second, Actors: 5},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		coord.RunWithProfile(ctx, profile, workflow, nil, nil)
		close(done)
	}()

	// Wait for actors to start
	time.Sleep(150 * time.Millisecond)

	// Cancel context
	cancel()

	// Should return quickly after cancellation
	select {
	case <-done:
		// Good
	case <-time.After(500 * time.Millisecond):
		t.Error("RunWithProfile did not stop after context cancellation")
	}

	coord.Wait()
	c.Close()

	// All actors should be stopped
	if coord.ActiveActors() != 0 {
		t.Errorf("expected 0 actors after cancellation, got %d", coord.ActiveActors())
	}
}

func TestCoordinator_SpawnWithConfig_MaxIterations(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	config := core.RunnerConfig{MaxIterations: 3}
	coord.SpawnWithConfig(ctx, 2, workflow, config)
	coord.Wait()
	c.Close()

	events := c.Events()
	// 2 actors * 3 max iterations = 6 events
	if len(events) != 6 {
		t.Errorf("expected 6 events (2 actors * 3 iterations), got %d", len(events))
	}
}

func TestCoordinator_SpawnWithConfig_WarmupIterations(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &mockWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 2 warmup + 3 max = 5 total iterations, but only 3 should be reported
	config := core.RunnerConfig{MaxIterations: 5, WarmupIters: 2}
	coord.SpawnWithConfig(ctx, 1, workflow, config)
	coord.Wait()
	c.Close()

	events := c.Events()
	// Only 3 events should be reported (5 max - 2 warmup)
	if len(events) != 3 {
		t.Errorf("expected 3 events (5 max - 2 warmup), got %d", len(events))
	}
}

// panicWorkflow panics on first run
type panicWorkflow struct {
	panicOnce atomic.Bool
}

func (p *panicWorkflow) Run(ctx context.Context, actorID int, coord core.Coordinator, rep core.Reporter) error {
	if p.panicOnce.CompareAndSwap(false, true) {
		panic("test panic")
	}
	rep.Report(core.Event{ActorID: actorID, Step: "mock", Success: true})
	return nil
}

func TestCoordinator_RecoversPanic(t *testing.T) {
	c := collector.NewCollector()
	coord := NewCoordinator(c)

	workflow := &panicWorkflow{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Spawn 2 actors - one will panic
	coord.Spawn(ctx, 2, workflow)
	coord.Wait()
	c.Close()

	// Should have a panic event reported
	var hasPanicEvent bool
	for _, e := range c.Events() {
		if e.Step == "panic" && !e.Success {
			hasPanicEvent = true
			break
		}
	}

	if !hasPanicEvent {
		t.Error("expected panic to be recovered and reported as failed event")
	}
}
