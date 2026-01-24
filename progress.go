package burstsmith

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// Progress displays live test progress to stderr.
type Progress struct {
	startTime time.Time
	collector *Collector
	ticker    *time.Ticker
	stopCh    chan struct{}
	stopped   atomic.Bool
	quiet     bool
}

// NewProgress creates a new progress indicator.
// If quiet is true, no progress will be displayed.
func NewProgress(collector *Collector, quiet bool) *Progress {
	return &Progress{
		collector: collector,
		quiet:     quiet,
		stopCh:    make(chan struct{}),
	}
}

// Start begins displaying progress updates every second.
func (p *Progress) Start() {
	if p.quiet {
		return
	}

	p.startTime = time.Now()
	p.ticker = time.NewTicker(1 * time.Second)

	go func() {
		for {
			select {
			case <-p.ticker.C:
				p.display()
			case <-p.stopCh:
				return
			}
		}
	}()
}

// Stop halts the progress display and clears the line.
func (p *Progress) Stop() {
	if p.quiet || p.stopped.Swap(true) {
		return
	}

	if p.ticker != nil {
		p.ticker.Stop()
	}
	close(p.stopCh)

	// Clear the progress line
	fmt.Fprint(os.Stderr, "\r\033[K")
}

// display prints the current progress to stderr.
func (p *Progress) display() {
	p.collector.mu.Lock()
	eventCount := len(p.collector.events)
	successCount := 0
	failureCount := 0
	for _, e := range p.collector.events {
		if e.Success {
			successCount++
		} else {
			failureCount++
		}
	}
	p.collector.mu.Unlock()

	elapsed := time.Since(p.startTime)
	elapsedStr := formatElapsed(elapsed)

	var rps float64
	if elapsed > 0 {
		rps = float64(eventCount) / elapsed.Seconds()
	}

	var errorRate float64
	if eventCount > 0 {
		errorRate = float64(failureCount) / float64(eventCount) * 100
	}

	// Format: [00:30] Requests: 1523 | RPS: 50.8 | Errors: 2 (0.1%)
	progressLine := fmt.Sprintf("[%s] Requests: %d | RPS: %.1f | Errors: %d (%.1f%%)",
		elapsedStr, eventCount, rps, failureCount, errorRate)

	// Print to stderr with carriage return (overwrite previous line)
	fmt.Fprintf(os.Stderr, "\r\033[K%s", progressLine)
}

// formatElapsed formats duration as MM:SS.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
