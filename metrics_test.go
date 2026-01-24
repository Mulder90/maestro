package burstsmith

import (
	"testing"
	"time"
)

func TestComputePercentile_EmptySlice(t *testing.T) {
	result := ComputePercentile([]time.Duration{}, 0.50)
	if result != 0 {
		t.Errorf("expected 0 for empty slice, got %v", result)
	}
}

func TestComputePercentile_SingleValue(t *testing.T) {
	durations := []time.Duration{100 * time.Millisecond}

	tests := []struct {
		percentile float64
		expected   time.Duration
	}{
		{0.0, 100 * time.Millisecond},
		{0.50, 100 * time.Millisecond},
		{0.99, 100 * time.Millisecond},
		{1.0, 100 * time.Millisecond},
	}

	for _, tt := range tests {
		result := ComputePercentile(durations, tt.percentile)
		if result != tt.expected {
			t.Errorf("p%.0f: expected %v, got %v", tt.percentile*100, tt.expected, result)
		}
	}
}

func TestComputePercentile_MultipleValues(t *testing.T) {
	// 10 sorted values: 10, 20, 30, 40, 50, 60, 70, 80, 90, 100 ms
	durations := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		durations[i] = time.Duration((i+1)*10) * time.Millisecond
	}

	tests := []struct {
		percentile float64
		expected   time.Duration
	}{
		{0.0, 10 * time.Millisecond},   // min
		{0.50, 50 * time.Millisecond},  // p50
		{0.90, 90 * time.Millisecond},  // p90
		{1.0, 100 * time.Millisecond},  // max
	}

	for _, tt := range tests {
		result := ComputePercentile(durations, tt.percentile)
		if result != tt.expected {
			t.Errorf("p%.0f: expected %v, got %v", tt.percentile*100, tt.expected, result)
		}
	}
}

func TestComputePercentile_BoundaryValues(t *testing.T) {
	durations := []time.Duration{10 * time.Millisecond, 100 * time.Millisecond}

	// Negative percentile should return first element
	result := ComputePercentile(durations, -0.5)
	if result != 10*time.Millisecond {
		t.Errorf("negative percentile: expected 10ms, got %v", result)
	}

	// Percentile > 1 should return last element
	result = ComputePercentile(durations, 1.5)
	if result != 100*time.Millisecond {
		t.Errorf("percentile > 1: expected 100ms, got %v", result)
	}
}

func TestComputeDurationMetrics_Empty(t *testing.T) {
	result := ComputeDurationMetrics([]time.Duration{})

	if result.Min != 0 || result.Max != 0 || result.Avg != 0 {
		t.Errorf("expected all zeros for empty input, got %+v", result)
	}
}

func TestComputeDurationMetrics_SingleValue(t *testing.T) {
	durations := []time.Duration{100 * time.Millisecond}
	result := ComputeDurationMetrics(durations)

	if result.Min != 100*time.Millisecond {
		t.Errorf("Min: expected 100ms, got %v", result.Min)
	}
	if result.Max != 100*time.Millisecond {
		t.Errorf("Max: expected 100ms, got %v", result.Max)
	}
	if result.Avg != 100*time.Millisecond {
		t.Errorf("Avg: expected 100ms, got %v", result.Avg)
	}
	if result.P50 != 100*time.Millisecond {
		t.Errorf("P50: expected 100ms, got %v", result.P50)
	}
}

func TestComputeDurationMetrics_MultipleValues(t *testing.T) {
	// Unsorted durations to verify sorting
	durations := []time.Duration{
		300 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		500 * time.Millisecond,
		400 * time.Millisecond,
	}
	result := ComputeDurationMetrics(durations)

	if result.Min != 100*time.Millisecond {
		t.Errorf("Min: expected 100ms, got %v", result.Min)
	}
	if result.Max != 500*time.Millisecond {
		t.Errorf("Max: expected 500ms, got %v", result.Max)
	}
	expectedAvg := 300 * time.Millisecond // (100+200+300+400+500)/5
	if result.Avg != expectedAvg {
		t.Errorf("Avg: expected %v, got %v", expectedAvg, result.Avg)
	}
}

func TestComputeDurationMetrics_PreservesOriginalSlice(t *testing.T) {
	original := []time.Duration{
		300 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
	}

	// Make a copy to compare
	expected := make([]time.Duration, len(original))
	copy(expected, original)

	ComputeDurationMetrics(original)

	// Original slice should be unchanged
	for i := range original {
		if original[i] != expected[i] {
			t.Errorf("original slice was modified: index %d, expected %v, got %v", i, expected[i], original[i])
		}
	}
}

func TestComputeDurationMetrics_LargeDataset(t *testing.T) {
	// Create 1000 values from 1ms to 1000ms
	durations := make([]time.Duration, 1000)
	for i := 0; i < 1000; i++ {
		durations[i] = time.Duration(i+1) * time.Millisecond
	}

	result := ComputeDurationMetrics(durations)

	if result.Min != 1*time.Millisecond {
		t.Errorf("Min: expected 1ms, got %v", result.Min)
	}
	if result.Max != 1000*time.Millisecond {
		t.Errorf("Max: expected 1000ms, got %v", result.Max)
	}
	// P99 should be around 990ms
	if result.P99 < 980*time.Millisecond || result.P99 > 1000*time.Millisecond {
		t.Errorf("P99: expected ~990ms, got %v", result.P99)
	}
}
