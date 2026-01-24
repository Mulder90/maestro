package burstsmith

import (
	"fmt"
	"io"
	"os"
	"sync"
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
	output    io.Writer
	mu        sync.Mutex
}

// NewProgress creates a new progress indicator.
// If quiet is true, no progress will be displayed.
func NewProgress(collector *Collector, quiet bool) *Progress {
	return &Progress{
		collector: collector,
		quiet:     quiet,
		stopCh:    make(chan struct{}),
		output:    os.Stderr,
	}
}

// SetOutput sets the output writer for progress display.
func (p *Progress) SetOutput(w io.Writer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.output = w
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
	p.mu.Lock()
	fmt.Fprint(p.output, "\r\033[K")
	p.mu.Unlock()
}

// Print outputs a message, clearing the progress line first if active.
// The message will be followed by a newline.
func (p *Progress) Print(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Clear current progress line if not in quiet mode
	if !p.quiet {
		fmt.Fprint(p.output, "\r\033[K")
	}

	// Print the message with newline
	fmt.Fprintln(p.output, message)
}

// Printf outputs a formatted message, clearing the progress line first if active.
func (p *Progress) Printf(format string, args ...interface{}) {
	p.Print(fmt.Sprintf(format, args...))
}

// display prints the current progress.
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

	// Print with carriage return (overwrite previous line)
	p.mu.Lock()
	fmt.Fprintf(p.output, "\r\033[K%s", progressLine)
	p.mu.Unlock()
}

// formatElapsed formats duration as MM:SS.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}
