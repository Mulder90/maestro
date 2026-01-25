package ratelimit

import (
	"testing"
	"time"

	"burstsmith/internal/config"
	"burstsmith/internal/core"
)

func TestPhaseManager_SteadyPhase(t *testing.T) {
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "steady", Duration: 1 * time.Second, Actors: 10},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

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
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "ramp", Duration: 100 * time.Millisecond, StartActors: 0, EndActors: 10},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

	// At start, should be 0
	actors := pm.TargetActors()
	if actors != 0 {
		t.Errorf("expected 0 actors at start, got %d", actors)
	}

	// Advance to midpoint (50ms)
	clock.Advance(50 * time.Millisecond)
	actors = pm.TargetActors()
	if actors != 5 {
		t.Errorf("expected 5 actors at midpoint, got %d", actors)
	}

	// Advance past end (another 60ms = 110ms total)
	clock.Advance(60 * time.Millisecond)
	if !pm.IsComplete() {
		t.Error("expected phase to be complete")
	}
}

func TestPhaseManager_MultiplePhases(t *testing.T) {
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "first", Duration: 50 * time.Millisecond, Actors: 5},
		{Name: "second", Duration: 50 * time.Millisecond, Actors: 10},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

	// In first phase
	phase := pm.CurrentPhase()
	if phase == nil || phase.Name != "first" {
		t.Errorf("expected phase 'first', got %v", phase)
	}
	if pm.TargetActors() != 5 {
		t.Errorf("expected 5 actors, got %d", pm.TargetActors())
	}

	// Advance past first phase
	clock.Advance(60 * time.Millisecond)

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
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "limited", Duration: 100 * time.Millisecond, Actors: 5, RPS: 100},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

	if pm.CurrentRPS() != 100 {
		t.Errorf("expected RPS 100, got %d", pm.CurrentRPS())
	}
}

func TestPhaseManager_IsComplete(t *testing.T) {
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "short", Duration: 50 * time.Millisecond, Actors: 5},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

	if pm.IsComplete() {
		t.Error("expected phase not to be complete initially")
	}

	clock.Advance(60 * time.Millisecond)

	if !pm.IsComplete() {
		t.Error("expected phase to be complete after duration")
	}
}

func TestPhaseManager_CurrentPhaseIndex(t *testing.T) {
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "first", Duration: 50 * time.Millisecond, Actors: 5},
		{Name: "second", Duration: 50 * time.Millisecond, Actors: 10},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

	if pm.CurrentPhaseIndex() != 0 {
		t.Errorf("expected phase index 0, got %d", pm.CurrentPhaseIndex())
	}

	clock.Advance(60 * time.Millisecond)

	if pm.CurrentPhaseIndex() != 1 {
		t.Errorf("expected phase index 1, got %d", pm.CurrentPhaseIndex())
	}

	clock.Advance(60 * time.Millisecond)

	if pm.CurrentPhaseIndex() != 2 {
		t.Errorf("expected phase index 2 (complete), got %d", pm.CurrentPhaseIndex())
	}
}

func TestPhaseManager_RampPhase_Interpolation(t *testing.T) {
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "ramp", Duration: 100 * time.Millisecond, StartActors: 0, EndActors: 100},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

	// Test precise interpolation at various points
	testCases := []struct {
		elapsed  time.Duration
		expected int
	}{
		{0, 0},
		{10 * time.Millisecond, 10},
		{25 * time.Millisecond, 25},
		{50 * time.Millisecond, 50},
		{75 * time.Millisecond, 75},
		{99 * time.Millisecond, 99},
	}

	for _, tc := range testCases {
		clock.Set(pm.startTime.Add(tc.elapsed))
		actors := pm.TargetActors()
		if actors != tc.expected {
			t.Errorf("at %v: expected %d actors, got %d", tc.elapsed, tc.expected, actors)
		}
	}
}

func TestPhaseManager_RampPhase_Descending(t *testing.T) {
	clock := core.NewFakeClock(time.Now())
	phases := []config.Phase{
		{Name: "ramp_down", Duration: 100 * time.Millisecond, StartActors: 100, EndActors: 0},
	}
	pm := NewPhaseManagerWithClock(phases, clock)

	// At start, should be 100
	if pm.TargetActors() != 100 {
		t.Errorf("expected 100 actors at start, got %d", pm.TargetActors())
	}

	// At midpoint, should be 50
	clock.Advance(50 * time.Millisecond)
	if pm.TargetActors() != 50 {
		t.Errorf("expected 50 actors at midpoint, got %d", pm.TargetActors())
	}

	// Near end, should be close to 0
	clock.Advance(49 * time.Millisecond)
	actors := pm.TargetActors()
	if actors != 1 {
		t.Errorf("expected 1 actor near end, got %d", actors)
	}
}

// TestPhaseManager_WithRealClock verifies the default constructor still works
func TestPhaseManager_WithRealClock(t *testing.T) {
	phases := []config.Phase{
		{Name: "steady", Duration: 1 * time.Second, Actors: 10},
	}
	pm := NewPhaseManager(phases)

	// Should work with real clock
	if pm.TargetActors() != 10 {
		t.Errorf("expected 10 actors, got %d", pm.TargetActors())
	}
	if pm.IsComplete() {
		t.Error("expected phase not to be complete")
	}
}
