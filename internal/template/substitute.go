// Package template provides variable substitution and extraction for workflows.
// It is protocol-agnostic and can be reused with HTTP, gRPC, WebSocket, etc.
package template

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"maestro/internal/core"
)

// varPattern matches ${var} and ${env:VAR} placeholders.
var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Substitute replaces ${var} and ${env:VAR} placeholders in text.
// Returns all errors joined if multiple variables are missing.
// If text contains no placeholders, it is returned unchanged (fast path).
func Substitute(text string, vars core.Variables) (string, error) {
	// Fast path: no variables to substitute
	if !strings.Contains(text, "${") {
		return text, nil
	}

	var errs []error
	result := varPattern.ReplaceAllStringFunc(text, func(match string) string {
		name := match[2 : len(match)-1] // Extract content between ${ and }

		// Handle environment variables
		if strings.HasPrefix(name, "env:") {
			envName := name[4:]
			if val, ok := os.LookupEnv(envName); ok {
				return val
			}
			errs = append(errs, fmt.Errorf("env var %q not set", envName))
			return match
		}

		// Handle workflow variables
		if val, ok := vars.Get(name); ok {
			return fmt.Sprintf("%v", val)
		}
		errs = append(errs, fmt.Errorf("variable %q not found", name))
		return match
	})

	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	return result, nil
}

// SubstituteMap applies substitution to all values in a map.
// Returns all errors joined if any substitution fails.
func SubstituteMap(m map[string]string, vars core.Variables) (map[string]string, error) {
	if m == nil {
		return nil, nil
	}

	result := make(map[string]string, len(m))
	var errs []error

	for k, v := range m {
		substituted, err := Substitute(v, vars)
		if err != nil {
			errs = append(errs, fmt.Errorf("header %q: %w", k, err))
			continue
		}
		result[k] = substituted
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return result, nil
}
