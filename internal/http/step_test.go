package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"burstsmith/internal/config"
	"burstsmith/internal/core"
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
