package ratelimit

import (
	"time"

	"burstsmith/internal/config"
)

type PhaseManager struct {
	phases    []config.Phase
	startTime time.Time
}

func NewPhaseManager(phases []config.Phase) *PhaseManager {
	return &PhaseManager{
		phases:    phases,
		startTime: time.Now(),
	}
}

func (pm *PhaseManager) Elapsed() time.Duration {
	return time.Since(pm.startTime)
}

func (pm *PhaseManager) CurrentPhaseIndex() int {
	elapsed := pm.Elapsed()
	var cumulative time.Duration
	for i, p := range pm.phases {
		cumulative += p.Duration
		if elapsed < cumulative {
			return i
		}
	}
	return len(pm.phases)
}

func (pm *PhaseManager) CurrentPhase() *config.Phase {
	idx := pm.CurrentPhaseIndex()
	if idx >= len(pm.phases) {
		return nil
	}
	return &pm.phases[idx]
}

func (pm *PhaseManager) IsComplete() bool {
	return pm.CurrentPhaseIndex() >= len(pm.phases)
}

func (pm *PhaseManager) TargetActors() int {
	phase := pm.CurrentPhase()
	if phase == nil {
		return 0
	}
	if phase.Actors > 0 {
		return phase.Actors
	}
	if phase.StartActors == phase.EndActors {
		return phase.StartActors
	}
	elapsed := pm.Elapsed()
	var phaseStart time.Duration
	for i := 0; i < pm.CurrentPhaseIndex(); i++ {
		phaseStart += pm.phases[i].Duration
	}
	phaseElapsed := elapsed - phaseStart
	progress := float64(phaseElapsed) / float64(phase.Duration)
	if progress > 1 {
		progress = 1
	}
	delta := float64(phase.EndActors - phase.StartActors)
	return phase.StartActors + int(delta*progress)
}

func (pm *PhaseManager) CurrentRPS() int {
	phase := pm.CurrentPhase()
	if phase == nil {
		return 0
	}
	return phase.RPS
}
