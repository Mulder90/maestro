package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"maestro/internal/collector"
	"maestro/internal/core"
)

func TestNewProgress(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	progress := NewProgress(c, false)

	if progress.collector != c {
		t.Error("collector not assigned")
	}
	if progress.quiet {
		t.Error("quiet should be false")
	}
}

func TestNewProgress_Quiet(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	progress := NewProgress(c, true)

	if !progress.quiet {
		t.Error("quiet should be true")
	}
}

func TestProgress_QuietMode(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	progress := NewProgress(c, true) // quiet mode

	// Start and stop should not panic in quiet mode
	progress.Start()
	time.Sleep(10 * time.Millisecond)
	progress.Stop()
}

func TestProgress_DoubleStop(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	progress := NewProgress(c, true)
	progress.Start()

	// Double stop should not panic
	progress.Stop()
	progress.Stop()
}

func TestProgress_StopWithoutStart(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	progress := NewProgress(c, false)

	// Stop without start should not panic
	progress.Stop()
}

func TestProgress_Print(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	var buf bytes.Buffer
	progress := NewProgress(c, false)
	progress.SetOutput(&buf)

	progress.Print("Phase: test (duration: 10s)")

	output := buf.String()

	// Should contain the escape sequence to clear line before message
	if !strings.Contains(output, "\033[K") {
		t.Error("expected output to contain line clear escape sequence")
	}

	// Should contain the message
	if !strings.Contains(output, "Phase: test (duration: 10s)") {
		t.Errorf("expected output to contain message, got: %q", output)
	}

	// Message should end with newline
	if !strings.Contains(output, "Phase: test (duration: 10s)\n") {
		t.Error("expected message to end with newline")
	}
}

func TestProgress_Print_QuietModeDoesNotPrint(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	var buf bytes.Buffer
	progress := NewProgress(c, true) // quiet mode
	progress.SetOutput(&buf)

	progress.Print("Phase: test")

	output := buf.String()

	// In quiet mode, Print should not output
	if output != "" {
		t.Errorf("expected no output in quiet mode, got: %q", output)
	}
}

func TestProgress_Printf(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	var buf bytes.Buffer
	progress := NewProgress(c, false)
	progress.SetOutput(&buf)

	progress.Printf("Phase: %s (actors: %d)", "warmup", 10)

	output := buf.String()

	if !strings.Contains(output, "Phase: warmup (actors: 10)\n") {
		t.Errorf("expected formatted message, got: %q", output)
	}
}

func TestProgress_SetOutput(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	var buf1, buf2 bytes.Buffer
	progress := NewProgress(c, false)

	progress.SetOutput(&buf1)
	progress.Print("message1")

	progress.SetOutput(&buf2)
	progress.Print("message2")

	if !strings.Contains(buf1.String(), "message1") {
		t.Error("expected message1 in buf1")
	}
	if !strings.Contains(buf2.String(), "message2") {
		t.Error("expected message2 in buf2")
	}
	if strings.Contains(buf1.String(), "message2") {
		t.Error("buf1 should not contain message2")
	}
}

func TestProgress_PrintProgress_Format(t *testing.T) {
	c := collector.NewCollector()

	// Add some events to the collector
	c.Report(core.Event{Success: true, Duration: 100 * time.Millisecond})
	c.Report(core.Event{Success: true, Duration: 200 * time.Millisecond})
	c.Report(core.Event{Success: false, Duration: 50 * time.Millisecond})

	// Wait for events to be collected
	time.Sleep(50 * time.Millisecond)

	var buf bytes.Buffer
	progress := NewProgress(c, false)
	progress.SetOutput(&buf)
	progress.startTime = time.Now() // Initialize start time

	// Directly call printProgress to test output format
	progress.printProgress()

	output := buf.String()

	// Should contain carriage return and clear line escape sequence
	if !strings.Contains(output, "\r\033[K") {
		t.Errorf("expected carriage return and line clear, got: %q", output)
	}

	// Should contain time format [MM:SS]
	if !strings.Contains(output, "[00:00]") {
		t.Errorf("expected time format [00:00], got: %q", output)
	}

	// Should contain Requests count
	if !strings.Contains(output, "Requests: 3") {
		t.Errorf("expected 'Requests: 3', got: %q", output)
	}

	// Should contain RPS
	if !strings.Contains(output, "RPS:") {
		t.Errorf("expected RPS field, got: %q", output)
	}

	// Should contain Errors count (1 failure)
	if !strings.Contains(output, "Errors: 1") {
		t.Errorf("expected 'Errors: 1', got: %q", output)
	}

	c.Close()
}

