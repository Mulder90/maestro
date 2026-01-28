package maestro_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"maestro/internal/collector"
	"maestro/internal/config"
	"maestro/internal/coordinator"
	"maestro/internal/core"
	"maestro/internal/data"
	httpwf "maestro/internal/http"
	"maestro/internal/ratelimit"
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
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Run test
	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 3, workflow)
	coord.Wait()
	c.Close()

	// Verify results
	if requestCount.Load() == 0 {
		t.Error("expected requests to be made")
	}
	events := c.Events()
	if len(events) == 0 {
		t.Error("expected events to be collected")
	}

	// Most events should be successful (some may fail due to context cancellation)
	successCount := 0
	for _, e := range events {
		if e.Success {
			successCount++
		}
	}
	// At least 50% of events should be successful
	if successCount < len(events)/2 {
		t.Errorf("expected at least half of events to be successful, got %d/%d", successCount, len(events))
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

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 2, workflow)
	coord.Wait()
	c.Close()

	// Each actor should hit all 3 endpoints in order, multiple times
	loginCount := stepCounts["/login"].Load()
	dataCount := stepCounts["/api/data"].Load()
	logoutCount := stepCounts["/logout"].Load()

	if loginCount == 0 || dataCount == 0 || logoutCount == 0 {
		t.Errorf("expected all endpoints to be hit: login=%d, data=%d, logout=%d",
			loginCount, dataCount, logoutCount)
	}

	// Login should be called at least as many times as logout (workflow runs in sequence)
	// Login might be called more times if context cancellation happens mid-workflow
	if loginCount < logoutCount {
		t.Errorf("expected login >= logout counts: login=%d, logout=%d", loginCount, logoutCount)
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

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.LoadProfile == nil {
		t.Fatal("expected load profile to be parsed")
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, cfg.LoadProfile, workflow, nil, nil)
	coord.Wait()
	c.Close()

	// Should have requests from increasing number of actors
	actorIDs := make(map[int]bool)
	for _, e := range c.Events() {
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

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	// Create rate limiter based on config
	rateLimiter := ratelimit.NewRateLimiter(cfg.LoadProfile.Phases[0].RPS)

	workflow := &httpwf.Workflow{
		Config:      cfg.Workflow,
		Client:      &http.Client{Timeout: 5 * time.Second},
		RateLimiter: rateLimiter,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	coord.RunWithProfile(ctx, cfg.LoadProfile, workflow, rateLimiter, nil)
	coord.Wait()
	c.Close()

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

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify no load profile
	if cfg.LoadProfile != nil {
		t.Error("expected nil load profile for backward compatible config")
	}

	// Run classic mode
	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 3, workflow)
	coord.Wait()
	c.Close()

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

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 2, workflow)
	coord.Wait()
	c.Close()

	// Should have a mix of successes and failures
	successCount := 0
	failureCount := 0
	for _, e := range c.Events() {
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
	var mu sync.Mutex
	var receivedContentType string
	var receivedAuth string
	var receivedBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedContentType = r.Header.Get("Content-Type")
		receivedAuth = r.Header.Get("Authorization")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		mu.Unlock()
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

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 1, workflow)
	coord.Wait()
	c.Close()

	mu.Lock()
	ct := receivedContentType
	auth := receivedAuth
	body := receivedBody
	mu.Unlock()

	if ct != "application/json" {
		t.Errorf("expected Content-Type header, got %q", ct)
	}
	if auth != "Bearer secret-token" {
		t.Errorf("expected Authorization header, got %q", auth)
	}
	if body != `{"name": "test", "value": 123}` {
		t.Errorf("expected body, got %q", body)
	}
}

func TestIntegration_ThresholdsPass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Thresholds Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"

thresholds:
  http_req_duration:
    p95: 500ms
  http_req_failed:
    rate: "5%"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 2, workflow)
	coord.Wait()
	c.Close()

	m := collector.ComputeMetrics(c.Events(), c.Duration())
	results := cfg.Thresholds.Check(m)

	if !results.Passed {
		t.Errorf("expected thresholds to pass, got violations: %v", results.Violations())
	}
}

