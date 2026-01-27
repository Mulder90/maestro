package coordinator_test

import (
	"context"
	"fmt"
	"time"

	"maestro/internal/collector"
	"maestro/internal/coordinator"
	"maestro/internal/core"
)

// simpleWorkflow is a minimal workflow for examples
type simpleWorkflow struct{}

func (s *simpleWorkflow) Run(ctx context.Context, actorID int, coord core.Coordinator, rep core.Reporter) error {
	rep.Report(core.Event{
		ActorID:  actorID,
		Step:     "work",
		Success:  true,
		Duration: time.Millisecond,
	})
	return nil
}

func ExampleNewCoordinator() {
	// Create collector and coordinator
	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	// Create a workflow
	workflow := &simpleWorkflow{}

	// Run with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Spawn 3 actors
	coord.Spawn(ctx, 3, workflow)

	// Wait for all actors to complete
	coord.Wait()
	c.Close()

	fmt.Printf("Collected %d events from actors\n", len(c.Events()))
}

func ExampleCoordinator_SpawnWithConfig() {
	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)
	workflow := &simpleWorkflow{}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Spawn with iteration limits
	config := core.RunnerConfig{
		MaxIterations: 5,  // Each actor runs 5 iterations
		WarmupIters:   1,  // First iteration is warmup (not reported)
	}
	coord.SpawnWithConfig(ctx, 2, workflow, config)
	coord.Wait()
	c.Close()

	// 2 actors * (5 max - 1 warmup) = 8 events
	fmt.Printf("Events after warmup: %d\n", len(c.Events()))
	// Output: Events after warmup: 8
}
