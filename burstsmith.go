package burstsmith

import (
	"context"
	"time"
)

// Event represents a single measurement from an actor's workflow step.
type Event struct {
	ActorID   int
	Timestamp time.Time
	Step      string
	Duration  time.Duration
	Success   bool
	Error     string
}

// Workflow defines a user journey that an actor executes.
// Each workflow models a complete user journey with all its complexity.
type Workflow interface {
	Run(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error
}

// Coordinator spawns and manages actors.
type Coordinator interface {
	Spawn(ctx context.Context, count int, workflow Workflow)
}

// Reporter is the interface actors use to send events to the Collector.
type Reporter interface {
	Report(Event)
}
