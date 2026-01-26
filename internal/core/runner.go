package core

import (
	"context"
	"errors"
)

// ErrMaxIterationsReached indicates the runner hit its iteration limit.
var ErrMaxIterationsReached = errors.New("max iterations reached")

// NullReporter discards all events (used during warmup).
var NullReporter Reporter = nullReporter{}

type nullReporter struct{}

func (nullReporter) Report(Event) {}

// RunnerConfig controls execution behavior.
type RunnerConfig struct {
	MaxIterations int // 0 = unlimited
	WarmupIters   int // iterations before metrics count (per-actor)
}

// Runner controls iteration-level workflow execution.
// A Runner is NOT safe for concurrent use; each actor goroutine must have its own Runner.
type Runner struct {
	workflow  Workflow
	reporter  Reporter
	coord     Coordinator
	actorID   int
	config    RunnerConfig
	iteration int
}

// NewRunner creates a Runner for a single actor.
// Each actor goroutine should create its own Runner instance.
func NewRunner(workflow Workflow, reporter Reporter, coord Coordinator, actorID int, config RunnerConfig) *Runner {
	return &Runner{
		workflow: workflow,
		reporter: reporter,
		coord:    coord,
		actorID:  actorID,
		config:   config,
	}
}

// RunIteration executes one complete workflow iteration.
// Returns nil on success, ErrMaxIterationsReached when limit hit, or workflow error.
func (r *Runner) RunIteration(ctx context.Context) error {
	// Check max iterations before running
	if r.config.MaxIterations > 0 && r.iteration >= r.config.MaxIterations {
		return ErrMaxIterationsReached
	}

	// Select reporter based on warmup state
	rep := r.reporter
	if r.iteration < r.config.WarmupIters {
		rep = NullReporter
	}

	// Execute workflow
	err := r.workflow.Run(ctx, r.actorID, r.coord, rep)
	r.iteration++
	return err
}

// Iteration returns current iteration count (1-indexed, after RunIteration completes).
func (r *Runner) Iteration() int {
	return r.iteration
}

// IsWarmup returns true if still in warmup phase.
func (r *Runner) IsWarmup() bool {
	return r.iteration < r.config.WarmupIters
}
