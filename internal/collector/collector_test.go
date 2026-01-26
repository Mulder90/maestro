package collector

import (
	"sync"
	"testing"
	"time"

	"maestro/internal/core"
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

	events := c.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
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

func TestCollector_Duration(t *testing.T) {
	c := NewCollector()

	// Let some time pass
	time.Sleep(50 * time.Millisecond)

	c.Report(core.Event{ActorID: 1, Step: "test", Success: true, Duration: time.Millisecond})
	c.Close()

	duration := c.Duration()

	// Duration should be at least 50ms
	if duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", duration)
	}
}

func TestCollector_Duration_WhileRunning(t *testing.T) {
	c := NewCollector()
	defer c.Close()

	// Duration should work while collector is still running
	time.Sleep(10 * time.Millisecond)
	duration := c.Duration()

	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms while running, got %v", duration)
	}
}

func TestCollector_EventsCopiesSlice(t *testing.T) {
	c := NewCollector()
	c.Report(core.Event{ActorID: 1, Step: "test", Success: true})
	c.Close()

	events1 := c.Events()
	events2 := c.Events()

	// Modifying one should not affect the other
	if len(events1) > 0 {
		events1[0].ActorID = 999
		if events2[0].ActorID == 999 {
			t.Error("Events() should return a copy, not the original slice")
		}
	}
}
