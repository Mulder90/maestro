package burstsmith

import (
	"testing"
	"time"
)

func TestPhaseManager_SteadyPhase(t *testing.T) {
	phases := []Phase{
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
	if pm.PhaseName() != "steady" {
		t.Errorf("expected phase name 'steady', got %q", pm.PhaseName())
	}
}

func TestPhaseManager_RampPhase(t *testing.T) {
	phases := []Phase{
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
	phases := []Phase{
		{Name: "first", Duration: 50 * time.Millisecond, Actors: 5},
		{Name: "second", Duration: 50 * time.Millisecond, Actors: 10},
	}
	pm := NewPhaseManager(phases)

	// In first phase
	if pm.PhaseName() != "first" {
		t.Errorf("expected phase 'first', got %q", pm.PhaseName())
	}
	if pm.TargetActors() != 5 {
		t.Errorf("expected 5 actors, got %d", pm.TargetActors())
	}

	// Wait for first phase to end
	time.Sleep(60 * time.Millisecond)

	// Now in second phase
	if pm.PhaseName() != "second" {
		t.Errorf("expected phase 'second', got %q", pm.PhaseName())
	}
	if pm.TargetActors() != 10 {
		t.Errorf("expected 10 actors, got %d", pm.TargetActors())
	}
}

func TestPhaseManager_RPS(t *testing.T) {
	phases := []Phase{
		{Name: "limited", Duration: 100 * time.Millisecond, Actors: 5, RPS: 100},
	}
	pm := NewPhaseManager(phases)

	if pm.CurrentRPS() != 100 {
		t.Errorf("expected RPS 100, got %d", pm.CurrentRPS())
	}
}

func TestLoadProfile_TotalDuration(t *testing.T) {
	lp := &LoadProfile{
		Phases: []Phase{
			{Duration: 10 * time.Second},
			{Duration: 20 * time.Second},
			{Duration: 5 * time.Second},
		},
	}

	expected := 35 * time.Second
	if lp.TotalDuration() != expected {
		t.Errorf("expected %v, got %v", expected, lp.TotalDuration())
	}
}