func TestProgress_PrintProgress_ZeroRequests(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	var buf bytes.Buffer
	progress := NewProgress(c, false)
	progress.SetOutput(&buf)

	// Call printProgress with no events
	progress.printProgress()

	output := buf.String()

	// Should show 0 requests without division error
	if !strings.Contains(output, "Requests: 0") {
		t.Errorf("expected 'Requests: 0', got: %q", output)
	}

	// Error rate should be 0% (not NaN or panic)
	if !strings.Contains(output, "0.0%") {
		t.Errorf("expected '0.0%%' error rate with zero requests, got: %q", output)
	}
}

func TestProgress_PrintProgress_ErrorRate(t *testing.T) {
	c := collector.NewCollector()

	// Add 10 events: 8 success, 2 failures = 20% error rate
	for i := 0; i < 8; i++ {
		c.Report(core.Event{Success: true, Duration: 10 * time.Millisecond})
	}
	for i := 0; i < 2; i++ {
		c.Report(core.Event{Success: false, Duration: 10 * time.Millisecond})
	}

	// Wait for events to be collected
	time.Sleep(50 * time.Millisecond)

	var buf bytes.Buffer
	progress := NewProgress(c, false)
	progress.SetOutput(&buf)
	progress.startTime = time.Now() // Initialize start time

	progress.printProgress()

	output := buf.String()

	// Should show 20% error rate
	if !strings.Contains(output, "20.0%") {
		t.Errorf("expected '20.0%%' error rate, got: %q", output)
	}

	c.Close()
}

func TestProgress_Start_PrintsProgress(t *testing.T) {
	c := collector.NewCollector()

	// Add some events
	c.Report(core.Event{Success: true, Duration: 50 * time.Millisecond})
	c.Report(core.Event{Success: true, Duration: 50 * time.Millisecond})

	var buf bytes.Buffer
	progress := NewProgress(c, false)
	progress.SetOutput(&buf)

	progress.Start()

	// Wait for at least one tick (1 second)
	time.Sleep(1100 * time.Millisecond)

	progress.Stop()

	output := buf.String()

	// Should have printed progress at least once
	if !strings.Contains(output, "Requests:") {
		t.Errorf("expected progress output after Start(), got: %q", output)
	}

	c.Close()
}

func TestProgress_Run_StopsOnStopChannel(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	progress := NewProgress(c, false)
	progress.stopCh = make(chan struct{})
	progress.ticker = time.NewTicker(10 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		progress.run()
		close(done)
	}()

	// Close stop channel
	close(progress.stopCh)

	// run() should exit
	select {
	case <-done:
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Error("run() did not exit after stopCh closed")
	}

	progress.ticker.Stop()
}

func TestProgress_Stop_ClearsLine(t *testing.T) {
	c := collector.NewCollector()
	defer c.Close()

	var buf bytes.Buffer
	progress := NewProgress(c, false)
	progress.SetOutput(&buf)

	progress.Start()
	time.Sleep(50 * time.Millisecond) // Let it initialize
	progress.Stop()

	output := buf.String()

	// Stop should output line clear
	if !strings.Contains(output, "\r\033[K") {
		t.Errorf("expected Stop() to clear line, got: %q", output)
	}
}
