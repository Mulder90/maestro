package collector

import (
	"testing"
	"time"

	"maestro/internal/core"
)

func TestComputeMetrics_EmptyEvents(t *testing.T) {
	m := ComputeMetrics(nil, 10*time.Second)

	if m.TotalRequests != 0 {
		t.Errorf("expected 0 total requests, got %d", m.TotalRequests)
	}
	if m.TestDuration != 10*time.Second {
		t.Errorf("expected 10s duration, got %v", m.TestDuration)
	}
	if m.Steps == nil {
		t.Error("expected Steps map to be initialized")
	}
}

func TestComputeMetrics_BasicCounts(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "s1", Success: true, Duration: 10 * time.Millisecond},
		{ActorID: 1, Step: "s1", Success: true, Duration: 20 * time.Millisecond},
		{ActorID: 1, Step: "s1", Success: false, Duration: 30 * time.Millisecond},
	}

	m := ComputeMetrics(events, 1*time.Second)

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

func TestComputeMetrics_SuccessRate(t *testing.T) {
	events := make([]core.Event, 0)

	// 7 successes, 3 failures = 70% success rate
	for i := 0; i < 7; i++ {
		events = append(events, core.Event{ActorID: 1, Step: "test", Success: true, Duration: time.Millisecond})
	}
	for i := 0; i < 3; i++ {
		events = append(events, core.Event{ActorID: 1, Step: "test", Success: false, Duration: time.Millisecond})
	}

	m := ComputeMetrics(events, 1*time.Second)

	if m.SuccessRate != 70.0 {
		t.Errorf("expected 70%% success rate, got %.1f%%", m.SuccessRate)
	}
}

func TestComputeMetrics_RequestsPerSec(t *testing.T) {
	events := make([]core.Event, 100)
	for i := range events {
		events[i] = core.Event{ActorID: 1, Step: "test", Success: true, Duration: time.Millisecond}
	}

	m := ComputeMetrics(events, 10*time.Second)

	expected := 10.0 // 100 requests / 10 seconds
	if m.RequestsPerSec != expected {
		t.Errorf("expected %.1f RPS, got %.1f", expected, m.RequestsPerSec)
	}
}

func TestComputeMetrics_MultipleSteps(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "login", Success: true, Duration: 100 * time.Millisecond},
		{ActorID: 1, Step: "login", Success: true, Duration: 150 * time.Millisecond},
		{ActorID: 1, Step: "fetch_data", Success: true, Duration: 200 * time.Millisecond},
		{ActorID: 1, Step: "logout", Success: true, Duration: 50 * time.Millisecond},
	}

	m := ComputeMetrics(events, 1*time.Second)

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

func TestComputeMetrics_StepSuccessFailure(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "api_call", Success: true, Duration: 10 * time.Millisecond},
		{ActorID: 2, Step: "api_call", Success: true, Duration: 20 * time.Millisecond},
		{ActorID: 3, Step: "api_call", Success: false, Duration: 30 * time.Millisecond},
	}

	m := ComputeMetrics(events, 1*time.Second)

	step := m.Steps["api_call"]
	if step.Count != 3 {
		t.Errorf("expected 3 api_call events, got %d", step.Count)
	}
	if step.Success != 2 {
		t.Errorf("expected 2 successes, got %d", step.Success)
	}
	if step.Failed != 1 {
		t.Errorf("expected 1 failure, got %d", step.Failed)
	}
}

func TestComputeMetrics_DurationMetrics(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond},
		{ActorID: 1, Step: "test", Success: true, Duration: 20 * time.Millisecond},
		{ActorID: 1, Step: "test", Success: true, Duration: 30 * time.Millisecond},
		{ActorID: 1, Step: "test", Success: true, Duration: 40 * time.Millisecond},
	}

	m := ComputeMetrics(events, 1*time.Second)

	if m.Duration.Min != 10*time.Millisecond {
		t.Errorf("expected min 10ms, got %v", m.Duration.Min)
	}
	if m.Duration.Max != 40*time.Millisecond {
		t.Errorf("expected max 40ms, got %v", m.Duration.Max)
	}
	// Avg = (10+20+30+40) / 4 = 25ms
	if m.Duration.Avg != 25*time.Millisecond {
		t.Errorf("expected avg 25ms, got %v", m.Duration.Avg)
	}
}

func TestComputeMetrics_StepDurationMetrics(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "fast", Success: true, Duration: 5 * time.Millisecond},
		{ActorID: 1, Step: "fast", Success: true, Duration: 10 * time.Millisecond},
		{ActorID: 1, Step: "slow", Success: true, Duration: 100 * time.Millisecond},
		{ActorID: 1, Step: "slow", Success: true, Duration: 200 * time.Millisecond},
	}

	m := ComputeMetrics(events, 1*time.Second)

	fastStep := m.Steps["fast"]
	if fastStep.Duration.Min != 5*time.Millisecond {
		t.Errorf("expected fast min 5ms, got %v", fastStep.Duration.Min)
	}
	if fastStep.Duration.Max != 10*time.Millisecond {
		t.Errorf("expected fast max 10ms, got %v", fastStep.Duration.Max)
	}

	slowStep := m.Steps["slow"]
	if slowStep.Duration.Min != 100*time.Millisecond {
		t.Errorf("expected slow min 100ms, got %v", slowStep.Duration.Min)
	}
	if slowStep.Duration.Max != 200*time.Millisecond {
		t.Errorf("expected slow max 200ms, got %v", slowStep.Duration.Max)
	}
}

func TestComputeMetrics_ZeroDuration(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "test", Success: true, Duration: time.Millisecond},
	}

	m := ComputeMetrics(events, 0)

	// Should not panic with zero duration
	if m.RequestsPerSec != 0 {
		t.Errorf("expected 0 RPS with zero duration, got %f", m.RequestsPerSec)
	}
}

func TestComputeMetrics_PureFunction(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond},
		{ActorID: 2, Step: "test", Success: false, Duration: 20 * time.Millisecond},
	}

	// Call multiple times with same input
	m1 := ComputeMetrics(events, 1*time.Second)
	m2 := ComputeMetrics(events, 1*time.Second)

	// Results should be identical
	if m1.TotalRequests != m2.TotalRequests {
		t.Error("pure function should return same result for same input")
	}
	if m1.SuccessCount != m2.SuccessCount {
		t.Error("pure function should return same result for same input")
	}
	if m1.FailureCount != m2.FailureCount {
		t.Error("pure function should return same result for same input")
	}
	if m1.SuccessRate != m2.SuccessRate {
		t.Error("pure function should return same result for same input")
	}
}

func TestComputeMetrics_DoesNotModifyInput(t *testing.T) {
	events := []core.Event{
		{ActorID: 1, Step: "test", Success: true, Duration: 10 * time.Millisecond},
	}

	originalLen := len(events)
	originalEvent := events[0]

	_ = ComputeMetrics(events, 1*time.Second)

	if len(events) != originalLen {
		t.Error("ComputeMetrics should not modify input slice length")
	}
	if events[0] != originalEvent {
		t.Error("ComputeMetrics should not modify input slice elements")
	}
}

func BenchmarkComputeMetrics(b *testing.B) {
	events := make([]core.Event, 10000)
	for i := range events {
		events[i] = core.Event{
			Duration: time.Duration(i) * time.Microsecond,
			Success:  true,
			Step:     "test",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeMetrics(events, time.Second)
	}
}

func BenchmarkCollectorReport(b *testing.B) {
	c := NewCollector()
	event := core.Event{ActorID: 1, Step: "test", Success: true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Report(event)
	}
	b.StopTimer()
	c.Close()
}