func TestIntegration_ThresholdsFail(t *testing.T) {
	// Server that always returns 500 errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Thresholds Fail Test"
  steps:
    - name: "failing"
      method: GET
      url: "` + server.URL + `"

thresholds:
  http_req_failed:
    rate: "1%"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 2, workflow)
	coord.Wait()
	c.Close()

	m := collector.ComputeMetrics(c.Events(), c.Duration())
	results := cfg.Thresholds.Check(m)

	// Should fail because all requests return 500 (100% error rate > 1% threshold)
	if results.Passed {
		t.Error("expected thresholds to fail, but they passed")
	}

	violations := results.Violations()
	if len(violations) == 0 {
		t.Error("expected at least one violation")
	}

	// Verify the failure rate violation
	foundRateViolation := false
	for _, v := range violations {
		if v.Name == "http_req_failed.rate" {
			foundRateViolation = true
			// v.Actual is a string like "100.00%"
			// All requests failed, so actual rate should be high
			t.Logf("failure rate violation: threshold=%s, actual=%s", v.Threshold, v.Actual)
		}
	}
	if !foundRateViolation {
		t.Error("expected http_req_failed.rate violation")
	}
}

func TestIntegration_ThresholdsDurationFail(t *testing.T) {
	// Server with intentional delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Duration Threshold Fail Test"
  steps:
    - name: "slow"
      method: GET
      url: "` + server.URL + `"

thresholds:
  http_req_duration:
    p95: 10ms
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 1, workflow)
	coord.Wait()
	c.Close()

	m := collector.ComputeMetrics(c.Events(), c.Duration())
	results := cfg.Thresholds.Check(m)

	// Should fail because responses take ~50ms but threshold is 10ms
	if results.Passed {
		t.Errorf("expected thresholds to fail (p95 should be ~50ms, threshold 10ms), got passed. p95=%v", m.Duration.P95)
	}
}

func TestIntegration_MultiPhaseProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Multi-Phase Test"
  steps:
    - name: "api"
      method: GET
      url: "` + server.URL + `"

loadProfile:
  phases:
    - name: "ramp_up"
      duration: 200ms
      startActors: 1
      endActors: 5

    - name: "steady"
      duration: 300ms
      actors: 5
      rps: 50

    - name: "ramp_down"
      duration: 200ms
      startActors: 5
      endActors: 0
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)
	rateLimiter := ratelimit.NewRateLimiter(50)

	workflow := &httpwf.Workflow{
		Config:      cfg.Workflow,
		Client:      &http.Client{Timeout: 5 * time.Second},
		RateLimiter: rateLimiter,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	coord.RunWithProfile(ctx, cfg.LoadProfile, workflow, rateLimiter, nil)
	coord.Wait()
	c.Close()

	m := collector.ComputeMetrics(c.Events(), c.Duration())

	// Should have completed multiple requests across all phases
	if m.TotalRequests < 10 {
		t.Errorf("expected at least 10 requests across all phases, got %d", m.TotalRequests)
	}

	// Success rate should be 100% (server always returns 200)
	if m.SuccessRate < 99.0 {
		t.Errorf("expected ~100%% success rate, got %.2f%%", m.SuccessRate)
	}

	// After ramp_down, should have 0 active actors
	if coord.ActiveActors() != 0 {
		t.Errorf("expected 0 active actors after ramp_down, got %d", coord.ActiveActors())
	}
}

