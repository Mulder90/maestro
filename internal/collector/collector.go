// Package collector aggregates events and computes metrics.
package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"burstsmith/internal/core"
)

// Collector aggregates events from actors and produces a summary.
type Collector struct {
	events    []core.Event
	ch        chan core.Event
	done      chan struct{}
	mu        sync.Mutex
	startTime time.Time
	endTime   time.Time
}

// NewCollector creates a new Collector and starts its collection goroutine.
func NewCollector() *Collector {
	c := &Collector{
		events:    make([]core.Event, 0),
		ch:        make(chan core.Event, 1000),
		done:      make(chan struct{}),
		startTime: time.Now(),
	}
	go c.collect()
	return c
}

func (c *Collector) collect() {
	for event := range c.ch {
		c.mu.Lock()
		c.events = append(c.events, event)
		c.mu.Unlock()
	}
	close(c.done)
}

// Report sends an event to the collector. Thread-safe.
func (c *Collector) Report(event core.Event) {
	select {
	case c.ch <- event:
	default:
	}
}

// Close signals the collector to stop accepting events.
func (c *Collector) Close() {
	c.endTime = time.Now()
	close(c.ch)
	<-c.done
}

// Events returns a copy of collected events (for testing).
func (c *Collector) Events() []core.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]core.Event, len(c.events))
	copy(result, c.events)
	return result
}

// Compute calculates metrics from all collected events.
func (c *Collector) Compute() *Metrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	m := &Metrics{
		Steps: make(map[string]*StepMetrics),
	}

	if len(c.events) == 0 {
		return m
	}

	if !c.endTime.IsZero() {
		m.TestDuration = c.endTime.Sub(c.startTime)
	} else {
		m.TestDuration = time.Since(c.startTime)
	}

	allDurations := make([]time.Duration, 0, len(c.events))
	stepDurations := make(map[string][]time.Duration)

	for _, e := range c.events {
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

// Summary computes and prints aggregated statistics.
func (c *Collector) Summary() {
	m := c.Compute()
	c.PrintText(os.Stdout, m, nil)
}

// PrintText outputs metrics in human-readable format.
func (c *Collector) PrintText(w io.Writer, m *Metrics, thresholds *ThresholdResults) {
	if m.TotalRequests == 0 {
		fmt.Fprintln(w, "No events collected")
		return
	}

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "BurstSmith - Load Test Results")
	fmt.Fprintln(w, "==============================")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "Duration:       %v\n", m.TestDuration.Round(time.Millisecond))
	fmt.Fprintf(w, "Total Requests: %s\n", formatNumber(m.TotalRequests))
	fmt.Fprintf(w, "Success Rate:   %.1f%% (%s / %s)\n",
		m.SuccessRate, formatNumber(m.SuccessCount), formatNumber(m.TotalRequests))
	fmt.Fprintf(w, "Requests/sec:   %.1f\n", m.RequestsPerSec)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Response Times:")
	fmt.Fprintf(w, "  Min:    %s\n", FormatDuration(m.Duration.Min))
	fmt.Fprintf(w, "  Avg:    %s\n", FormatDuration(m.Duration.Avg))
	fmt.Fprintf(w, "  P50:    %s\n", FormatDuration(m.Duration.P50))
	fmt.Fprintf(w, "  P90:    %s\n", FormatDuration(m.Duration.P90))
	fmt.Fprintf(w, "  P95:    %s\n", FormatDuration(m.Duration.P95))
	fmt.Fprintf(w, "  P99:    %s\n", FormatDuration(m.Duration.P99))
	fmt.Fprintf(w, "  Max:    %s\n", FormatDuration(m.Duration.Max))
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "By Step:")
	for step, sm := range m.Steps {
		fmt.Fprintf(w, "  %-15s %s reqs   avg=%s  p95=%s  p99=%s\n",
			step, formatNumber(sm.Count),
			FormatDuration(sm.Duration.Avg),
			FormatDuration(sm.Duration.P95),
			FormatDuration(sm.Duration.P99))
	}

	if thresholds != nil && len(thresholds.Results) > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Thresholds:")
		for _, result := range thresholds.Results {
			symbol := "✓"
			if !result.Passed {
				symbol = "✗"
			}
			fmt.Fprintf(w, "  %s %s < %s (actual: %s)\n",
				symbol, result.Name, result.Threshold, result.Actual)
		}
	}
}

// PrintJSON outputs metrics in JSON format.
func (c *Collector) PrintJSON(w io.Writer, m *Metrics, thresholds *ThresholdResults) {
	output := struct {
		Duration       string                     `json:"duration"`
		TotalRequests  int                        `json:"totalRequests"`
		SuccessCount   int                        `json:"successCount"`
		FailureCount   int                        `json:"failureCount"`
		SuccessRate    float64                    `json:"successRate"`
		RequestsPerSec float64                    `json:"requestsPerSec"`
		Durations      jsonDurationMetrics        `json:"durations"`
		Steps          map[string]jsonStepMetrics `json:"steps"`
		Thresholds     *ThresholdResults          `json:"thresholds,omitempty"`
	}{
		Duration:       m.TestDuration.Round(time.Millisecond).String(),
		TotalRequests:  m.TotalRequests,
		SuccessCount:   m.SuccessCount,
		FailureCount:   m.FailureCount,
		SuccessRate:    m.SuccessRate,
		RequestsPerSec: m.RequestsPerSec,
		Durations:      toJSONDurationMetrics(m.Duration),
		Steps:          make(map[string]jsonStepMetrics),
		Thresholds:     thresholds,
	}

	for step, sm := range m.Steps {
		output.Steps[step] = jsonStepMetrics{
			Count:       sm.Count,
			Success:     sm.Success,
			Failed:      sm.Failed,
			SuccessRate: float64(sm.Success) / float64(sm.Count) * 100,
			Durations:   toJSONDurationMetrics(sm.Duration),
		}
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(output)
}

type jsonDurationMetrics struct {
	Min string `json:"min"`
	Max string `json:"max"`
	Avg string `json:"avg"`
	P50 string `json:"p50"`
	P90 string `json:"p90"`
	P95 string `json:"p95"`
	P99 string `json:"p99"`
}

type jsonStepMetrics struct {
	Count       int                 `json:"count"`
	Success     int                 `json:"success"`
	Failed      int                 `json:"failed"`
	SuccessRate float64             `json:"successRate"`
	Durations   jsonDurationMetrics `json:"durations"`
}

func toJSONDurationMetrics(d DurationMetrics) jsonDurationMetrics {
	return jsonDurationMetrics{
		Min: FormatDuration(d.Min),
		Max: FormatDuration(d.Max),
		Avg: FormatDuration(d.Avg),
		P50: FormatDuration(d.P50),
		P90: FormatDuration(d.P90),
		P95: FormatDuration(d.P95),
		P99: FormatDuration(d.P99),
	}
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}
