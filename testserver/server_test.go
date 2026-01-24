package testserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	tests := []struct {
		code     int
		expected int
	}{
		{200, 200},
		{201, 201},
		{400, 400},
		{404, 404},
		{500, 500},
		{503, 503},
	}

	for _, tt := range tests {
		resp, err := http.Get(ts.URL + "/status/" + itoa(tt.code))
		if err != nil {
			t.Fatalf("GET /status/%d failed: %v", tt.code, err)
		}
		resp.Body.Close()

		if resp.StatusCode != tt.expected {
			t.Errorf("GET /status/%d: expected %d, got %d", tt.code, tt.expected, resp.StatusCode)
		}
	}
}

func TestDelayEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	start := time.Now()
	resp, err := http.Get(ts.URL + "/delay/100")
	if err != nil {
		t.Fatalf("GET /delay/100 failed: %v", err)
	}
	resp.Body.Close()
	elapsed := time.Since(start)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Should take at least 100ms
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected delay of at least 100ms, got %v", elapsed)
	}

	// But not too long (allow some tolerance)
	if elapsed > 200*time.Millisecond {
		t.Errorf("delay took too long: %v", elapsed)
	}
}

func TestEchoEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	body := `{"message": "hello", "count": 42}`
	resp, err := http.Post(ts.URL+"/echo", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /echo failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if string(respBody) != body {
		t.Errorf("expected body %q, got %q", body, string(respBody))
	}

	// Should echo content-type header
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", resp.Header.Get("Content-Type"))
	}
}

func TestRandomDelayEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	start := time.Now()
	resp, err := http.Get(ts.URL + "/random-delay?min=50&max=100")
	if err != nil {
		t.Fatalf("GET /random-delay failed: %v", err)
	}
	resp.Body.Close()
	elapsed := time.Since(start)

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// Should be within range (with some tolerance)
	if elapsed < 50*time.Millisecond {
		t.Errorf("delay too short: %v (min=50ms)", elapsed)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("delay too long: %v (max=100ms)", elapsed)
	}
}

func TestFailRateEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// With 100% fail rate, all requests should fail
	successes := 0
	failures := 0
	for i := 0; i < 20; i++ {
		resp, err := http.Get(ts.URL + "/fail-rate?rate=100")
		if err != nil {
			t.Fatalf("GET /fail-rate failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			successes++
		} else if resp.StatusCode == 500 {
			failures++
		}
	}

	if successes > 0 {
		t.Errorf("with 100%% fail rate, expected all failures, got %d successes", successes)
	}

	// With 0% fail rate, all requests should succeed
	successes = 0
	for i := 0; i < 20; i++ {
		resp, err := http.Get(ts.URL + "/fail-rate?rate=0")
		if err != nil {
			t.Fatalf("GET /fail-rate failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			successes++
		}
	}

	if successes != 20 {
		t.Errorf("with 0%% fail rate, expected all successes, got %d", successes)
	}
}

func TestJSONEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json")
	if err != nil {
		t.Fatalf("GET /json failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Errorf("failed to decode JSON response: %v", err)
	}

	// Should have some standard fields
	if _, ok := data["timestamp"]; !ok {
		t.Error("expected 'timestamp' field in JSON response")
	}
	if _, ok := data["id"]; !ok {
		t.Error("expected 'id' field in JSON response")
	}
}

func TestHealthEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("expected body to contain 'ok', got %q", string(body))
	}
}

func TestHeadersEndpoint(t *testing.T) {
	server := NewServer()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/headers", nil)
	req.Header.Set("X-Custom-Header", "test-value")
	req.Header.Set("Authorization", "Bearer token123")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /headers failed: %v", err)
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Errorf("failed to decode JSON response: %v", err)
	}

	headers, ok := data["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'headers' object in response")
	}

	if headers["X-Custom-Header"] != "test-value" {
		t.Errorf("expected X-Custom-Header to be 'test-value', got %v", headers["X-Custom-Header"])
	}
}

// Helper to convert int to string without importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
