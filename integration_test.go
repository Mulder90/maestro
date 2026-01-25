package maestro_test

import (
	"context"
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

	m := c.Compute()
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

	m := c.Compute()
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

	m := c.Compute()
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

	m := c.Compute()

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
	m := c.Compute()
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
	m := c.Compute()
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

	m := c.Compute()

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
