package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"maestro/internal/config"
	"maestro/internal/core"
)

func TestStep_SuccessfulGET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	step := NewStep(
		config.StepConfig{Name: "test", Method: "GET", URL: server.URL},
		&http.Client{Timeout: 5 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.StatusCode != 200 {
		t.Errorf("expected 200, got %d", result.StatusCode)
	}
}

func TestStep_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	step := NewStep(
		config.StepConfig{Name: "test", Method: "GET", URL: server.URL},
		&http.Client{Timeout: 5 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure")
	}
	if result.StatusCode != 500 {
		t.Errorf("expected 500, got %d", result.StatusCode)
	}
}

func TestStep_ConnectionError(t *testing.T) {
	step := NewStep(
		config.StepConfig{Name: "test", Method: "GET", URL: "http://localhost:99999"},
		&http.Client{Timeout: 1 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err == nil {
		t.Error("expected error")
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestStep_InvalidURL(t *testing.T) {
	// URL with invalid characters that will fail http.NewRequestWithContext
	step := NewStep(
		config.StepConfig{Name: "test", Method: "GET", URL: "http://[invalid-url"},
		&http.Client{Timeout: 1 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err == nil {
		t.Error("expected error for invalid URL")
	}
	if result.Success {
		t.Error("expected failure for invalid URL")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestStep_InvalidMethod(t *testing.T) {
	// Invalid HTTP method with space
	step := NewStep(
		config.StepConfig{Name: "test", Method: "INVALID METHOD", URL: "http://example.com"},
		&http.Client{Timeout: 1 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err == nil {
		t.Error("expected error for invalid method")
	}
	if result.Success {
		t.Error("expected failure for invalid method")
	}
}

func TestStep_POSTWithBody(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		receivedBody = string(body[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	requestBody := `{"name": "test", "value": 123}`
	step := NewStep(
		config.StepConfig{
			Name:   "test",
			Method: "POST",
			URL:    server.URL,
			Body:   requestBody,
		},
		&http.Client{Timeout: 5 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if result.StatusCode != 201 {
		t.Errorf("expected 201, got %d", result.StatusCode)
	}
	if receivedBody != requestBody {
		t.Errorf("expected body %q, got %q", requestBody, receivedBody)
	}
	if result.BytesSent != int64(len(requestBody)) {
		t.Errorf("expected BytesSent %d, got %d", len(requestBody), result.BytesSent)
	}
}

func TestStep_Headers(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	step := NewStep(
		config.StepConfig{
			Name:   "test",
			Method: "GET",
			URL:    server.URL,
			Headers: map[string]string{
				"X-Custom-Header": "custom-value",
				"Authorization":   "Bearer test-token",
			},
		},
		&http.Client{Timeout: 5 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected success")
	}
	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected X-Custom-Header, got %q", receivedHeaders.Get("X-Custom-Header"))
	}
	if receivedHeaders.Get("Authorization") != "Bearer test-token" {
		t.Errorf("expected Authorization header, got %q", receivedHeaders.Get("Authorization"))
	}
}

func TestStep_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	step := NewStep(
		config.StepConfig{Name: "test", Method: "GET", URL: server.URL},
		&http.Client{Timeout: 10 * time.Second},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ctx = core.ContextWithActorID(ctx, 1)

	result, err := step.Execute(ctx, core.NewVariables())

	if err == nil {
		t.Error("expected error due to context timeout")
	}
	if result.Success {
		t.Error("expected failure due to context timeout")
	}
}

func TestStep_Name(t *testing.T) {
	step := NewStep(
		config.StepConfig{Name: "my-step", Method: "GET", URL: "http://example.com"},
		&http.Client{},
		nil,
	)

	if step.Name() != "my-step" {
		t.Errorf("expected name 'my-step', got %q", step.Name())
	}
}

func TestStep_4xxStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	step := NewStep(
		config.StepConfig{Name: "test", Method: "GET", URL: server.URL},
		&http.Client{Timeout: 5 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4xx should be considered failure
	if result.Success {
		t.Error("expected failure for 404")
	}
	if result.StatusCode != 404 {
		t.Errorf("expected 404, got %d", result.StatusCode)
	}
	if result.Error == "" {
		t.Error("expected error message for 404")
	}
}

func TestStep_3xxStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	// Disable redirects to test raw 3xx response
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	step := NewStep(
		config.StepConfig{Name: "test", Method: "GET", URL: server.URL},
		client,
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 3xx should be considered success (< 400)
	if !result.Success {
		t.Error("expected success for 301")
	}
	if result.StatusCode != 301 {
		t.Errorf("expected 301, got %d", result.StatusCode)
	}
}

func TestStep_LargeBodyExtraction(t *testing.T) {
	// Create a response body larger than maxDebugBodySize (4KB) but within maxExtractBodySize (10MB)
	// to verify extraction works with large responses
	largeData := make([]byte, 8192) // 8KB - larger than debug limit
	for i := range largeData {
		largeData[i] = 'x'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// JSON with data field at the end (beyond 4KB mark)
		w.Write([]byte(`{"padding": "`))
		w.Write(largeData)
		w.Write([]byte(`", "target": "extracted_value"}`))
	}))
	defer server.Close()

	step := NewStep(
		config.StepConfig{
			Name:   "test",
			Method: "GET",
			URL:    server.URL,
			Extract: map[string]string{
				"result": "$.target",
			},
		},
		&http.Client{Timeout: 5 * time.Second},
		nil,
	)

	ctx := core.ContextWithActorID(context.Background(), 1)
	result, err := step.Execute(ctx, core.NewVariables())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Extract == nil {
		t.Fatal("expected extracted values")
	}
	if result.Extract["result"] != "extracted_value" {
		t.Errorf("expected 'extracted_value', got %v", result.Extract["result"])
	}
}
