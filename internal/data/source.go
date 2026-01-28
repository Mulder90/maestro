// Package data provides data file loading and iteration for parameterized tests.
package data

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Mode defines how data rows are selected during iteration.
type Mode string

const (
	// ModeSequential iterates through rows in order, wrapping around.
	ModeSequential Mode = "sequential"
	// ModeRandom selects a random row for each iteration.
	ModeRandom Mode = "random"
)

// Source represents a loaded data file with iteration support.
type Source struct {
	name    string
	rows    []map[string]any
	mode    Mode
	counter atomic.Uint64
	mu      sync.Mutex
	rng     *rand.Rand
}

// NewSource creates a data source from loaded rows.
func NewSource(name string, rows []map[string]any, mode Mode) *Source {
	if mode == "" {
		mode = ModeSequential
	}
	return &Source{
		name: name,
		rows: rows,
		mode: mode,
		rng:  rand.New(rand.NewSource(rand.Int63())),
	}
}

// Name returns the source name.
func (s *Source) Name() string {
	return s.name
}

// Len returns the number of rows.
func (s *Source) Len() int {
	return len(s.rows)
}

// Next returns the next row based on the iteration mode.
// Thread-safe for concurrent access by multiple actors.
func (s *Source) Next() map[string]any {
	if len(s.rows) == 0 {
		return nil
	}

	var idx int
	switch s.mode {
	case ModeRandom:
		s.mu.Lock()
		idx = s.rng.Intn(len(s.rows))
		s.mu.Unlock()
	default: // ModeSequential
		n := s.counter.Add(1) - 1
		idx = int(n % uint64(len(s.rows)))
	}

	return s.rows[idx]
}

// LoadFile loads a data file (CSV or JSON) and returns a Source.
func LoadFile(name, path string, mode Mode, configDir string) (*Source, error) {
	// Resolve relative paths against config file directory
	if !filepath.IsAbs(path) {
		path = filepath.Join(configDir, path)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var rows []map[string]any
	var err error

	switch ext {
	case ".csv":
		rows, err = loadCSV(path)
	case ".json":
		rows, err = loadJSON(path)
	default:
		return nil, fmt.Errorf("unsupported file format %q (use .csv or .json)", ext)
	}

	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("data file %s is empty", path)
	}

	return NewSource(name, rows, mode), nil
}

// loadCSV loads a CSV file. First row is headers, subsequent rows are data.
func loadCSV(path string) ([]map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV must have header row and at least one data row")
	}

	headers := records[0]
	rows := make([]map[string]any, 0, len(records)-1)

	for _, record := range records[1:] {
		row := make(map[string]any, len(headers))
		for i, header := range headers {
			if i < len(record) {
				row[header] = record[i]
			} else {
				row[header] = ""
			}
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// loadJSON loads a JSON file. Must be an array of objects.
func loadJSON(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("JSON must be an array of objects: %w", err)
	}

	return rows, nil
}

// Sources is a collection of named data sources.
type Sources map[string]*Source

// InjectVariables adds data from all sources to the variables map.
// Each source's fields are accessible as "data.sourcename.fieldname".
func (s Sources) InjectVariables(vars interface {
	Set(key string, value any)
}) {
	for name, source := range s {
		row := source.Next()
		for field, value := range row {
			key := fmt.Sprintf("data.%s.%s", name, field)
			vars.Set(key, value)
		}
	}
}
