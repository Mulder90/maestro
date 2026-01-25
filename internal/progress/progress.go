package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"burstsmith/internal/collector"
)

type Progress struct {
	startTime time.Time
	collector *collector.Collector
	ticker    *time.Ticker
	stopCh    chan struct{}
	stopped   atomic.Bool
	quiet     bool
	output    io.Writer
	mu        sync.Mutex
}

func NewProgress(c *collector.Collector, quiet bool) *Progress {
	return &Progress{
		collector: c,
		quiet:     quiet,
		output:    os.Stderr,
	}
}

func (p *Progress) SetOutput(w io.Writer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.output = w
}

func (p *Progress) Start() {
	if p.quiet {
		return
	}
	p.startTime = time.Now()
	p.stopCh = make(chan struct{})
	p.ticker = time.NewTicker(1 * time.Second)
	go p.run()
}

func (p *Progress) run() {
	for {
		select {
		case <-p.stopCh:
			return
		case <-p.ticker.C:
			p.printProgress()
		}
	}
}

func (p *Progress) printProgress() {
	m := p.collector.Compute()
	elapsed := time.Since(p.startTime).Round(time.Second)
	mins := int(elapsed.Minutes())
	secs := int(elapsed.Seconds()) % 60
	errorRate := 0.0
	if m.TotalRequests > 0 {
		errorRate = float64(m.FailureCount) / float64(m.TotalRequests) * 100
	}
	p.mu.Lock()
	fmt.Fprintf(p.output, "\033[K[%02d:%02d] Requests: %d | RPS: %.1f | Errors: %d (%.1f%%)",
		mins, secs, m.TotalRequests, m.RequestsPerSec, m.FailureCount, errorRate)
	p.mu.Unlock()
}

func (p *Progress) Stop() {
	if p.quiet || p.stopped.Swap(true) {
		return
	}
	if p.ticker != nil {
		p.ticker.Stop()
	}
	if p.stopCh != nil {
		close(p.stopCh)
	}
	p.mu.Lock()
	fmt.Fprintf(p.output, "\033[K")
	p.mu.Unlock()
}

func (p *Progress) Print(message string) {
	if p.quiet {
		return
	}
	p.mu.Lock()
	fmt.Fprintf(p.output, "\033[K%s\n", message)
	p.mu.Unlock()
}

func (p *Progress) Printf(format string, args ...interface{}) {
	if p.quiet {
		return
	}
	p.mu.Lock()
	fmt.Fprintf(p.output, "\033[K"+format+"\n", args...)
	p.mu.Unlock()
}
