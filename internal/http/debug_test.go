package http

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDebugLogger_LogRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDebugLogger(&buf)

	req, _ := http.NewRequest("POST", "http://example.com/api/users", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer token123")

	logger.LogRequest(1, "create_user", req)

	output := buf.String()

	// Should contain actor ID
	if !strings.Contains(output, "[Actor 1]") {
		t.Errorf("expected actor ID in output, got: %s", output)
	}

	// Should contain step name
	if !strings.Contains(output, "create_user") {
		t.Errorf("expected step name in output, got: %s", output)
	}

	// Should contain method and URL
	if !strings.Contains(output, "POST") {
		t.Errorf("expected method in output, got: %s", output)
	}
	if !strings.Contains(output, "http://example.com/api/users") {
		t.Errorf("expected URL in output, got: %s", output)
	}

	// Should contain headers
	if !strings.Contains(output, "Content-Type") {
		t.Errorf("expected Content-Type header in output, got: %s", output)
	}

	// Should contain body
	if !strings.Contains(output, `{"name":"test"}`) {
		t.Errorf("expected body in output, got: %s", output)
	}
}

func TestDebugLogger_LogResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDebugLogger(&buf)

	resp := &http.Response{
		StatusCode: 201,
		Status:     "201 Created",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	body := []byte(`{"id": 123, "name": "test"}`)

	logger.LogResponse(1, "create_user", resp, body, 150*time.Millisecond)

	output := buf.String()

	// Should contain actor ID
	if !strings.Contains(output, "[Actor 1]") {
		t.Errorf("expected actor ID in output, got: %s", output)
	}

	// Should contain status
	if !strings.Contains(output, "201") {
		t.Errorf("expected status code in output, got: %s", output)
	}

	// Should contain duration
	if !strings.Contains(output, "150ms") {
		t.Errorf("expected duration in output, got: %s", output)
	}

	// Should contain response body
	if !strings.Contains(output, `"id": 123`) {
		t.Errorf("expected response body in output, got: %s", output)
	}
}

func TestDebugLogger_LogError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDebugLogger(&buf)

	logger.LogError(1, "fetch_data", "connection refused", 50*time.Millisecond)

	output := buf.String()

	if !strings.Contains(output, "[Actor 1]") {
		t.Errorf("expected actor ID in output, got: %s", output)
	}
	if !strings.Contains(output, "fetch_data") {
		t.Errorf("expected step name in output, got: %s", output)
	}
	if !strings.Contains(output, "ERROR") {
		t.Errorf("expected ERROR in output, got: %s", output)
	}
	if !strings.Contains(output, "connection refused") {
		t.Errorf("expected error message in output, got: %s", output)
	}
}

func TestDebugLogger_TruncatesLongBodies(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDebugLogger(&buf)

	// Create a very long body
	longBody := strings.Repeat("x", 2000)
	req, _ := http.NewRequest("POST", "http://example.com/api", strings.NewReader(longBody))

	logger.LogRequest(1, "test", req)

	output := buf.String()

	// Should be truncated and indicate truncation
	if !strings.Contains(output, "truncated") {
		t.Errorf("expected long body to be truncated, got: %s", output)
	}
}

func TestDebugLogger_NilLogger(t *testing.T) {
	var logger *DebugLogger = nil

	// These should not panic
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	logger.LogRequest(1, "test", req)
	logger.LogResponse(1, "test", &http.Response{StatusCode: 200}, nil, time.Millisecond)
	logger.LogError(1, "test", "error", time.Millisecond)
}

func TestDebugLogger_EmptyBody(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDebugLogger(&buf)

	req, _ := http.NewRequest("GET", "http://example.com/api", nil)
	logger.LogRequest(1, "test", req)

	output := buf.String()

	// Should contain request info but no body
	if !strings.Contains(output, "GET") {
		t.Errorf("expected method in output, got: %s", output)
	}
}

func TestDebugLogger_ResponseWithNoBody(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDebugLogger(&buf)

	resp := &http.Response{
		StatusCode: 204,
		Status:     "204 No Content",
		Header:     http.Header{},
	}

	logger.LogResponse(1, "delete_user", resp, nil, 100*time.Millisecond)

	output := buf.String()

	if !strings.Contains(output, "204") {
		t.Errorf("expected status code in output, got: %s", output)
	}
}
