package burstsmith

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "00:00"},
		{30 * time.Second, "00:30"},
		{60 * time.Second, "01:00"},
		{90 * time.Second, "01:30"},
		{5 * time.Minute, "05:00"},
		{10*time.Minute + 30*time.Second, "10:30"},
		{59*time.Minute + 59*time.Second, "59:59"},
		{60 * time.Minute, "60:00"}, // handles > 59 minutes
	}

	for _, tt := range tests {
		result := formatElapsed(tt.input)
		if result != tt.expected {
			t.Errorf("formatElapsed(%v): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestProgress_QuietMode(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	progress := NewProgress(collector, true) // quiet mode

	// Start and stop should not panic in quiet mode
	progress.Start()
	time.Sleep(10 * time.Millisecond)
	progress.Stop()
}

func TestProgress_DoubleStop(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	progress := NewProgress(collector, true)
	progress.Start()

	// Double stop should not panic
	progress.Stop()
	progress.Stop()
}

func TestProgress_StopWithoutStart(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	progress := NewProgress(collector, false)

	// Stop without start should not panic
	progress.Stop()
}

func TestNewProgress(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	progress := NewProgress(collector, false)

	if progress.collector != collector {
		t.Error("collector not assigned")
	}
	if progress.quiet {
		t.Error("quiet should be false")
	}
	if progress.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

func TestNewProgress_Quiet(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	progress := NewProgress(collector, true)

	if !progress.quiet {
		t.Error("quiet should be true")
	}
}

func TestProgress_Print_ClearsLineAndPrintsMessage(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	var buf bytes.Buffer
	progress := NewProgress(collector, false)
	progress.SetOutput(&buf)
	progress.Start()

	// Give progress time to display
	time.Sleep(50 * time.Millisecond)

	// Print should clear line and print message
	progress.Print("Phase: test (duration: 10s)")

	progress.Stop()

	output := buf.String()

	// Should contain the escape sequence to clear line before message
	if !strings.Contains(output, "\r\033[K") {
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

func TestProgress_Print_QuietModeStillPrints(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	var buf bytes.Buffer
	progress := NewProgress(collector, true) // quiet mode
	progress.SetOutput(&buf)

	progress.Print("Phase: test")

	output := buf.String()

	// In quiet mode, Print should still output the message (just no progress)
	if !strings.Contains(output, "Phase: test\n") {
		t.Errorf("expected message in quiet mode, got: %q", output)
	}
}

func TestProgress_Printf_FormatsAndPrints(t *testing.T) {
	collector := NewCollector()
	defer collector.Close()

	var buf bytes.Buffer
	progress := NewProgress(collector, false)
	progress.SetOutput(&buf)

	progress.Printf("Phase: %s (actors: %d)", "warmup", 10)

	output := buf.String()

	if !strings.Contains(output, "Phase: warmup (actors: 10)\n") {
		t.Errorf("expected formatted message, got: %q", output)
	}
}
