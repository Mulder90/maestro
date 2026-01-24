package burstsmith

import (
	"testing"
	"time"
)

func TestThresholds_NilThresholds(t *testing.T) {
	var thresholds *Thresholds = nil
	metrics := &Metrics{
		TotalRequests: 100,
		SuccessCount:  95,
		SuccessRate:   95.0,
	}

	results := thresholds.Check(metrics)

	if !results.Passed {
		t.Error("nil thresholds should always pass")
	}
	if len(results.Results) != 0 {
		t.Errorf("expected no results for nil thresholds, got %d", len(results.Results))
	}
}

func TestThresholds_DurationThresholds_AllPass(t *testing.T) {
	thresholds := &Thresholds{
		HTTPReqDuration: &DurationThresholds{
			P95: 500 * time.Millisecond,
			P99: 1 * time.Second,
		},
	}

	metrics := &Metrics{
		Duration: DurationMetrics{
			P95: 300 * time.Millisecond,
			P99: 800 * time.Millisecond,
		},
	}

	results := thresholds.Check(metrics)

	if !results.Passed {
		t.Error("expected all thresholds to pass")
	}

	passedCount := 0
	for _, r := range results.Results {
		if r.Passed {
			passedCount++
		}
	}
	if passedCount != 2 {
		t.Errorf("expected 2 passed results, got %d", passedCount)
	}
}

func TestThresholds_DurationThresholds_SomeFail(t *testing.T) {
	thresholds := &Thresholds{
		HTTPReqDuration: &DurationThresholds{
			P95: 200 * time.Millisecond, // actual is 300ms - FAIL
			P99: 1 * time.Second,        // actual is 800ms - PASS
		},
	}

	metrics := &Metrics{
		Duration: DurationMetrics{
			P95: 300 * time.Millisecond,
			P99: 800 * time.Millisecond,
		},
	}

	results := thresholds.Check(metrics)

	if results.Passed {
		t.Error("expected thresholds to fail")
	}

	violations := results.Violations()
	if len(violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Name != "http_req_duration.p95" {
		t.Errorf("expected p95 violation, got %s", violations[0].Name)
	}
}

func TestThresholds_FailureRate_Pass(t *testing.T) {
	thresholds := &Thresholds{
		HTTPReqFailed: &FailureThresholds{
			Rate: "5%",
		},
	}

	metrics := &Metrics{
		TotalRequests: 100,
		SuccessCount:  97,
		SuccessRate:   97.0, // 3% failure rate
	}

	results := thresholds.Check(metrics)

	if !results.Passed {
		t.Error("expected threshold to pass with 3% failure rate < 5%")
	}
}

func TestThresholds_FailureRate_Fail(t *testing.T) {
	thresholds := &Thresholds{
		HTTPReqFailed: &FailureThresholds{
			Rate: "1%",
		},
	}

	metrics := &Metrics{
		TotalRequests: 100,
		SuccessCount:  95,
		SuccessRate:   95.0, // 5% failure rate
	}

	results := thresholds.Check(metrics)

	if results.Passed {
		t.Error("expected threshold to fail with 5% failure rate > 1%")
	}

	violations := results.Violations()
	if len(violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Name != "http_req_failed.rate" {
		t.Errorf("expected failure rate violation, got %s", violations[0].Name)
	}
}

func TestThresholds_CombinedThresholds(t *testing.T) {
	thresholds := &Thresholds{
		HTTPReqDuration: &DurationThresholds{
			P95: 500 * time.Millisecond,
			P99: 1 * time.Second,
		},
		HTTPReqFailed: &FailureThresholds{
			Rate: "2%", // threshold is < 2%, actual is 1% = PASS
		},
	}

	metrics := &Metrics{
		TotalRequests: 100,
		SuccessCount:  99,
		SuccessRate:   99.0, // 1% failure rate
		Duration: DurationMetrics{
			P95: 300 * time.Millisecond,
			P99: 700 * time.Millisecond,
		},
	}

	results := thresholds.Check(metrics)

	if !results.Passed {
		t.Error("expected all thresholds to pass")
	}
	if len(results.Results) != 3 {
		t.Errorf("expected 3 results (p95, p99, rate), got %d", len(results.Results))
	}
}

func TestThresholds_ZeroThresholdNotChecked(t *testing.T) {
	thresholds := &Thresholds{
		HTTPReqDuration: &DurationThresholds{
			P95: 0, // zero means not set
			P99: 500 * time.Millisecond,
		},
	}

	metrics := &Metrics{
		Duration: DurationMetrics{
			P95: 999 * time.Second, // would fail if checked
			P99: 300 * time.Millisecond,
		},
	}

	results := thresholds.Check(metrics)

	if !results.Passed {
		t.Error("expected to pass when zero threshold is not checked")
	}
	if len(results.Results) != 1 {
		t.Errorf("expected 1 result (only p99), got %d", len(results.Results))
	}
}

func TestParsePercentage_ValidFormats(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"1%", 1.0},
		{"0.5%", 0.5},
		{"99.9%", 99.9},
		{"0%", 0.0},
		{"  5%  ", 5.0}, // with whitespace
	}

	for _, tt := range tests {
		result, err := parsePercentage(tt.input)
		if err != nil {
			t.Errorf("unexpected error for %q: %v", tt.input, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("parsePercentage(%q): expected %v, got %v", tt.input, tt.expected, result)
		}
	}
}

func TestParsePercentage_InvalidFormats(t *testing.T) {
	invalidInputs := []string{
		"1",      // no percent sign
		"1%%",    // double percent
		"abc%",   // not a number
		"",       // empty
	}

	for _, input := range invalidInputs {
		_, err := parsePercentage(input)
		if err == nil {
			t.Errorf("expected error for invalid input %q", input)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{500 * time.Microsecond, "500µs"},
		{100 * time.Millisecond, "100ms"},
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1m30s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.input)
		if result != tt.expected {
			t.Errorf("formatDuration(%v): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestThresholdResults_Violations(t *testing.T) {
	results := &ThresholdResults{
		Passed: false,
		Results: []ThresholdResult{
			{Name: "test1", Passed: true},
			{Name: "test2", Passed: false},
			{Name: "test3", Passed: true},
			{Name: "test4", Passed: false},
		},
	}

	violations := results.Violations()

	if len(violations) != 2 {
		t.Errorf("expected 2 violations, got %d", len(violations))
	}

	expectedNames := map[string]bool{"test2": true, "test4": true}
	for _, v := range violations {
		if !expectedNames[v.Name] {
			t.Errorf("unexpected violation: %s", v.Name)
		}
	}
}
