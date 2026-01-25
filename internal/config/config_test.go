package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_ValidWorkflow(t *testing.T) {
	content := `
workflow:
  name: "Test Workflow"
  steps:
    - name: "step1"
      method: GET
      url: "https://example.com/api"
`
	cfg := loadConfigFromString(t, content)

	if cfg.Workflow.Name != "Test Workflow" {
		t.Errorf("expected workflow name 'Test Workflow', got %q", cfg.Workflow.Name)
	}
	if len(cfg.Workflow.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(cfg.Workflow.Steps))
	}
	if cfg.Workflow.Steps[0].Name != "step1" {
		t.Errorf("expected step name 'step1', got %q", cfg.Workflow.Steps[0].Name)
	}
	if cfg.Workflow.Steps[0].Method != "GET" {
		t.Errorf("expected method GET, got %q", cfg.Workflow.Steps[0].Method)
	}
	if cfg.Workflow.Steps[0].URL != "https://example.com/api" {
		t.Errorf("expected URL 'https://example.com/api', got %q", cfg.Workflow.Steps[0].URL)
	}
}

func TestLoadConfig_WithHeaders(t *testing.T) {
	content := `
workflow:
  name: "With Headers"
  steps:
    - name: "auth"
      method: POST
      url: "https://example.com/login"
      headers:
        Content-Type: "application/json"
        Authorization: "Bearer token123"
      body: '{"user": "test"}'
`
	cfg := loadConfigFromString(t, content)

	step := cfg.Workflow.Steps[0]
	if step.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type header, got %v", step.Headers)
	}
	if step.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("expected Authorization header, got %v", step.Headers)
	}
	if step.Body != `{"user": "test"}` {
		t.Errorf("expected body, got %q", step.Body)
	}
}

func TestLoadConfig_MultipleSteps(t *testing.T) {
	content := `
workflow:
  name: "Multi-Step"
  steps:
    - name: "get"
      method: GET
      url: "https://example.com/get"
    - name: "post"
      method: POST
      url: "https://example.com/post"
    - name: "delete"
      method: DELETE
      url: "https://example.com/delete"
`
	cfg := loadConfigFromString(t, content)

	if len(cfg.Workflow.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(cfg.Workflow.Steps))
	}

	expected := []struct {
		name   string
		method string
	}{
		{"get", "GET"},
		{"post", "POST"},
		{"delete", "DELETE"},
	}

	for i, exp := range expected {
		if cfg.Workflow.Steps[i].Name != exp.name {
			t.Errorf("step %d: expected name %q, got %q", i, exp.name, cfg.Workflow.Steps[i].Name)
		}
		if cfg.Workflow.Steps[i].Method != exp.method {
			t.Errorf("step %d: expected method %q, got %q", i, exp.method, cfg.Workflow.Steps[i].Method)
		}
	}
}

func TestLoadConfig_WithLoadProfile(t *testing.T) {
	content := `
workflow:
  name: "With Profile"
  steps:
    - name: "health"
      method: GET
      url: "https://example.com/health"

loadProfile:
  phases:
    - name: "ramp_up"
      duration: 30s
      startActors: 1
      endActors: 50
    - name: "steady"
      duration: 2m
      actors: 50
      rps: 100
    - name: "ramp_down"
      duration: 15s
      startActors: 50
      endActors: 0
`
	cfg := loadConfigFromString(t, content)

	if cfg.LoadProfile == nil {
		t.Fatal("expected loadProfile to be set")
	}
	if len(cfg.LoadProfile.Phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(cfg.LoadProfile.Phases))
	}

	// Check ramp_up phase
	phase := cfg.LoadProfile.Phases[0]
	if phase.Name != "ramp_up" {
		t.Errorf("expected phase name 'ramp_up', got %q", phase.Name)
	}
	if phase.Duration != 30*time.Second {
		t.Errorf("expected duration 30s, got %v", phase.Duration)
	}
	if phase.StartActors != 1 {
		t.Errorf("expected startActors 1, got %d", phase.StartActors)
	}
	if phase.EndActors != 50 {
		t.Errorf("expected endActors 50, got %d", phase.EndActors)
	}

	// Check steady phase
	phase = cfg.LoadProfile.Phases[1]
	if phase.Name != "steady" {
		t.Errorf("expected phase name 'steady', got %q", phase.Name)
	}
	if phase.Duration != 2*time.Minute {
		t.Errorf("expected duration 2m, got %v", phase.Duration)
	}
	if phase.Actors != 50 {
		t.Errorf("expected actors 50, got %d", phase.Actors)
	}
	if phase.RPS != 100 {
		t.Errorf("expected rps 100, got %d", phase.RPS)
	}

	// Check ramp_down phase
	phase = cfg.LoadProfile.Phases[2]
	if phase.Name != "ramp_down" {
		t.Errorf("expected phase name 'ramp_down', got %q", phase.Name)
	}
	if phase.StartActors != 50 {
		t.Errorf("expected startActors 50, got %d", phase.StartActors)
	}
	if phase.EndActors != 0 {
		t.Errorf("expected endActors 0, got %d", phase.EndActors)
	}
}

func TestLoadConfig_NoLoadProfile(t *testing.T) {
	content := `
workflow:
  name: "No Profile"
  steps:
    - name: "health"
      method: GET
      url: "https://example.com/health"
`
	cfg := loadConfigFromString(t, content)

	if cfg.LoadProfile != nil {
		t.Error("expected loadProfile to be nil")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	content := `
workflow:
  name: "Invalid
  steps: [[[invalid
`
	tmpFile := createTempFile(t, content)
	defer os.Remove(tmpFile)

	_, err := LoadConfig(tmpFile)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	tmpFile := createTempFile(t, "")
	defer os.Remove(tmpFile)

	cfg, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Workflow.Name != "" {
		t.Errorf("expected empty workflow name, got %q", cfg.Workflow.Name)
	}
}

func TestLoadProfile_TotalDuration_Empty(t *testing.T) {
	lp := &LoadProfile{Phases: []Phase{}}
	if lp.TotalDuration() != 0 {
		t.Errorf("expected 0 duration, got %v", lp.TotalDuration())
	}
}

func TestLoadProfile_TotalDuration_Single(t *testing.T) {
	lp := &LoadProfile{
		Phases: []Phase{
			{Duration: 30 * time.Second},
		},
	}
	if lp.TotalDuration() != 30*time.Second {
		t.Errorf("expected 30s, got %v", lp.TotalDuration())
	}
}

func TestLoadProfile_TotalDuration_Multiple(t *testing.T) {
	lp := &LoadProfile{
		Phases: []Phase{
			{Duration: 10 * time.Second},
			{Duration: 20 * time.Second},
			{Duration: 5 * time.Second},
		},
	}

	expected := 35 * time.Second
	if lp.TotalDuration() != expected {
		t.Errorf("expected %v, got %v", expected, lp.TotalDuration())
	}
}

// Helper functions

func loadConfigFromString(t *testing.T, content string) *Config {
	t.Helper()
	tmpFile := createTempFile(t, content)
	defer os.Remove(tmpFile)

	cfg, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	return cfg
}

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	return tmpFile
}
