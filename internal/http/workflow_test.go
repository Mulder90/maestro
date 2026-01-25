package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"burstsmith/internal/collector"
	"burstsmith/internal/config"
	"burstsmith/internal/core"
	"burstsmith/internal/ratelimit"
)

func TestHTTPWorkflow_SuccessfulGET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "get", Method: "GET", URL: server.URL},
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := c.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if !events[0].Success {
		t.Error("expected successful event")
	}
}

func TestHTTPWorkflow_SuccessfulPOST(t *testing.T) {
	var receivedBody string
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		receivedContentType = r.Header.Get("Content-Type")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{
					Name:    "post",
					Method:  "POST",
					URL:     server.URL,
					Headers: map[string]string{"Content-Type": "application/json"},
					Body:    `{"name":"test"}`,
				},
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}
	if receivedBody != `{"name":"test"}` {
		t.Errorf("expected body, got %s", receivedBody)
	}
}

func TestHTTPWorkflow_MultipleSteps(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "step1", Method: "GET", URL: server.URL + "/1"},
				{Name: "step2", Method: "GET", URL: server.URL + "/2"},
				{Name: "step3", Method: "GET", URL: server.URL + "/3"},
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestCount.Load() != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount.Load())
	}
	events := c.Events()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}
}

func TestHTTPWorkflow_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "fail", Method: "GET", URL: server.URL},
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	// HTTP errors don't return error, but are marked as unsuccessful
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	events := c.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Success {
		t.Error("expected unsuccessful event for 500 status")
	}
	if events[0].Error == "" {
		t.Error("expected error message in event")
	}
}

func TestHTTPWorkflow_ConnectionError(t *testing.T) {
	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "fail", Method: "GET", URL: "http://localhost:99999"}, // Invalid port
			},
		},
		Client: &http.Client{Timeout: 1 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	// Connection errors do return error
	if err == nil {
		t.Error("expected error for connection failure")
	}
	events := c.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Success {
		t.Error("expected unsuccessful event")
	}
}

func TestHTTPWorkflow_ContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		// Block until request context is done
		<-r.Context().Done()
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "slow", Method: "GET", URL: server.URL},
			},
		},
		Client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_ = workflow.Run(ctx, 1, nil, c)
	elapsed := time.Since(start)
	c.Close()

	// Should cancel within the timeout window
	if elapsed > 300*time.Millisecond {
		t.Errorf("context cancellation didn't work, took %v", elapsed)
	}
}

func TestHTTPWorkflow_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{
					Name:   "headers",
					Method: "GET",
					URL:    server.URL,
					Headers: map[string]string{
						"Authorization": "Bearer token123",
						"X-Custom":      "custom-value",
					},
				},
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("expected Authorization header, got %s", receivedHeaders.Get("Authorization"))
	}
	if receivedHeaders.Get("X-Custom") != "custom-value" {
		t.Errorf("expected X-Custom header, got %s", receivedHeaders.Get("X-Custom"))
	}
}

func TestHTTPWorkflow_WithRateLimiter(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := collector.NewCollector()
	rateLimiter := ratelimit.NewRateLimiter(10) // 10 RPS

	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "limited", Method: "GET", URL: server.URL},
			},
		},
		Client:      &http.Client{Timeout: 5 * time.Second},
		RateLimiter: rateLimiter,
	}

	// Run for 500ms with rate limit of 10 RPS
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	for {
		if ctx.Err() != nil {
			break
		}
		err := workflow.Run(ctx, 1, nil, c)
		if err != nil {
			break
		}
	}
	c.Close()

	// With 10 RPS over 500ms, expect roughly 5-6 requests (initial burst + sustained)
	// Token bucket allows burst up to the limit, so first 10 go through immediately
	count := requestCount.Load()
	if count < 3 || count > 15 {
		t.Errorf("unexpected request count with rate limiting: got %d requests in 500ms (expected 3-15 with 10 RPS)", count)
	}
}

func TestHTTPWorkflow_RecordsDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "slow", Method: "GET", URL: server.URL},
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := c.Events()
	duration := events[0].Duration
	if duration < 40*time.Millisecond || duration > 200*time.Millisecond {
		t.Errorf("expected duration ~50ms, got %v", duration)
	}
}

func TestHTTPWorkflow_AllHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var receivedMethod string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			c := collector.NewCollector()
			workflow := &Workflow{
				Config: config.WorkflowConfig{
					Name: "Test",
					Steps: []config.StepConfig{
						{Name: "test", Method: method, URL: server.URL},
					},
				},
				Client: &http.Client{Timeout: 5 * time.Second},
			}

			_ = workflow.Run(context.Background(), 1, nil, c)
			c.Close()

			if receivedMethod != method {
				t.Errorf("expected method %s, got %s", method, receivedMethod)
			}
		})
	}
}

func TestHTTPWorkflow_ContinuesAfterHTTPError(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		if count == 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	c := collector.NewCollector()
	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "step1", Method: "GET", URL: server.URL},
				{Name: "step2", Method: "GET", URL: server.URL}, // This will fail
				{Name: "step3", Method: "GET", URL: server.URL}, // Should still run
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	err := workflow.Run(context.Background(), 1, nil, c)
	c.Close()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All 3 steps should run even though step2 returned 500
	if requestCount.Load() != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount.Load())
	}
	events := c.Events()
	if len(events) != 3 {
		t.Errorf("expected 3 events, got %d", len(events))
	}

	// Check success/failure pattern
	if !events[0].Success {
		t.Error("step1 should be successful")
	}
	if events[1].Success {
		t.Error("step2 should be unsuccessful")
	}
	if !events[2].Success {
		t.Error("step3 should be successful")
	}
}

func TestHTTPWorkflow_VerboseMode(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	var debugBuf core.MockWriter
	c := collector.NewCollector()
	defer c.Close()

	workflow := &Workflow{
		Config: config.WorkflowConfig{
			Name: "Test",
			Steps: []config.StepConfig{
				{Name: "health", Method: "GET", URL: server.URL + "/health"},
			},
		},
		Client: &http.Client{Timeout: 5 * time.Second},
		Debug:  NewDebugLogger(&debugBuf),
	}

	ctx := context.Background()
	err := workflow.Run(ctx, 1, nil, c)
	if err != nil {
		t.Fatalf("workflow failed: %v", err)
	}

	output := debugBuf.String()

	// Should log request
	if output == "" {
		t.Error("expected debug output, got empty string")
	}
}
