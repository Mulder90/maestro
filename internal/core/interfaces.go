// Package core defines the fundamental interfaces and types for BurstSmith.
package core

import (
	"context"
	"time"
)

// Event represents a single measurement from an actor's workflow step.
type Event struct {
	ActorID    int
	Timestamp  time.Time
	Step       string
	Protocol   string        // "http", "grpc", "websocket"
	Duration   time.Duration
	Success    bool
	Error      string
	StatusCode int   // Protocol-specific status (HTTP 200, gRPC 0=OK)
	BytesSent  int64 // Request size for throughput metrics
	BytesRecv  int64 // Response size for throughput metrics
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
