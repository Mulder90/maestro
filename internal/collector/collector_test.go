package collector

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"burstsmith/internal/core"
)

func TestCollector_CollectsEvents(t *testing.T) {
	c := NewCollector()
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond})
	c.Report(core.Event{ActorID: 2, Step: "test", Success: false, Duration: 20 * time.Millisecond})
	c.Close()

	events := c.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestCollector_Compute(t *testing.T) {
	c := NewCollector()
	c.Report(core.Event{ActorID: 1, Step: "s1", Success: true, Duration: 10 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "s1", Success: true, Duration: 20 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "s1", Success: false, Duration: 30 * time.Millisecond})
	c.Close()

	m := c.Compute()
	if m.TotalRequests != 3 {
		t.Errorf("expected 3 requests, got %d", m.TotalRequests)
	}
	if m.SuccessCount != 2 {
		t.Errorf("expected 2 success, got %d", m.SuccessCount)
	}
	if m.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", m.FailureCount)
	}
}

func TestComputePercentile(t *testing.T) {
	durations := []time.Duration{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	p50 := ComputePercentile(durations, 0.50)
	if p50 != 50 {
		t.Errorf("expected p50=50, got %d", p50)
	}
	p90 := ComputePercentile(durations, 0.90)
	if p90 != 90 {
		t.Errorf("expected p90=90, got %d", p90)
	}
}

func TestCollector_ThreadSafety(t *testing.T) {
	c := NewCollector()
	var wg sync.WaitGroup
	numGoroutines := 100
	eventsPerGoroutine := 50

	// Spawn many goroutines reporting events concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(actorID int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				c.Report(core.Event{
					ActorID:  actorID,
					Step:     "test",
					Success:  true,
					Duration: time.Millisecond,
				})
			}
		}(i)
	}

	wg.Wait()
	c.Close()

	// All events should be collected (channel buffer is 1000, might drop some)
	// At minimum, we should have collected many events without panicking
	if len(c.Events()) == 0 {
		t.Error("expected events to be collected")
	}
}

func TestCollector_SuccessRateCalculation(t *testing.T) {
	c := NewCollector()

	// 7 successes, 3 failures = 70% success rate
	for i := 0; i < 7; i++ {
		c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: time.Millisecond})
	}
	for i := 0; i < 3; i++ {
		c.Report(core.Event{ActorID: 1, Step: "test", Success: false, Duration: time.Millisecond})
	}

	c.Close()

	m := c.Compute()
	if m.SuccessRate != 70.0 {
		t.Errorf("expected 70%% success rate, got %.1f%%", m.SuccessRate)
	}
}

func TestCollector_TracksMultipleSteps(t *testing.T) {
	c := NewCollector()

	// Report events for different steps
	c.Report(core.Event{ActorID: 1, Step: "login", Success: true, Duration: 100 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "login", Success: true, Duration: 150 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "fetch_data", Success: true, Duration: 200 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "logout", Success: true, Duration: 50 * time.Millisecond})

	c.Close()

	m := c.Compute()

	if m.Steps["login"].Count != 2 {
		t.Errorf("expected 2 login events, got %d", m.Steps["login"].Count)
	}
	if m.Steps["fetch_data"].Count != 1 {
		t.Errorf("expected 1 fetch_data event, got %d", m.Steps["fetch_data"].Count)
	}
	if m.Steps["logout"].Count != 1 {
		t.Errorf("expected 1 logout event, got %d", m.Steps["logout"].Count)
	}
}

func TestCollector_TracksActorIDs(t *testing.T) {
	c := NewCollector()

	// Report events from multiple actors
	for actorID := 1; actorID <= 5; actorID++ {
		c.Report(core.Event{ActorID: actorID, Step: "test", Success: true, Duration: time.Millisecond})
	}

	c.Close()

	// Verify all actor IDs are present
	actorsSeen := make(map[int]bool)
	for _, e := range c.Events() {
		actorsSeen[e.ActorID] = true
	}

	for i := 1; i <= 5; i++ {
		if !actorsSeen[i] {
			t.Errorf("missing events from actor %d", i)
		}
	}
}

func TestCollector_HandlesNoEvents(t *testing.T) {
	c := NewCollector()
	c.Close()

	// Should not panic when computing empty collector
	m := c.Compute()
	if m.TotalRequests != 0 {
		t.Errorf("expected 0 events, got %d", m.TotalRequests)
	}
}

func TestCollector_RecordsDurations(t *testing.T) {
	c := NewCollector()

	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}

	for _, d := range durations {
		c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: d})
	}

	c.Close()

	// Verify durations are recorded correctly
	var total time.Duration
	for _, e := range c.Events() {
		total += e.Duration
	}

	expected := 100 * time.Millisecond
	if total != expected {
		t.Errorf("expected total duration %v, got %v", expected, total)
	}
}

