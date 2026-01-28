// Package testserver provides a configurable HTTP server for load testing.
package testserver

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Server is a configurable HTTP test server.
type Server struct {
	mux       *http.ServeMux
	requestID atomic.Int64
}

// NewServer creates a new test server with all endpoints configured.
func NewServer() *Server {
	s := &Server{
		mux: http.NewServeMux(),
	}
	s.registerHandlers()
	return s
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// registerHandlers sets up all the test endpoints.
func (s *Server) registerHandlers() {
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/status/", s.handleStatus)
	s.mux.HandleFunc("/delay/", s.handleDelay)
	s.mux.HandleFunc("/echo", s.handleEcho)
	s.mux.HandleFunc("/random-delay", s.handleRandomDelay)
	s.mux.HandleFunc("/fail-rate", s.handleFailRate)
	s.mux.HandleFunc("/json", s.handleJSON)
	s.mux.HandleFunc("/headers", s.handleHeaders)
	s.mux.HandleFunc("/auth/login", s.handleLogin)
	s.mux.HandleFunc("/users/", s.handleUsers)
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

// handleStatus returns the specified HTTP status code.
// Example: GET /status/404 returns 404 Not Found
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	// Extract status code from path: /status/{code}
	path := strings.TrimPrefix(r.URL.Path, "/status/")
	code, err := strconv.Atoi(path)
	if err != nil || code < 100 || code > 599 {
		http.Error(w, "invalid status code", http.StatusBadRequest)
		return
	}
	w.WriteHeader(code)
	fmt.Fprintf(w, "%d %s", code, http.StatusText(code))
}

// handleDelay waits for the specified duration before responding.
// Example: GET /delay/100 waits 100ms
func (s *Server) handleDelay(w http.ResponseWriter, r *http.Request) {
	// Extract delay from path: /delay/{ms}
	path := strings.TrimPrefix(r.URL.Path, "/delay/")
	ms, err := strconv.Atoi(path)
	if err != nil || ms < 0 {
		http.Error(w, "invalid delay", http.StatusBadRequest)
		return
	}

	time.Sleep(time.Duration(ms) * time.Millisecond)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "delayed %dms", ms)
}

// handleEcho echoes back the request body with the same content type.
func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// handleRandomDelay waits for a random duration within the specified range.
// Example: GET /random-delay?min=50&max=200 waits 50-200ms
func (s *Server) handleRandomDelay(w http.ResponseWriter, r *http.Request) {
	minStr := r.URL.Query().Get("min")
	maxStr := r.URL.Query().Get("max")

	minMs, err := strconv.Atoi(minStr)
	if err != nil || minMs < 0 {
		minMs = 0
	}

	maxMs, err := strconv.Atoi(maxStr)
	if err != nil || maxMs < minMs {
		maxMs = minMs + 100
	}

	delay := minMs
	if maxMs > minMs {
		delay = minMs + rand.Intn(maxMs-minMs)
	}

	time.Sleep(time.Duration(delay) * time.Millisecond)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "delayed %dms (range: %d-%d)", delay, minMs, maxMs)
}

// handleFailRate fails a percentage of requests with 500 status.
// Example: GET /fail-rate?rate=10 fails 10% of requests
func (s *Server) handleFailRate(w http.ResponseWriter, r *http.Request) {
	rateStr := r.URL.Query().Get("rate")
	rate, err := strconv.Atoi(rateStr)
	if err != nil || rate < 0 || rate > 100 {
		rate = 0
	}

	if rand.Intn(100) < rate {
		http.Error(w, "simulated failure", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "success")
}

// handleJSON returns a JSON response with common test fields.
func (s *Server) handleJSON(w http.ResponseWriter, r *http.Request) {
	id := s.requestID.Add(1)

	response := map[string]interface{}{
		"id":        id,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"method":    r.Method,
		"path":      r.URL.Path,
		"message":   "Hello from test server",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleHeaders returns the request headers as JSON.
func (s *Server) handleHeaders(w http.ResponseWriter, r *http.Request) {
	headers := make(map[string]string)
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	response := map[string]interface{}{
		"headers": headers,
		"method":  r.Method,
		"path":    r.URL.Path,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleLogin simulates an authentication endpoint.
// Returns a token that can be used in subsequent requests.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := s.requestID.Add(1)
	token := fmt.Sprintf("token-%d-%d", id, time.Now().UnixNano())

	response := map[string]interface{}{
		"auth": map[string]interface{}{
			"token":      token,
			"expires_in": 3600,
		},
		"user": map[string]interface{}{
			"id":   id,
			"name": "testuser",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleUsers returns user data. Expects Authorization header.
// Example: GET /users/123 or GET /users/me
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")

	path := strings.TrimPrefix(r.URL.Path, "/users/")
	userID := path
	if userID == "" {
		userID = "unknown"
	}

	response := map[string]interface{}{
		"user_id":       userID,
		"name":          "Test User",
		"email":         "test@example.com",
		"authenticated": auth != "",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