func TestIntegration_EmptyWorkflow(t *testing.T) {
	configContent := `
workflow:
  name: "Empty Workflow"
  steps: []
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 2, workflow)
	coord.Wait()
	c.Close()

	// Should complete without panic, with no events
	m := collector.ComputeMetrics(c.Events(), c.Duration())
	if m.TotalRequests != 0 {
		t.Errorf("expected 0 requests for empty workflow, got %d", m.TotalRequests)
	}
}

func TestIntegration_LargePayload(t *testing.T) {
	var receivedSize atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedSize.Store(int64(len(body)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	// Generate a large payload with repeated characters (not null bytes)
	largeData := strings.Repeat("x", 10000)
	largeBody := `{"data": "` + largeData + `"}`

	configContent := `
workflow:
  name: "Large Payload Test"
  steps:
    - name: "large_post"
      method: POST
      url: "` + server.URL + `"
      headers:
        Content-Type: "application/json"
      body: '` + largeBody + `'
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	coord.Spawn(ctx, 1, workflow)
	coord.Wait()
	c.Close()

	// Should have sent the large payload successfully
	m := collector.ComputeMetrics(c.Events(), c.Duration())
	if m.TotalRequests == 0 {
		t.Error("expected at least one request")
	}
	if m.SuccessRate < 99.0 {
		t.Errorf("expected high success rate, got %.2f%%", m.SuccessRate)
	}
	if receivedSize.Load() < 10000 {
		t.Errorf("expected large body to be received, got %d bytes", receivedSize.Load())
	}
}

