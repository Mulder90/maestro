package burstsmith

import (
	"sort"
	"time"
)

// Metrics contains aggregated test results.
type Metrics struct {
	TotalRequests  int              `json:"totalRequests"`
	SuccessCount   int              `json:"successCount"`
	FailureCount   int              `json:"failureCount"`
	SuccessRate    float64          `json:"successRate"`
	RequestsPerSec float64          `json:"requestsPerSec"`
	TestDuration   time.Duration    `json:"testDuration"`
	Duration       DurationMetrics  `json:"durations"`
	Steps          map[string]*StepMetrics `json:"steps"`
}

// DurationMetrics contains latency statistics.
type DurationMetrics struct {
	Min time.Duration `json:"min"`
	Max time.Duration `json:"max"`
	Avg time.Duration `json:"avg"`
	P50 time.Duration `json:"p50"`
	P90 time.Duration `json:"p90"`
	P95 time.Duration `json:"p95"`
	P99 time.Duration `json:"p99"`
}

// StepMetrics contains per-step statistics.
type StepMetrics struct {
	Count    int             `json:"count"`
	Success  int             `json:"success"`
	Failed   int             `json:"failed"`
	Duration DurationMetrics `json:"durations"`
}

// ComputePercentile calculates the percentile value from a sorted slice of durations.
// The percentile p should be between 0 and 1 (e.g., 0.95 for p95).
// The slice must be sorted in ascending order.
func ComputePercentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}

	// Use the "nearest rank" method
	index := int(float64(len(sorted)-1) * p)
	return sorted[index]
}

// ComputeDurationMetrics calculates all duration statistics from a slice of durations.
func ComputeDurationMetrics(durations []time.Duration) DurationMetrics {
	if len(durations) == 0 {
		return DurationMetrics{}
	}

	// Sort durations for percentile calculation
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate total for average
	var total time.Duration
	for _, d := range sorted {
		total += d
	}

	return DurationMetrics{
		Min: sorted[0],
		Max: sorted[len(sorted)-1],
		Avg: total / time.Duration(len(sorted)),
		P50: ComputePercentile(sorted, 0.50),
		P90: ComputePercentile(sorted, 0.90),
		P95: ComputePercentile(sorted, 0.95),
		P99: ComputePercentile(sorted, 0.99),
	}
}
