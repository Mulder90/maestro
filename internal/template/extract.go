package template

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
)

// Extract extracts values from JSON using JSONPath expressions.
// Paths use JSONPath syntax ($.foo.bar) which is converted to gjson format.
// Array access: $.items[0].id -> items.0.id
// Returns all errors joined if multiple extractions fail.
func Extract(body []byte, rules map[string]string) (map[string]any, error) {
	if len(rules) == 0 {
		return nil, nil
	}

	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid JSON in response body")
	}

	result := make(map[string]any, len(rules))
	var errs []error

	for varName, jsonPath := range rules {
		path := convertJSONPath(jsonPath)
		value := gjson.GetBytes(body, path)

		if !value.Exists() {
			errs = append(errs, fmt.Errorf("path %q not found for variable %q", jsonPath, varName))
			continue
		}

		result[varName] = value.Value()
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return result, nil
}

// convertJSONPath converts JSONPath syntax to gjson path format.
// $.foo.bar -> foo.bar
// $.items[0].id -> items.0.id
// $.data[*].name -> data.#.name
func convertJSONPath(path string) string {
	// Remove leading $. or $
	if strings.HasPrefix(path, "$.") {
		path = path[2:]
	} else if strings.HasPrefix(path, "$") {
		path = path[1:]
	}

	// Convert array access [n] to .n
	// Convert [*] to .#
	var result strings.Builder
	i := 0
	for i < len(path) {
		if path[i] == '[' {
			// Find closing bracket
			j := i + 1
			for j < len(path) && path[j] != ']' {
				j++
			}
			if j < len(path) {
				content := path[i+1 : j]
				if content == "*" {
					result.WriteString(".#")
				} else {
					result.WriteByte('.')
					result.WriteString(content)
				}
				i = j + 1
				continue
			}
		}
		result.WriteByte(path[i])
		i++
	}

	return result.String()
}