func TestIntegration_ConcurrentActors(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Concurrent Test"
  steps:
    - name: "api"
      method: GET
      url: "` + server.URL + `"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Spawn many actors to test concurrency
	coord.Spawn(ctx, 20, workflow)
	coord.Wait()
	c.Close()

	m := collector.ComputeMetrics(c.Events(), c.Duration())

	// With 20 concurrent actors and 10ms per request over 200ms
	// we should get significantly more than 20 requests
	// (approximately 20 * 20 = 400 requests if fully concurrent)
	if m.TotalRequests < 100 {
		t.Errorf("expected significant concurrent throughput, got only %d requests", m.TotalRequests)
	}

	// Verify events came from different actors
	actorIDs := make(map[int]bool)
	for _, e := range c.Events() {
		actorIDs[e.ActorID] = true
	}
	if len(actorIDs) < 15 {
		t.Errorf("expected most of 20 actors to contribute, got %d unique actors", len(actorIDs))
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

// Iteration control tests

func TestIntegration_MaxIterations(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Max Iterations Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	// Use a very long timeout - we expect to exit via max iterations, not timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 3 actors, 5 max iterations each = 15 total requests
	runnerConfig := core.RunnerConfig{
		MaxIterations: 5,
	}
	coord.SpawnWithConfig(ctx, 3, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Verify exactly 15 requests (3 actors * 5 iterations)
	actualCount := requestCount.Load()
	if actualCount != 15 {
		t.Errorf("expected exactly 15 requests (3 actors * 5 iterations), got %d", actualCount)
	}

	events := c.Events()
	if len(events) != 15 {
		t.Errorf("expected 15 events, got %d", len(events))
	}
}

func TestIntegration_WarmupExcludesMetrics(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Warmup Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 2 actors, 5 max iterations, 2 warmup iterations each
	// Total requests: 2 * 5 = 10
	// Reported events: 2 * (5 - 2) = 6
	runnerConfig := core.RunnerConfig{
		MaxIterations: 5,
		WarmupIters:   2,
	}
	coord.SpawnWithConfig(ctx, 2, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Should have made 10 requests total
	actualCount := requestCount.Load()
	if actualCount != 10 {
		t.Errorf("expected 10 total requests, got %d", actualCount)
	}

	// But only 6 events reported (excluding warmup)
	events := c.Events()
	if len(events) != 6 {
		t.Errorf("expected 6 events (excluding warmup), got %d", len(events))
	}
}

func TestIntegration_DeterministicIteration(t *testing.T) {
	// Test that we can run exactly N iterations and verify the state
	var stepCounts sync.Map

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		val, _ := stepCounts.LoadOrStore(path, new(atomic.Int32))
		val.(*atomic.Int32).Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Deterministic Test"
  steps:
    - name: "step1"
      method: GET
      url: "` + server.URL + `/step1"
    - name: "step2"
      method: GET
      url: "` + server.URL + `/step2"
    - name: "step3"
      method: GET
      url: "` + server.URL + `/step3"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1 actor, 4 iterations = 4 complete workflow runs = 12 steps
	runnerConfig := core.RunnerConfig{
		MaxIterations: 4,
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Each step should be called exactly 4 times
	for _, step := range []string{"/step1", "/step2", "/step3"} {
		val, ok := stepCounts.Load(step)
		if !ok {
			t.Errorf("step %s was never called", step)
			continue
		}
		count := val.(*atomic.Int32).Load()
		if count != 4 {
			t.Errorf("expected step %s to be called 4 times, got %d", step, count)
		}
	}

	// Total events should be 12 (4 iterations * 3 steps)
	events := c.Events()
	if len(events) != 12 {
		t.Errorf("expected 12 events, got %d", len(events))
	}
}

func TestIntegration_ConfigFileExecution(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test that execution config in YAML works
	configContent := `
workflow:
  name: "Config File Execution Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"

execution:
  max_iterations: 3
  warmup_iterations: 1
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify config was parsed correctly
	if cfg.Execution.MaxIterations != 3 {
		t.Errorf("expected max_iterations=3, got %d", cfg.Execution.MaxIterations)
	}
	if cfg.Execution.WarmupIterations != 1 {
		t.Errorf("expected warmup_iterations=1, got %d", cfg.Execution.WarmupIterations)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: cfg.Execution.MaxIterations,
		WarmupIters:   cfg.Execution.WarmupIterations,
	}
	coord.SpawnWithConfig(ctx, 2, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// 2 actors * 3 iterations = 6 requests
	actualCount := requestCount.Load()
	if actualCount != 6 {
		t.Errorf("expected 6 requests, got %d", actualCount)
	}

	// 2 actors * (3 - 1) iterations = 4 reported events
	events := c.Events()
	if len(events) != 4 {
		t.Errorf("expected 4 events (excluding warmup), got %d", len(events))
	}
}

func TestIntegration_MaxIterationsStopsBeforeTimeout(t *testing.T) {
	// Verify that max iterations causes exit before context timeout
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Early Exit Test"
  steps:
    - name: "health"
      method: GET
      url: "` + server.URL + `"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	// Very long timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	start := time.Now()

	// Only 2 iterations should complete quickly
	runnerConfig := core.RunnerConfig{
		MaxIterations: 2,
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	elapsed := time.Since(start)

	// Should complete in well under 1 second (not 5 minutes)
	if elapsed > 5*time.Second {
		t.Errorf("expected quick exit via max iterations, took %v", elapsed)
	}

	if requestCount.Load() != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount.Load())
	}
}

func TestIntegration_VariableExtraction(t *testing.T) {
	var mu sync.Mutex
	var receivedBody string

	// Server that returns JSON with an ID and echoes the POST body
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id": "test-123", "user": {"name": "alice"}}`))
			return
		}
		if r.URL.Path == "/echo" {
			mu.Lock()
			body, _ := io.ReadAll(r.Body)
			receivedBody = string(body)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Config that extracts from first request and uses in second
	configContent := `
workflow:
  name: "Variable Extraction Test"
  steps:
    - name: "get_json"
      method: GET
      url: "` + server.URL + `/json"
      extract:
        request_id: "$.id"
        user_name: "$.user.name"
    - name: "echo_id"
      method: POST
      url: "` + server.URL + `/echo"
      headers:
        Content-Type: "application/json"
      body: '{"extracted_id": "${request_id}", "name": "${user_name}"}'
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 1, // Run once
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Verify extraction worked - the POST body should contain the extracted values
	mu.Lock()
	body := receivedBody
	mu.Unlock()

	if !strings.Contains(body, `"extracted_id": "test-123"`) {
		t.Errorf("expected body to contain extracted_id, got: %s", body)
	}
	if !strings.Contains(body, `"name": "alice"`) {
		t.Errorf("expected body to contain name, got: %s", body)
	}

	// Verify both steps were successful
	events := c.Events()
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
	for _, e := range events {
		if !e.Success {
			t.Errorf("expected step %q to succeed, got error: %s", e.Step, e.Error)
		}
	}
}

func TestIntegration_VariableSubstitution_InURL(t *testing.T) {
	var mu sync.Mutex
	var receivedPaths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPaths = append(receivedPaths, r.URL.Path)
		mu.Unlock()

		if r.URL.Path == "/users" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"user_id": 42}`))
			return
		}
		// Return success for /users/42
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "URL Substitution Test"
  steps:
    - name: "get_user_id"
      method: GET
      url: "` + server.URL + `/users"
      extract:
        user_id: "$.user_id"
    - name: "get_user_profile"
      method: GET
      url: "` + server.URL + `/users/${user_id}"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 1,
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	mu.Lock()
	paths := receivedPaths
	mu.Unlock()

	// Should have hit /users and then /users/42
	if len(paths) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(paths))
	}
	if paths[0] != "/users" {
		t.Errorf("expected first path /users, got %s", paths[0])
	}
	if paths[1] != "/users/42" {
		t.Errorf("expected second path /users/42, got %s", paths[1])
	}
}

