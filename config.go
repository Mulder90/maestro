package burstsmith

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure.
type Config struct {
	Workflow    WorkflowConfig `yaml:"workflow"`
	LoadProfile *LoadProfile   `yaml:"loadProfile,omitempty"`
	Thresholds  *Thresholds    `yaml:"thresholds,omitempty"`
}

// LoadProfile defines the load pattern for a test.
type LoadProfile struct {
	Phases []Phase `yaml:"phases"`
}

// Phase represents a single phase in the load profile.
type Phase struct {
	Name        string        `yaml:"name"`
	Duration    time.Duration `yaml:"duration"`
	Actors      int           `yaml:"actors"`      // for steady state
	StartActors int           `yaml:"startActors"` // for ramp phases
	EndActors   int           `yaml:"endActors"`   // for ramp phases
	RPS         int           `yaml:"rps"`         // rate limit (requests per second)
}

// TotalDuration returns the sum of all phase durations.
func (lp *LoadProfile) TotalDuration() time.Duration {
	var total time.Duration
	for _, p := range lp.Phases {
		total += p.Duration
	}
	return total
}

// WorkflowConfig defines a named workflow with a sequence of steps.
type WorkflowConfig struct {
	Name  string       `yaml:"name"`
	Steps []StepConfig `yaml:"steps"`
}

// StepConfig defines a single HTTP request step.
type StepConfig struct {
	Name    string            `yaml:"name"`
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
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
