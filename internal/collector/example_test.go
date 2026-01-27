package collector_test

import (
	"fmt"
	"os"
	"time"

	"maestro/internal/collector"
	"maestro/internal/core"
)

func ExampleNewCollector() {
	// Create a new collector to aggregate events
	c := collector.NewCollector()

	// Report some events (typically done by actors)
	c.Report(core.Event{
		ActorID:  1,
		Step:     "login",
		Success:  true,
		Duration: 50 * time.Millisecond,
	})
	c.Report(core.Event{
		ActorID:  1,
		Step:     "fetch",
		Success:  true,
		Duration: 100 * time.Millisecond,
	})

	// Close when done collecting
	c.Close()

	// Get collected events
	events := c.Events()
	fmt.Printf("Collected %d events\n", len(events))
	// Output: Collected 2 events
}

func ExampleComputeMetrics() {
	events := []core.Event{
		{Step: "api", Success: true, Duration: 10 * time.Millisecond},
		{Step: "api", Success: true, Duration: 20 * time.Millisecond},
		{Step: "api", Success: true, Duration: 30 * time.Millisecond},
		{Step: "api", Success: false, Duration: 5 * time.Millisecond},
	}

	metrics := collector.ComputeMetrics(events, 1*time.Second)

	fmt.Printf("Total: %d, Success: %d, Rate: %.0f%%\n",
		metrics.TotalRequests, metrics.SuccessCount, metrics.SuccessRate)
	// Output: Total: 4, Success: 3, Rate: 75%
}

func ExampleFormatText() {
	events := []core.Event{
		{Step: "health", Success: true, Duration: 15 * time.Millisecond},
		{Step: "health", Success: true, Duration: 25 * time.Millisecond},
	}

	metrics := collector.ComputeMetrics(events, 1*time.Second)
	collector.FormatText(os.Stdout, metrics, nil)
}

func ExampleCollector_DroppedEvents() {
	c := collector.NewCollector()

	// After test run, check if any events were dropped
	c.Close()

	if dropped := c.DroppedEvents(); dropped > 0 {
		fmt.Printf("Warning: %d events dropped\n", dropped)
	} else {
		fmt.Println("No events dropped")
	}
	// Output: No events dropped
}
