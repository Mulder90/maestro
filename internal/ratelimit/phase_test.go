package ratelimit

import (
	"testing"
	"time"

	"burstsmith/internal/config"
)

func TestPhaseManager_SteadyPhase(t *testing.T) {
	phases := []config.Phase{
		{Name: "steady", Duration: 1 * time.Second, Actors: 10},
	}
	pm := NewPhaseManager(phases)

	// Should return 10 actors for steady phase
	actors := pm.TargetActors()
	if actors != 10 {
		t.Errorf("expected 10 actors, got %d", actors)
	}

	// Should not be complete
	if pm.IsComplete() {
		t.Error("expected phase not to be complete")
	}

	// Phase name should match
	phase := pm.CurrentPhase()
	if phase == nil || phase.Name != "steady" {
		t.Errorf("expected phase name 'steady', got %v", phase)
	}
}

func TestPhaseManager_RampPhase(t *testing.T) {
	phases := []config.Phase{
		{Name: "ramp", Duration: 100 * time.Millisecond, StartActors: 0, EndActors: 10},
	}
	pm := NewPhaseManager(phases)

	// At start, should be near 0
	actors := pm.TargetActors()
	if actors > 2 {
		t.Errorf("expected ~0 actors at start, got %d", actors)
	}

	// Wait for half the duration
	time.Sleep(50 * time.Millisecond)
	actors = pm.TargetActors()
	if actors < 3 || actors > 7 {
		t.Errorf("expected ~5 actors at midpoint, got %d", actors)
	}

	// Wait for phase to complete
	time.Sleep(60 * time.Millisecond)
	if !pm.IsComplete() {
		t.Error("expected phase to be complete")
	}
}

func TestPhaseManager_MultiplePhases(t *testing.T) {
	phases := []config.Phase{
		{Name: "first", Duration: 50 * time.Millisecond, Actors: 5},
		{Name: "second", Duration: 50 * time.Millisecond, Actors: 10},
	}
	pm := NewPhaseManager(phases)

	// In first phase
	phase := pm.CurrentPhase()
	if phase == nil || phase.Name != "first" {
		t.Errorf("expected phase 'first', got %v", phase)
	}
	if pm.TargetActors() != 5 {
		t.Errorf("expected 5 actors, got %d", pm.TargetActors())
	}

	// Wait for first phase to end
	time.Sleep(60 * time.Millisecond)

	// Now in second phase
	phase = pm.CurrentPhase()
	if phase == nil || phase.Name != "second" {
		t.Errorf("expected phase 'second', got %v", phase)
	}
	if pm.TargetActors() != 10 {
		t.Errorf("expected 10 actors, got %d", pm.TargetActors())
	}
}

func TestPhaseManager_RPS(t *testing.T) {
	phases := []config.Phase{
		{Name: "limited", Duration: 100 * time.Millisecond, Actors: 5, RPS: 100},
	}
	pm := NewPhaseManager(phases)

	if pm.CurrentRPS() != 100 {
		t.Errorf("expected RPS 100, got %d", pm.CurrentRPS())
	}
}

func TestPhaseManager_IsComplete(t *testing.T) {
	phases := []config.Phase{
		{Name: "short", Duration: 50 * time.Millisecond, Actors: 5},
	}
	pm := NewPhaseManager(phases)

	if pm.IsComplete() {
		t.Error("expected phase not to be complete initially")
	}

	time.Sleep(60 * time.Millisecond)

	if !pm.IsComplete() {
		t.Error("expected phase to be complete after duration")
	}
}

func TestPhaseManager_CurrentPhaseIndex(t *testing.T) {
	phases := []config.Phase{
		{Name: "first", Duration: 50 * time.Millisecond, Actors: 5},
		{Name: "second", Duration: 50 * time.Millisecond, Actors: 10},
	}
	pm := NewPhaseManager(phases)

	if pm.CurrentPhaseIndex() != 0 {
		t.Errorf("expected phase index 0, got %d", pm.CurrentPhaseIndex())
	}

	time.Sleep(60 * time.Millisecond)

	if pm.CurrentPhaseIndex() != 1 {
		t.Errorf("expected phase index 1, got %d", pm.CurrentPhaseIndex())
	}

	time.Sleep(60 * time.Millisecond)

	if pm.CurrentPhaseIndex() != 2 {
		t.Errorf("expected phase index 2 (complete), got %d", pm.CurrentPhaseIndex())
	}
}
