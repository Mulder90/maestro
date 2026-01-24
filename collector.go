package burstsmith

import (
	"fmt"
	"sync"
	"time"
)

// Collector aggregates events from actors and produces a summary.
type Collector struct {
	events []Event
	ch     chan Event
	done   chan struct{}
	mu     sync.Mutex
}

// NewCollector creates a new Collector and starts its collection goroutine.
func NewCollector() *Collector {
	c := &Collector{
		events: make([]Event, 0),
		ch:     make(chan Event, 1000),
		done:   make(chan struct{}),
	}
	go c.collect()
	return c
}

// collect runs in a goroutine, receiving events until the channel is closed.
func (c *Collector) collect() {
	for event := range c.ch {
		c.mu.Lock()
		c.events = append(c.events, event)
		c.mu.Unlock()
	}
	close(c.done)
}

// Report sends an event to the collector. Thread-safe.
func (c *Collector) Report(event Event) {
	select {
	case c.ch <- event:
	default:
		// Channel full, drop event (could log this in production)
	}
}

// Close signals the collector to stop accepting events.
func (c *Collector) Close() {
	close(c.ch)
	<-c.done
}

// Summary computes and prints aggregated statistics.
func (c *Collector) Summary() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.events) == 0 {
		fmt.Println("No events collected")
		return
	}

	total := len(c.events)
	successCount := 0
	var totalDuration time.Duration
	stepCounts := make(map[string]int)
	stepDurations := make(map[string]time.Duration)

	for _, e := range c.events {
		if e.Success {
			successCount++
		}
		totalDuration += e.Duration
		stepCounts[e.Step]++
		stepDurations[e.Step] += e.Duration
	}

	successRate := float64(successCount) / float64(total) * 100
	avgDuration := totalDuration / time.Duration(total)

	fmt.Println("\n========== Summary ==========")
	fmt.Printf("Total events:   %d\n", total)
	fmt.Printf("Success rate:   %.1f%%\n", successRate)
	fmt.Printf("Avg duration:   %v\n", avgDuration)
	fmt.Println("\nBy step:")
	for step, count := range stepCounts {
		avg := stepDurations[step] / time.Duration(count)
		fmt.Printf("  %-20s count=%-5d avg=%v\n", step, count, avg)
	}
	fmt.Println("=============================")
}
