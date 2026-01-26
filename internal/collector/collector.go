// Package collector aggregates events and computes metrics.
package collector

import (
	"sync"
	"time"

	"maestro/internal/core"
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

// Events returns a copy of collected events.
func (c *Collector) Events() []core.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]core.Event, len(c.events))
	copy(result, c.events)
	return result
}

// Duration returns the test duration.
// If the collector is closed, returns the duration from start to end.
// If still running, returns the duration from start to now.
func (c *Collector) Duration() time.Duration {
	if !c.endTime.IsZero() {
		return c.endTime.Sub(c.startTime)
	}
	return time.Since(c.startTime)
}
