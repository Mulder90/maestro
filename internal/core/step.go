package core

import (
	"context"
	"time"
)

// Step represents a single executable action in a workflow.
type Step interface {
	Execute(ctx context.Context, vars Variables) (Result, error)
	Name() string
}

// Result represents the outcome of a step execution.
type Result struct {
	Duration   time.Duration
	Success    bool
	Error      string
	StatusCode int
	BytesSent  int64
	BytesRecv  int64
	Extract    map[string]any
}

// Variables provides shared state between steps in a workflow run.
type Variables interface {
	Get(key string) (any, bool)
	Set(key string, value any)
}

// MapVariables is a simple map-based Variables implementation.
type MapVariables struct {
	data map[string]any
}

func NewVariables() *MapVariables {
	return &MapVariables{data: make(map[string]any)}
}

func (v *MapVariables) Get(key string) (any, bool) {
	val, ok := v.data[key]
	return val, ok
}

func (v *MapVariables) Set(key string, value any) {
	v.data[key] = value
}

// Context key for passing actor ID to steps.
type contextKey string

const actorIDContextKey contextKey = "actorID"

func ContextWithActorID(ctx context.Context, actorID int) context.Context {
	return context.WithValue(ctx, actorIDContextKey, actorID)
}

func ActorIDFromContext(ctx context.Context) int {
	if id, ok := ctx.Value(actorIDContextKey).(int); ok {
		return id
	}
	return 0
}
