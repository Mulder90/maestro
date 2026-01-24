package burstsmith

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Integration tests verify end-to-end behavior of the load testing tool

func TestIntegration_SimpleWorkflow(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create config file
	configContent := `
workflow:
  name: "Integration Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	// Load config
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Run test
	collector := NewCollector()
	coordinator := NewCoordinator(collector)

	workflow := &HTTPWorkflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coordinator.Spawn(ctx, 3, workflow)
	coordinator.Wait()
	collector.Close()

	// Verify results
	if requestCount.Load() == 0 {
		t.Error("expected requests to be made")
	}
	if len(collector.events) == 0 {
		t.Error("expected events to be collected")
	}

	// All should be successful
	for _, e := range collector.events {
		if !e.Success {
			t.Errorf("expected successful event, got error: %s", e.Error)
		}
	}
}

func TestIntegration_MultiStepWorkflow(t *testing.T) {
	stepCounts := make(map[string]*atomic.Int32)
	stepCounts["/login"] = &atomic.Int32{}
	stepCounts["/api/data"] = &atomic.Int32{}
	stepCounts["/logout"] = &atomic.Int32{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if counter, ok := stepCounts[r.URL.Path]; ok {
			counter.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Multi-Step Test"
  steps:
    - name: "login"
      method: POST
      url: "` + server.URL + `/login"
    - name: "fetch_data"
      method: GET
      url: "` + server.URL + `/api/data"
    - name: "logout"
      method: POST
      url: "` + server.URL + `/logout"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	collector := NewCollector()
	coordinator := NewCoordinator(collector)

	workflow := &HTTPWorkflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	coordinator.Spawn(ctx, 2, workflow)
	coordinator.Wait()
	collector.Close()

	// Each actor should hit all 3 endpoints in order, multiple times
	loginCount := stepCounts["/login"].Load()
	dataCount := stepCounts["/api/data"].Load()
	logoutCount := stepCounts["/logout"].Load()

	if loginCount == 0 || dataCount == 0 || logoutCount == 0 {
		t.Errorf("expected all endpoints to be hit: login=%d, data=%d, logout=%d",
			loginCount, dataCount, logoutCount)
	}

	// All steps should be called roughly equally (since they're in a sequence)
	if loginCount != logoutCount {
		t.Errorf("expected equal login/logout counts: login=%d, logout=%d", loginCount, logoutCount)
	}
}

func TestIntegration_LoadProfileRampUp(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Ramp Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"

loadProfile:
  phases:
    - name: "ramp_up"
      duration: 200ms
      startActors: 1
      endActors: 5
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.LoadProfile == nil {
		t.Fatal("expected load profile to be parsed")
	}

	collector := NewCollector()
	coordinator := NewCoordinator(collector)

	workflow := &HTTPWorkflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coordinator.RunWithProfile(ctx, cfg.LoadProfile, workflow, nil, nil)
	coordinator.Wait()
	collector.Close()

	// Should have requests from increasing number of actors
	actorIDs := make(map[int]bool)
	for _, e := range collector.events {
		actorIDs[e.ActorID] = true
	}

	if len(actorIDs) < 2 {
		t.Errorf("expected multiple actors during ramp, got %d", len(actorIDs))
	}
}

func TestIntegration_LoadProfileWithRateLimit(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Rate Limited Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"

loadProfile:
  phases:
    - name: "steady"
      duration: 300ms
      actors: 5
      rps: 30
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	collector := NewCollector()
	coordinator := NewCoordinator(collector)

	// Create rate limiter based on config
	rateLimiter := NewRateLimiter(cfg.LoadProfile.Phases[0].RPS)

	workflow := &HTTPWorkflow{
		Config:      cfg.Workflow,
		Client:      &http.Client{Timeout: 5 * time.Second},
		RateLimiter: rateLimiter,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coordinator.RunWithProfile(ctx, cfg.LoadProfile, workflow, rateLimiter, nil)
	coordinator.Wait()
	collector.Close()

	// With 30 RPS over 300ms, expect roughly 30*0.3 + 30 (burst) = ~39 requests max
	count := requestCount.Load()
	if count > 60 {
		t.Errorf("rate limiting may not be working, got %d requests (expected <60)", count)
	}
}

func TestIntegration_BackwardCompatibility(t *testing.T) {
	// Test that configs without loadProfile work with classic mode
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Config without loadProfile
	configContent := `
workflow:
  name: "Classic Mode Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify no load profile
	if cfg.LoadProfile != nil {
		t.Error("expected nil load profile for backward compatible config")
	}

	// Run classic mode
	collector := NewCollector()
	coordinator := NewCoordinator(collector)

	workflow := &HTTPWorkflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coordinator.Spawn(ctx, 3, workflow)
	coordinator.Wait()
	collector.Close()

	if requestCount.Load() == 0 {
		t.Error("expected requests in classic mode")
	}
}

func TestIntegration_ErrorHandling(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count%2 == 0 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Error Test"
  steps:
    - name: "flaky"
      method: GET
      url: "` + server.URL + `"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	collector := NewCollector()
	coordinator := NewCoordinator(collector)

	workflow := &HTTPWorkflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coordinator.Spawn(ctx, 2, workflow)
	coordinator.Wait()
	collector.Close()

	// Should have a mix of successes and failures
	successCount := 0
	failureCount := 0
	for _, e := range collector.events {
		if e.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	if successCount == 0 {
		t.Error("expected some successful events")
	}
	if failureCount == 0 {
		t.Error("expected some failed events")
	}
}

func TestIntegration_HeadersAndBody(t *testing.T) {
	var receivedContentType string
	var receivedAuth string
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedAuth = r.Header.Get("Authorization")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Headers Test"
  steps:
    - name: "create"
      method: POST
      url: "` + server.URL + `"
      headers:
        Content-Type: "application/json"
        Authorization: "Bearer secret-token"
      body: '{"name": "test", "value": 123}'
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	collector := NewCollector()
	coordinator := NewCoordinator(collector)

	workflow := &HTTPWorkflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coordinator.Spawn(ctx, 1, workflow)
	coordinator.Wait()
	collector.Close()

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type header, got %q", receivedContentType)
	}
	if receivedAuth != "Bearer secret-token" {
		t.Errorf("expected Authorization header, got %q", receivedAuth)
	}
	if receivedBody != `{"name": "test", "value": 123}` {
		t.Errorf("expected body, got %q", receivedBody)
	}
}

// Helper function
func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp config: %v", err)
	}
	return tmpFile
}