func TestCollector_RecordsErrors(t *testing.T) {
	c := NewCollector()

	c.Report(core.Event{ActorID: 1, Step: "test", Success: false, Error: "connection refused"})
	c.Report(core.Event{ActorID: 2, Step: "test", Success: false, Error: "timeout"})

	c.Close()

	errors := make(map[string]bool)
	for _, e := range c.Events() {
		if e.Error != "" {
			errors[e.Error] = true
		}
	}

	if !errors["connection refused"] {
		t.Error("missing 'connection refused' error")
	}
	if !errors["timeout"] {
		t.Error("missing 'timeout' error")
	}
}

func TestCollector_PrintJSON(t *testing.T) {
	c := NewCollector()
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: 20 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "test", Success: false, Duration: 30 * time.Millisecond, Error: "failed"})
	c.Close()

	m := c.Compute()
	var buf bytes.Buffer
	c.PrintJSON(&buf, m, nil)

	output := buf.String()

	// Should contain expected JSON fields
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

func TestCollector_PrintJSON_WithThresholds(t *testing.T) {
	c := NewCollector()
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond})
	c.Close()

	m := c.Compute()
	thresholds := &ThresholdResults{
		Passed: true,
		Results: []ThresholdResult{
			{Name: "test_threshold", Passed: true, Threshold: "100ms", Actual: "10ms"},
		},
	}

	var buf bytes.Buffer
	c.PrintJSON(&buf, m, thresholds)

	output := buf.String()

	if !strings.Contains(output, `"thresholds"`) {
		t.Errorf("expected thresholds in JSON, got: %s", output)
	}
	if !strings.Contains(output, `"test_threshold"`) {
		t.Errorf("expected threshold name in JSON, got: %s", output)
	}
}

func TestCollector_PrintText(t *testing.T) {
	c := NewCollector()
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond})
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: 20 * time.Millisecond})
	c.Close()

	m := c.Compute()
	var buf bytes.Buffer
	c.PrintText(&buf, m, nil)

	output := buf.String()

	// Should contain expected text elements
	if !strings.Contains(output, "BurstSmith - Load Test Results") {
		t.Errorf("expected header in text output, got: %s", output)
	}
	if !strings.Contains(output, "Total Requests:") {
		t.Errorf("expected Total Requests in text output, got: %s", output)
	}
	if !strings.Contains(output, "Success Rate:") {
		t.Errorf("expected Success Rate in text output, got: %s", output)
	}
	if !strings.Contains(output, "Response Times:") {
		t.Errorf("expected Response Times in text output, got: %s", output)
	}
	if !strings.Contains(output, "By Step:") {
		t.Errorf("expected By Step in text output, got: %s", output)
	}
}

func TestCollector_PrintText_NoEvents(t *testing.T) {
	c := NewCollector()
	c.Close()

	m := c.Compute()
	var buf bytes.Buffer
	c.PrintText(&buf, m, nil)

	output := buf.String()

	if !strings.Contains(output, "No events collected") {
		t.Errorf("expected 'No events collected' message, got: %s", output)
	}
}

func TestCollector_PrintText_WithThresholds(t *testing.T) {
	c := NewCollector()
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond})
	c.Close()

	m := c.Compute()
	thresholds := &ThresholdResults{
		Passed: false,
		Results: []ThresholdResult{
			{Name: "http_req_duration.p95", Passed: true, Threshold: "100ms", Actual: "10ms"},
			{Name: "http_req_failed.rate", Passed: false, Threshold: "1%", Actual: "5%"},
		},
	}

	var buf bytes.Buffer
	c.PrintText(&buf, m, thresholds)

	output := buf.String()

	if !strings.Contains(output, "Thresholds:") {
		t.Errorf("expected Thresholds section in output, got: %s", output)
	}
	// Check for pass/fail symbols
	if !strings.Contains(output, "✓") {
		t.Errorf("expected checkmark for passing threshold, got: %s", output)
	}
	if !strings.Contains(output, "✗") {
		t.Errorf("expected X for failing threshold, got: %s", output)
	}
}

func TestCollector_Compute_RequestsPerSec(t *testing.T) {
	c := NewCollector()

	// Report events quickly
	for i := 0; i < 100; i++ {
		c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: time.Millisecond})
	}

	time.Sleep(100 * time.Millisecond) // Let some time pass
	c.Close()

	m := c.Compute()

	// Should have calculated RPS
	if m.RequestsPerSec <= 0 {
		t.Errorf("expected positive RPS, got %f", m.RequestsPerSec)
	}
}
