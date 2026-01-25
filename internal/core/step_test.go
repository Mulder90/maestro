package core

import (
	"context"
	"testing"
)

func TestMapVariables(t *testing.T) {
	vars := NewVariables()
	vars.Set("key", "value")
	val, ok := vars.Get("key")
	if !ok || val != "value" {
		t.Errorf("expected 'value', got %v", val)
	}
	_, ok = vars.Get("missing")
	if ok {
		t.Error("expected not found")
	}
}

func TestContextWithActorID(t *testing.T) {
	ctx := context.Background()
	if id := ActorIDFromContext(ctx); id != 0 {
		t.Errorf("expected 0, got %d", id)
	}
	ctx = ContextWithActorID(ctx, 42)
	if id := ActorIDFromContext(ctx); id != 42 {
		t.Errorf("expected 42, got %d", id)
	}
}
