package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxBodyLogSize = 1024

type DebugLogger struct {
	out io.Writer
	mu  sync.Mutex
}

func NewDebugLogger(out io.Writer) *DebugLogger {
	return &DebugLogger{out: out}
}

func (d *DebugLogger) LogRequest(actorID int, stepName string, req *http.Request) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("\n[Actor %d] >>> REQUEST: %s\n", actorID, stepName))
	buf.WriteString(fmt.Sprintf("  %s %s\n", req.Method, req.URL.String()))

	if len(req.Header) > 0 {
		buf.WriteString("  Headers:\n")
		for name, values := range req.Header {
			buf.WriteString(fmt.Sprintf("    %s: %s\n", name, strings.Join(values, ", ")))
		}
	}

	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(req.Body)
		if err == nil && len(body) > 0 {
			req.Body = io.NopCloser(bytes.NewReader(body))
			buf.WriteString(fmt.Sprintf("  Body: %s\n", truncateBody(body)))
		}
	}
	fmt.Fprint(d.out, buf.String())
}

func (d *DebugLogger) LogResponse(actorID int, stepName string, resp *http.Response, body []byte, duration time.Duration) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("[Actor %d] <<< RESPONSE: %s (%s)\n", actorID, stepName, duration.Round(time.Millisecond)))
	buf.WriteString(fmt.Sprintf("  Status: %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode)))

	if len(resp.Header) > 0 {
		buf.WriteString("  Headers:\n")
		for name, values := range resp.Header {
			buf.WriteString(fmt.Sprintf("    %s: %s\n", name, strings.Join(values, ", ")))
		}
	}

	if len(body) > 0 {
		buf.WriteString(fmt.Sprintf("  Body: %s\n", truncateBody(body)))
	}
	fmt.Fprint(d.out, buf.String())
}

func (d *DebugLogger) LogError(actorID int, stepName string, errMsg string, duration time.Duration) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fmt.Fprintf(d.out, "[Actor %d] !!! ERROR: %s (%s)\n  %s\n",
		actorID, stepName, duration.Round(time.Millisecond), errMsg)
}

func truncateBody(body []byte) string {
	if len(body) <= maxBodyLogSize {
		return string(body)
	}
	return string(body[:maxBodyLogSize]) + fmt.Sprintf("... (truncated, %d bytes total)", len(body))
}
