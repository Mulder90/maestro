// Package config handles YAML configuration parsing.
package config

import (
	"fmt"
	"os"
	"time"

	"maestro/internal/collector"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure.
type Config struct {
	Workflow    WorkflowConfig        `yaml:"workflow"`
	LoadProfile *LoadProfile          `yaml:"loadProfile,omitempty"`
	Thresholds  *collector.Thresholds `yaml:"thresholds,omitempty"`
	Execution   ExecutionConfig       `yaml:"execution,omitempty"`
}

// ExecutionConfig controls iteration-level execution behavior.
type ExecutionConfig struct {
	MaxIterations    int `yaml:"max_iterations"`
	WarmupIterations int `yaml:"warmup_iterations"`
}

// LoadProfile defines the load pattern for a test.
type LoadProfile struct {
	Phases []Phase `yaml:"phases"`
}

// TotalDuration returns the sum of all phase durations.
func (lp *LoadProfile) TotalDuration() time.Duration {
	var total time.Duration
	for _, p := range lp.Phases {
		total += p.Duration
	}
	return total
}

// Phase represents a single phase in the load profile.
type Phase struct {
	Name        string        `yaml:"name"`
	Duration    time.Duration `yaml:"duration"`
	Actors      int           `yaml:"actors"`
	StartActors int           `yaml:"startActors"`
	EndActors   int           `yaml:"endActors"`
	RPS         int           `yaml:"rps"`
}

// WorkflowConfig defines a named workflow with a sequence of steps.
type WorkflowConfig struct {
	Name  string       `yaml:"name"`
	Steps []StepConfig `yaml:"steps"`
}

// StepConfig defines a single request step.
type StepConfig struct {
	Name    string            `yaml:"name"`
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
	Extract map[string]string `yaml:"extract,omitempty"` // JSONPath extraction rules
}

// LoadConfig reads and parses a YAML configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}
