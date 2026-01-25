package collector

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Thresholds defines pass/fail criteria for the test.
type Thresholds struct {
	HTTPReqDuration *DurationThresholds `yaml:"http_req_duration"`
	HTTPReqFailed   *FailureThresholds  `yaml:"http_req_failed"`
}

// DurationThresholds defines latency limits.
type DurationThresholds struct {
	Avg time.Duration `yaml:"avg"`
	P50 time.Duration `yaml:"p50"`
	P90 time.Duration `yaml:"p90"`
	P95 time.Duration `yaml:"p95"`
	P99 time.Duration `yaml:"p99"`
}

// FailureThresholds defines error rate limits.
type FailureThresholds struct {
	Rate string `yaml:"rate"`
}

// ThresholdResult represents the outcome of a single threshold check.
type ThresholdResult struct {
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	Threshold string `json:"threshold"`
	Actual    string `json:"actual"`
}

// ThresholdResults contains all threshold check results.
type ThresholdResults struct {
	Passed  bool              `json:"passed"`
	Results []ThresholdResult `json:"results"`
}

// Check evaluates all thresholds against computed metrics.
func (t *Thresholds) Check(m *Metrics) *ThresholdResults {
	if t == nil {
		return &ThresholdResults{Passed: true, Results: nil}
	}

	results := &ThresholdResults{
		Passed:  true,
		Results: make([]ThresholdResult, 0),
	}

	if t.HTTPReqDuration != nil {
		results.checkDurationThresholds(t.HTTPReqDuration, &m.Duration)
	}

	if t.HTTPReqFailed != nil && t.HTTPReqFailed.Rate != "" {
		results.checkFailureRate(t.HTTPReqFailed, m)
	}

	return results
}

func (r *ThresholdResults) checkDurationThresholds(thresholds *DurationThresholds, actual *DurationMetrics) {
	checks := []struct {
		name      string
		threshold time.Duration
		actual    time.Duration
	}{
		{"http_req_duration.avg", thresholds.Avg, actual.Avg},
		{"http_req_duration.p50", thresholds.P50, actual.P50},
		{"http_req_duration.p90", thresholds.P90, actual.P90},
		{"http_req_duration.p95", thresholds.P95, actual.P95},
		{"http_req_duration.p99", thresholds.P99, actual.P99},
	}

	for _, check := range checks {
		if check.threshold == 0 {
			continue
		}

		passed := check.actual < check.threshold
		if !passed {
			r.Passed = false
		}

		r.Results = append(r.Results, ThresholdResult{
			Name:      check.name,
			Passed:    passed,
			Threshold: FormatDuration(check.threshold),
			Actual:    FormatDuration(check.actual),
		})
	}
}

func (r *ThresholdResults) checkFailureRate(thresholds *FailureThresholds, m *Metrics) {
	thresholdRate, err := parsePercentage(thresholds.Rate)
	if err != nil {
		return
	}

	actualRate := 100.0 - m.SuccessRate
	passed := actualRate < thresholdRate

	if !passed {
		r.Passed = false
	}

	r.Results = append(r.Results, ThresholdResult{
		Name:      "http_req_failed.rate",
		Passed:    passed,
		Threshold: thresholds.Rate,
		Actual:    fmt.Sprintf("%.2f%%", actualRate),
	})
}

func parsePercentage(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "%") {
		return 0, fmt.Errorf("invalid percentage format: %s", s)
	}
	s = strings.TrimSuffix(s, "%")
	return strconv.ParseFloat(s, 64)
}

// FormatDuration formats a duration for display.
func FormatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// Violations returns only the failed threshold results.
func (r *ThresholdResults) Violations() []ThresholdResult {
	violations := make([]ThresholdResult, 0)
	for _, result := range r.Results {
		if !result.Passed {
			violations = append(violations, result)
		}
	}
	return violations
}
