package burstsmith

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxBodyLogSize = 1024 // Max bytes to log for request/response bodies

// DebugLogger logs HTTP request/response details for debugging.
type DebugLogger struct {
	out io.Writer
	mu  sync.Mutex
}

// NewDebugLogger creates a new debug logger that writes to the given writer.
func NewDebugLogger(out io.Writer) *DebugLogger {
	return &DebugLogger{out: out}
}

// LogRequest logs details of an outgoing HTTP request.
func (d *DebugLogger) LogRequest(actorID int, stepName string, req *http.Request) {
	if d == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("\n[Actor %d] >>> REQUEST: %s\n", actorID, stepName))
	buf.WriteString(fmt.Sprintf("  %s %s\n", req.Method, req.URL.String()))

	// Log headers
	if len(req.Header) > 0 {
		buf.WriteString("  Headers:\n")
		for name, values := range req.Header {
			buf.WriteString(fmt.Sprintf("    %s: %s\n", name, strings.Join(values, ", ")))
		}
	}

	// Log body if present
	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(req.Body)
		if err == nil && len(body) > 0 {
			// Restore the body for the actual request
			req.Body = io.NopCloser(bytes.NewReader(body))

			bodyStr := truncateBody(body)
			buf.WriteString(fmt.Sprintf("  Body: %s\n", bodyStr))
		}
	}

	fmt.Fprint(d.out, buf.String())
}

// LogResponse logs details of an HTTP response.
func (d *DebugLogger) LogResponse(actorID int, stepName string, resp *http.Response, body []byte, duration time.Duration) {
	if d == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("[Actor %d] <<< RESPONSE: %s (%s)\n", actorID, stepName, duration.Round(time.Millisecond)))
	buf.WriteString(fmt.Sprintf("  Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode)))

	// Log selected response headers
	if len(resp.Header) > 0 {
		buf.WriteString("  Headers:\n")
		for name, values := range resp.Header {
			buf.WriteString(fmt.Sprintf("    %s: %s\n", name, strings.Join(values, ", ")))
		}
	}

	// Log body if present
	if len(body) > 0 {
		bodyStr := truncateBody(body)
		buf.WriteString(fmt.Sprintf("  Body: %s\n", bodyStr))
	}

	fmt.Fprint(d.out, buf.String())
}

// LogError logs an error that occurred during a request.
func (d *DebugLogger) LogError(actorID int, stepName string, errMsg string, duration time.Duration) {
	if d == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	fmt.Fprintf(d.out, "[Actor %d] !!! ERROR: %s (%s)\n  %s\n",
		actorID, stepName, duration.Round(time.Millisecond), errMsg)
}

// truncateBody truncates a body to maxBodyLogSize and indicates if truncated.
func truncateBody(body []byte) string {
	if len(body) <= maxBodyLogSize {
		return string(body)
	}
	return string(body[:maxBodyLogSize]) + fmt.Sprintf("... (truncated, %d bytes total)", len(body))
}