func TestIntegration_VariableSubstitution_InHeaders(t *testing.T) {
	var mu sync.Mutex
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"auth": {"token": "secret-bearer-token"}}`))
			return
		}
		if r.URL.Path == "/protected" {
			mu.Lock()
			receivedAuth = r.Header.Get("Authorization")
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Header Substitution Test"
  steps:
    - name: "login"
      method: POST
      url: "` + server.URL + `/login"
      extract:
        token: "$.auth.token"
    - name: "access_protected"
      method: GET
      url: "` + server.URL + `/protected"
      headers:
        Authorization: "Bearer ${token}"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 1,
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	mu.Lock()
	auth := receivedAuth
	mu.Unlock()

	if auth != "Bearer secret-bearer-token" {
		t.Errorf("expected Authorization header 'Bearer secret-bearer-token', got %q", auth)
	}
}

func TestIntegration_VariableSubstitution_EnvironmentVariable(t *testing.T) {
	os.Setenv("TEST_API_PATH", "/api/test")
	defer os.Unsetenv("TEST_API_PATH")

	var mu sync.Mutex
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPath = r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Env Variable Test"
  steps:
    - name: "api_call"
      method: GET
      url: "` + server.URL + `${env:TEST_API_PATH}"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 1,
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	mu.Lock()
	path := receivedPath
	mu.Unlock()

	if path != "/api/test" {
		t.Errorf("expected path /api/test from env var, got %s", path)
	}
}

func TestIntegration_VariableSubstitution_MissingVariable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Missing Variable Test"
  steps:
    - name: "use_missing"
      method: GET
      url: "` + server.URL + `/users/${nonexistent_id}"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 1,
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Should have failed due to missing variable
	events := c.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Success {
		t.Error("expected step to fail due to missing variable")
	}
	if !strings.Contains(events[0].Error, "nonexistent_id") {
		t.Errorf("expected error to mention missing variable, got: %s", events[0].Error)
	}
}

func TestIntegration_VariableExtraction_PathNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"different": "structure"}`))
	}))
	defer server.Close()

	configContent := `
workflow:
  name: "Extraction Path Not Found Test"
  steps:
    - name: "extract_missing"
      method: GET
      url: "` + server.URL + `"
      extract:
        missing_field: "$.nonexistent.path"
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config: cfg.Workflow,
		Client: &http.Client{Timeout: 5 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 1,
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Should have failed due to extraction path not found
	events := c.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Success {
		t.Error("expected step to fail due to extraction path not found")
	}
	if !strings.Contains(events[0].Error, "not found") {
		t.Errorf("expected error to mention 'not found', got: %s", events[0].Error)
	}
}

func TestIntegration_DataFiles_CSV(t *testing.T) {
	var receivedUsernames []string
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		// Extract username from JSON body
		var data map[string]string
		json.Unmarshal(body, &data)
		if u, ok := data["username"]; ok {
			receivedUsernames = append(receivedUsernames, u)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create temp CSV file
	csvContent := `username,password
alice,secret1
bob,secret2
charlie,secret3`
	csvPath := filepath.Join(t.TempDir(), "users.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatal(err)
	}

	configContent := `
