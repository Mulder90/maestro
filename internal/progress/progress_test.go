package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"burstsmith/internal/collector"
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
