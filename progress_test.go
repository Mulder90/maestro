package burstsmith

import (
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