workflow:
  name: "Data Files CSV Test"
  data:
    users:
      file: "` + csvPath + `"
      mode: sequential
  steps:
    - name: "login"
      method: POST
      url: "` + server.URL + `"
      headers:
        Content-Type: "application/json"
      body: '{"username": "${data.users.username}", "password": "${data.users.password}"}'
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Load data sources
	dataSources := make(data.Sources)
	for name, dsCfg := range cfg.Workflow.Data {
		src, err := data.LoadFile(name, dsCfg.File, data.Mode(dsCfg.Mode), "")
		if err != nil {
			t.Fatalf("failed to load data source %q: %v", name, err)
		}
		dataSources[name] = src
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config:      cfg.Workflow,
		Client:      &http.Client{Timeout: 5 * time.Second},
		DataSources: dataSources,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 4, // More than 3 rows to test wrap-around
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Should have received 4 usernames in sequential order with wrap-around
	expected := []string{"alice", "bob", "charlie", "alice"}
	if len(receivedUsernames) != 4 {
		t.Fatalf("expected 4 usernames, got %d: %v", len(receivedUsernames), receivedUsernames)
	}
	for i, want := range expected {
		if receivedUsernames[i] != want {
			t.Errorf("username[%d] = %q, want %q", i, receivedUsernames[i], want)
		}
	}

	m := collector.ComputeMetrics(c.Events(), c.Duration())
	if m.SuccessRate < 100 {
		t.Errorf("expected 100%% success rate, got %.2f%%", m.SuccessRate)
	}
}

func TestIntegration_DataFiles_JSON_Random(t *testing.T) {
	var receivedIDs []float64
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		var data map[string]any
		json.Unmarshal(body, &data)
		if id, ok := data["product_id"].(float64); ok {
			receivedIDs = append(receivedIDs, id)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create temp JSON file
	jsonContent := `[
		{"id": 1, "name": "Widget"},
		{"id": 2, "name": "Gadget"},
		{"id": 3, "name": "Gizmo"}
	]`
	jsonPath := filepath.Join(t.TempDir(), "products.json")
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	configContent := `
workflow:
  name: "Data Files JSON Random Test"
  data:
    products:
      file: "` + jsonPath + `"
      mode: random
  steps:
    - name: "get_product"
      method: POST
      url: "` + server.URL + `"
      headers:
        Content-Type: "application/json"
      body: '{"product_id": ${data.products.id}, "product_name": "${data.products.name}"}'
`
	configPath := createTempConfigFile(t, configContent)
	defer os.Remove(configPath)

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Load data sources
	dataSources := make(data.Sources)
	for name, dsCfg := range cfg.Workflow.Data {
		src, err := data.LoadFile(name, dsCfg.File, data.Mode(dsCfg.Mode), "")
		if err != nil {
			t.Fatalf("failed to load data source %q: %v", name, err)
		}
		dataSources[name] = src
	}

	c := collector.NewCollector()
	coord := coordinator.NewCoordinator(c)

	workflow := &httpwf.Workflow{
		Config:      cfg.Workflow,
		Client:      &http.Client{Timeout: 5 * time.Second},
		DataSources: dataSources,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runnerConfig := core.RunnerConfig{
		MaxIterations: 50, // Enough iterations to see randomness
	}
	coord.SpawnWithConfig(ctx, 1, workflow, runnerConfig)
	coord.Wait()
	c.Close()

	// Should have received 50 product IDs, all valid (1, 2, or 3)
	if len(receivedIDs) != 50 {
		t.Fatalf("expected 50 IDs, got %d", len(receivedIDs))
	}

	// Check all IDs are valid and we see multiple different values (randomness)
	seen := make(map[float64]bool)
	for _, id := range receivedIDs {
		if id < 1 || id > 3 {
			t.Errorf("invalid product ID: %v", id)
		}
		seen[id] = true
	}

	// With 50 iterations and 3 products, we should see at least 2 different products
	if len(seen) < 2 {
		t.Errorf("expected random mode to pick multiple products, only saw %d different IDs", len(seen))
	}
}
