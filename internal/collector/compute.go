// Package collector provides metrics computation for load test events.
package collector

import (
	"time"

	"maestro/internal/core"
)

// ComputeMetrics computes metrics from events. Pure function, no side effects.
func ComputeMetrics(events []core.Event, testDuration time.Duration) *Metrics {
	m := &Metrics{
		Steps:        make(map[string]*StepMetrics),
		TestDuration: testDuration,
	}

	if len(events) == 0 {
		return m
	}

	allDurations := make([]time.Duration, 0, len(events))
	stepDurations := make(map[string][]time.Duration)

	for _, e := range events {
		m.TotalRequests++
		if e.Success {
			m.SuccessCount++
		} else {
			m.FailureCount++
		}

		allDurations = append(allDurations, e.Duration)

		if _, exists := m.Steps[e.Step]; !exists {
			m.Steps[e.Step] = &StepMetrics{}
			stepDurations[e.Step] = make([]time.Duration, 0)
		}

		step := m.Steps[e.Step]
		step.Count++
		if e.Success {
			step.Success++
		} else {
			step.Failed++
		}
		stepDurations[e.Step] = append(stepDurations[e.Step], e.Duration)
	}

	if m.TotalRequests > 0 {
		m.SuccessRate = float64(m.SuccessCount) / float64(m.TotalRequests) * 100
	}

	if m.TestDuration > 0 {
		m.RequestsPerSec = float64(m.TotalRequests) / m.TestDuration.Seconds()
	}

	m.Duration = ComputeDurationMetrics(allDurations)

	for step, durations := range stepDurations {
		m.Steps[step].Duration = ComputeDurationMetrics(durations)
	}

	return m
}
