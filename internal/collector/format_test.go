package collector

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFormatText_BasicOutput(t *testing.T) {
	m := &Metrics{
		TotalRequests:  100,
		SuccessCount:   95,
		FailureCount:   5,
		SuccessRate:    95.0,
		RequestsPerSec: 10.0,
		TestDuration:   10 * time.Second,
		Duration: DurationMetrics{
			Min: 10 * time.Millisecond,
			Max: 100 * time.Millisecond,
			Avg: 50 * time.Millisecond,
			P50: 45 * time.Millisecond,
			P90: 80 * time.Millisecond,
			P95: 90 * time.Millisecond,
			P99: 98 * time.Millisecond,
		},
		Steps: map[string]*StepMetrics{
			"test": {
				Count:   100,
				Success: 95,
				Failed:  5,
				Duration: DurationMetrics{
					Min: 10 * time.Millisecond,
					Max: 100 * time.Millisecond,
					Avg: 50 * time.Millisecond,
					P50: 45 * time.Millisecond,
					P90: 80 * time.Millisecond,
					P95: 90 * time.Millisecond,
					P99: 98 * time.Millisecond,
				},
			},
		},
	}

	var buf bytes.Buffer
	FormatText(&buf, m, nil)

	output := buf.String()

	// Check header
	if !strings.Contains(output, "Maestro - Load Test Results") {
		t.Errorf("expected header in output, got: %s", output)
	}

	// Check summary stats
	if !strings.Contains(output, "Total Requests: 100") {
		t.Errorf("expected Total Requests in output, got: %s", output)
	}
	if !strings.Contains(output, "Success Rate:   95.0%") {
		t.Errorf("expected Success Rate in output, got: %s", output)
	}
	if !strings.Contains(output, "Requests/sec:   10.0") {
		t.Errorf("expected Requests/sec in output, got: %s", output)
	}

	// Check response times section
	if !strings.Contains(output, "Response Times:") {
		t.Errorf("expected Response Times section, got: %s", output)
	}

	// Check steps section
	if !strings.Contains(output, "By Step:") {
		t.Errorf("expected By Step section, got: %s", output)
	}
}

func TestFormatText_NoEvents(t *testing.T) {
	m := &Metrics{
		TotalRequests: 0,
		Steps:         make(map[string]*StepMetrics),
	}

	var buf bytes.Buffer
	FormatText(&buf, m, nil)

	output := buf.String()

	if !strings.Contains(output, "No events collected") {
		t.Errorf("expected 'No events collected' message, got: %s", output)
	}
}

func TestFormatText_WithThresholds(t *testing.T) {
	m := &Metrics{
		TotalRequests:  10,
		SuccessCount:   9,
		FailureCount:   1,
		SuccessRate:    90.0,
		RequestsPerSec: 1.0,
		TestDuration:   10 * time.Second,
		Duration:       DurationMetrics{},
		Steps:          make(map[string]*StepMetrics),
	}

	thresholds := &ThresholdResults{
		Passed: false,
		Results: []ThresholdResult{
			{Name: "http_req_duration.p95", Passed: true, Threshold: "100ms", Actual: "10ms"},
			{Name: "http_req_failed.rate", Passed: false, Threshold: "1%", Actual: "5%"},
		},
	}

	var buf bytes.Buffer
	FormatText(&buf, m, thresholds)

	output := buf.String()

	if !strings.Contains(output, "Thresholds:") {
		t.Errorf("expected Thresholds section in output, got: %s", output)
	}
	if !strings.Contains(output, "✓") {
		t.Errorf("expected checkmark for passing threshold, got: %s", output)
	}
	if !strings.Contains(output, "✗") {
		t.Errorf("expected X for failing threshold, got: %s", output)
	}
}

func TestFormatJSON_BasicOutput(t *testing.T) {
	m := &Metrics{
		TotalRequests:  3,
		SuccessCount:   2,
		FailureCount:   1,
		SuccessRate:    66.67,
		RequestsPerSec: 3.0,
		TestDuration:   1 * time.Second,
		Duration: DurationMetrics{
			Min: 10 * time.Millisecond,
			Max: 30 * time.Millisecond,
			Avg: 20 * time.Millisecond,
		},
		Steps: map[string]*StepMetrics{
			"test": {
				Count:   3,
				Success: 2,
				Failed:  1,
				Duration: DurationMetrics{
					Min: 10 * time.Millisecond,
					Max: 30 * time.Millisecond,
					Avg: 20 * time.Millisecond,
				},
			},
		},
	}

	var buf bytes.Buffer
	FormatJSON(&buf, m, nil)

	output := buf.String()

	// Check JSON fields
	if !strings.Contains(output, `"totalRequests": 3`) {
		t.Errorf("expected totalRequests in JSON, got: %s", output)
	}
	if !strings.Contains(output, `"successCount": 2`) {
		t.Errorf("expected successCount in JSON, got: %s", output)
	}
	if !strings.Contains(output, `"failureCount": 1`) {
		t.Errorf("expected failureCount in JSON, got: %s", output)
	}
	if !strings.Contains(output, `"steps"`) {
		t.Errorf("expected steps in JSON, got: %s", output)
	}
}

func TestFormatJSON_WithThresholds(t *testing.T) {
	m := &Metrics{
		TotalRequests:  1,
		SuccessCount:   1,
		SuccessRate:    100.0,
		RequestsPerSec: 1.0,
		TestDuration:   1 * time.Second,
		Duration:       DurationMetrics{},
		Steps:          make(map[string]*StepMetrics),
	}

	thresholds := &ThresholdResults{
		Passed: true,
		Results: []ThresholdResult{
			{Name: "test_threshold", Passed: true, Threshold: "100ms", Actual: "10ms"},
		},
	}

	var buf bytes.Buffer
	FormatJSON(&buf, m, thresholds)

	output := buf.String()

	if !strings.Contains(output, `"thresholds"`) {
		t.Errorf("expected thresholds in JSON, got: %s", output)
	}
	if !strings.Contains(output, `"test_threshold"`) {
		t.Errorf("expected threshold name in JSON, got: %s", output)
	}
}

func TestFormatJSON_NilThresholds(t *testing.T) {
	m := &Metrics{
		TotalRequests: 1,
		SuccessCount:  1,
		TestDuration:  1 * time.Second,
		Steps:         make(map[string]*StepMetrics),
	}

	var buf bytes.Buffer
	FormatJSON(&buf, m, nil)

	output := buf.String()

	// thresholds should be omitted when nil
	if strings.Contains(output, `"thresholds"`) {
		t.Errorf("expected thresholds to be omitted when nil, got: %s", output)
	}
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{9999, "9,999"},
	}

	for _, tt := range tests {
		result := formatNumber(tt.input)
		if result != tt.expected {
			t.Errorf("formatNumber(%d) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestFormatText_LargeNumbers(t *testing.T) {
	m := &Metrics{
		TotalRequests:  1500,
		SuccessCount:   1450,
		FailureCount:   50,
		SuccessRate:    96.67,
		RequestsPerSec: 150.0,
		TestDuration:   10 * time.Second,
		Duration:       DurationMetrics{},
		Steps:          make(map[string]*StepMetrics),
	}

	var buf bytes.Buffer
	FormatText(&buf, m, nil)

	output := buf.String()

	// Check that large numbers are formatted with commas
	if !strings.Contains(output, "1,500") {
		t.Errorf("expected formatted number 1,500 in output, got: %s", output)
	}
}
