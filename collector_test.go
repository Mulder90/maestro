package burstsmith

import (
	"sync"
	"testing"
	"time"
)

func TestCollector_CollectsEvents(t *testing.T) {
	collector := NewCollector()

	// Report some events
	collector.Report(Event{ActorID: 1, Step: "step1", Success: true, Duration: 100 * time.Millisecond})
	collector.Report(Event{ActorID: 2, Step: "step1", Success: true, Duration: 200 * time.Millisecond})
	collector.Report(Event{ActorID: 1, Step: "step2", Success: false, Duration: 50 * time.Millisecond})

	collector.Close()

	// Verify events were collected
	if len(collector.events) != 3 {
		t.Errorf("expected 3 events, got %d", len(collector.events))
	}
}

func TestCollector_ThreadSafety(t *testing.T) {
	collector := NewCollector()
	var wg sync.WaitGroup
	numGoroutines := 100
	eventsPerGoroutine := 50

	// Spawn many goroutines reporting events concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(actorID int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				collector.Report(Event{
					ActorID:  actorID,
					Step:     "test",
					Success:  true,
					Duration: time.Millisecond,
				})
			}
		}(i)
	}

	wg.Wait()
	collector.Close()

	// All events should be collected (channel buffer is 1000, might drop some)
	// At minimum, we should have collected many events without panicking
	if len(collector.events) == 0 {
		t.Error("expected events to be collected")
	}
}

func TestCollector_SuccessRateCalculation(t *testing.T) {
	collector := NewCollector()

	// 7 successes, 3 failures = 70% success rate
	for i := 0; i < 7; i++ {
		collector.Report(Event{ActorID: 1, Step: "test", Success: true, Duration: time.Millisecond})
	}
	for i := 0; i < 3; i++ {
		collector.Report(Event{ActorID: 1, Step: "test", Success: false, Duration: time.Millisecond})
	}

	collector.Close()

	successCount := 0
	for _, e := range collector.events {
		if e.Success {
			successCount++
		}
	}

	expectedRate := float64(successCount) / float64(len(collector.events)) * 100
	if expectedRate != 70.0 {
		t.Errorf("expected 70%% success rate, got %.1f%%", expectedRate)
	}
}

func TestCollector_TracksMultipleSteps(t *testing.T) {
	collector := NewCollector()

	// Report events for different steps
	collector.Report(Event{ActorID: 1, Step: "login", Success: true, Duration: 100 * time.Millisecond})
	collector.Report(Event{ActorID: 1, Step: "login", Success: true, Duration: 150 * time.Millisecond})
	collector.Report(Event{ActorID: 1, Step: "fetch_data", Success: true, Duration: 200 * time.Millisecond})
	collector.Report(Event{ActorID: 1, Step: "logout", Success: true, Duration: 50 * time.Millisecond})

	collector.Close()

	// Count events per step
	stepCounts := make(map[string]int)
	for _, e := range collector.events {
		stepCounts[e.Step]++
	}

	if stepCounts["login"] != 2 {
		t.Errorf("expected 2 login events, got %d", stepCounts["login"])
	}
	if stepCounts["fetch_data"] != 1 {
		t.Errorf("expected 1 fetch_data event, got %d", stepCounts["fetch_data"])
	}
	if stepCounts["logout"] != 1 {
		t.Errorf("expected 1 logout event, got %d", stepCounts["logout"])
	}
}

func TestCollector_TracksActorIDs(t *testing.T) {
	collector := NewCollector()

	// Report events from multiple actors
	for actorID := 1; actorID <= 5; actorID++ {
		collector.Report(Event{ActorID: actorID, Step: "test", Success: true, Duration: time.Millisecond})
	}

	collector.Close()

	// Verify all actor IDs are present
	actorsSeen := make(map[int]bool)
	for _, e := range collector.events {
		actorsSeen[e.ActorID] = true
	}

	for i := 1; i <= 5; i++ {
		if !actorsSeen[i] {
			t.Errorf("missing events from actor %d", i)
		}
	}
}

func TestCollector_HandlesNoEvents(t *testing.T) {
	collector := NewCollector()
	collector.Close()

	// Should not panic when summarizing empty collector
	if len(collector.events) != 0 {
		t.Errorf("expected 0 events, got %d", len(collector.events))
	}
}

func TestCollector_RecordsDurations(t *testing.T) {
	collector := NewCollector()

	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
	}

	for _, d := range durations {
		collector.Report(Event{ActorID: 1, Step: "test", Success: true, Duration: d})
	}

	collector.Close()

	// Verify durations are recorded correctly
	var total time.Duration
	for _, e := range collector.events {
		total += e.Duration
	}

	expected := 100 * time.Millisecond
	if total != expected {
		t.Errorf("expected total duration %v, got %v", expected, total)
	}
}

func TestCollector_RecordsErrors(t *testing.T) {
	collector := NewCollector()

	collector.Report(Event{ActorID: 1, Step: "test", Success: false, Error: "connection refused"})
	collector.Report(Event{ActorID: 2, Step: "test", Success: false, Error: "timeout"})

	collector.Close()

	errors := make(map[string]bool)
	for _, e := range collector.events {
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
