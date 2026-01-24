package burstsmith

import (
	"time"
)

// PhaseManager tracks the current phase and calculates target actor counts.
type PhaseManager struct {
	phases    []Phase
	startTime time.Time
}

// NewPhaseManager creates a new phase manager with the given phases.
func NewPhaseManager(phases []Phase) *PhaseManager {
	return &PhaseManager{
		phases:    phases,
		startTime: time.Now(),
	}
}

// CurrentPhase returns the currently active phase, or nil if all phases are complete.
func (pm *PhaseManager) CurrentPhase() *Phase {
	elapsed := time.Since(pm.startTime)
	var accumulated time.Duration

	for i := range pm.phases {
		phaseEnd := accumulated + pm.phases[i].Duration
		if elapsed < phaseEnd {
			return &pm.phases[i]
		}
		accumulated = phaseEnd
	}
	return nil
}

// CurrentPhaseIndex returns the index of the current phase (0-based), or -1 if complete.
func (pm *PhaseManager) CurrentPhaseIndex() int {
	elapsed := time.Since(pm.startTime)
	var accumulated time.Duration

	for i := range pm.phases {
		phaseEnd := accumulated + pm.phases[i].Duration
		if elapsed < phaseEnd {
			return i
		}
		accumulated = phaseEnd
	}
	return -1
}

// TargetActors returns the target number of actors for the current moment.
// For ramp phases, this interpolates between start and end actors.
func (pm *PhaseManager) TargetActors() int {
	elapsed := time.Since(pm.startTime)
	var accumulated time.Duration

	for i := range pm.phases {
		phase := &pm.phases[i]
		phaseEnd := accumulated + phase.Duration
		if elapsed < phaseEnd {
			phaseElapsed := elapsed - accumulated
			return pm.calculateActors(phase, phaseElapsed)
		}
		accumulated = phaseEnd
	}
	return 0 // All phases complete
}

// calculateActors determines actor count for a phase at a given elapsed time.
func (pm *PhaseManager) calculateActors(phase *Phase, phaseElapsed time.Duration) int {
	// If Actors is set (steady state), use it directly
	if phase.Actors > 0 {
		return phase.Actors
	}

	// Otherwise, interpolate between StartActors and EndActors (ramp phase)
	if phase.Duration <= 0 {
		return phase.EndActors
	}

	progress := float64(phaseElapsed) / float64(phase.Duration)
	if progress > 1 {
		progress = 1
	}
	if progress < 0 {
		progress = 0
	}

	diff := float64(phase.EndActors - phase.StartActors)
	return phase.StartActors + int(diff*progress)
}

// CurrentRPS returns the rate limit for the current phase (0 means no limit).
func (pm *PhaseManager) CurrentRPS() int {
	phase := pm.CurrentPhase()
	if phase == nil {
		return 0
	}
	return phase.RPS
}

// IsComplete returns true if all phases have finished.
func (pm *PhaseManager) IsComplete() bool {
	return pm.CurrentPhase() == nil
}

// PhaseName returns the name of the current phase, or empty string if complete.
func (pm *PhaseManager) PhaseName() string {
	phase := pm.CurrentPhase()
	if phase == nil {
		return ""
	}
	return phase.Name
}

// TimeRemaining returns the time remaining in the current phase.
func (pm *PhaseManager) TimeRemaining() time.Duration {
	elapsed := time.Since(pm.startTime)
	var accumulated time.Duration

	for i := range pm.phases {
		phaseEnd := accumulated + pm.phases[i].Duration
		if elapsed < phaseEnd {
			return phaseEnd - elapsed
		}
		accumulated = phaseEnd
	}
	return 0
}

// TotalRemaining returns the total time remaining across all phases.
func (pm *PhaseManager) TotalRemaining() time.Duration {
	var total time.Duration
	for _, p := range pm.phases {
		total += p.Duration
	}
	return total - time.Since(pm.startTime)
}
