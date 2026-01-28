package template

import (
	"strings"
	"testing"
)

func TestExtract_SimpleField(t *testing.T) {
	body := []byte(`{"name": "test", "id": 123}`)
	rules := map[string]string{
		"name": "$.name",
	}

	result, err := Extract(body, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["name"] != "test" {
		t.Errorf("expected 'test', got %v", result["name"])
	}
}

func TestExtract_NestedField(t *testing.T) {
	body := []byte(`{"auth": {"token": "abc123", "expires": 3600}}`)
	rules := map[string]string{
		"token": "$.auth.token",
	}

	result, err := Extract(body, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["token"] != "abc123" {
		t.Errorf("expected 'abc123', got %v", result["token"])
	}
}

func TestExtract_ArrayIndex(t *testing.T) {
	body := []byte(`{"items": [{"id": 1}, {"id": 2}, {"id": 3}]}`)
	rules := map[string]string{
		"first_id":  "$.items[0].id",
		"second_id": "$.items[1].id",
	}

	result, err := Extract(body, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["first_id"] != float64(1) {
		t.Errorf("expected 1, got %v", result["first_id"])
	}
	if result["second_id"] != float64(2) {
		t.Errorf("expected 2, got %v", result["second_id"])
	}
}

func TestExtract_NumericValue(t *testing.T) {
	body := []byte(`{"user": {"id": 42, "score": 95.5}}`)
	rules := map[string]string{
		"user_id": "$.user.id",
		"score":   "$.user.score",
	}

	result, err := Extract(body, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["user_id"] != float64(42) {
		t.Errorf("expected 42, got %v", result["user_id"])
	}
	if result["score"] != 95.5 {
		t.Errorf("expected 95.5, got %v", result["score"])
	}
}

func TestExtract_BooleanValue(t *testing.T) {
	body := []byte(`{"active": true, "verified": false}`)
	rules := map[string]string{
		"active":   "$.active",
		"verified": "$.verified",
	}

	result, err := Extract(body, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["active"] != true {
		t.Errorf("expected true, got %v", result["active"])
	}
	if result["verified"] != false {
		t.Errorf("expected false, got %v", result["verified"])
	}
}

func TestExtract_PathNotFound(t *testing.T) {
	body := []byte(`{"name": "test"}`)
	rules := map[string]string{
		"missing": "$.nonexistent",
	}

	_, err := Extract(body, rules)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestExtract_InvalidJSON(t *testing.T) {
	body := []byte(`not valid json`)
	rules := map[string]string{
		"field": "$.field",
	}

	_, err := Extract(body, rules)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("expected 'invalid JSON' in error, got: %v", err)
	}
}

func TestExtract_EmptyRules(t *testing.T) {
	body := []byte(`{"name": "test"}`)

	result, err := Extract(body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty rules, got %v", result)
	}

	result, err = Extract(body, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty rules, got %v", result)
	}
}

func TestExtract_MultipleErrors(t *testing.T) {
	body := []byte(`{"name": "test"}`)
	rules := map[string]string{
		"missing1": "$.field1",
		"missing2": "$.field2",
	}

	_, err := Extract(body, rules)
	if err == nil {
		t.Fatal("expected errors for missing paths")
	}
	// Both errors should be mentioned
	errStr := err.Error()
	if !strings.Contains(errStr, "missing1") || !strings.Contains(errStr, "missing2") {
		t.Errorf("expected both missing variables in error, got: %v", err)
	}
}

func TestExtract_DeeplyNested(t *testing.T) {
	body := []byte(`{"level1": {"level2": {"level3": {"value": "deep"}}}}`)
	rules := map[string]string{
		"value": "$.level1.level2.level3.value",
	}

	result, err := Extract(body, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["value"] != "deep" {
		t.Errorf("expected 'deep', got %v", result["value"])
	}
}

func TestExtract_ArrayWildcard(t *testing.T) {
	body := []byte(`{"items": [{"name": "a"}, {"name": "b"}, {"name": "c"}]}`)
	rules := map[string]string{
		"names": "$.items[*].name",
	}

	result, err := Extract(body, rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, ok := result["names"].([]any)
	if !ok {
		t.Fatalf("expected slice, got %T", result["names"])
	}
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}
}

func TestConvertJSONPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"$.foo.bar", "foo.bar"},
		{"$foo.bar", "foo.bar"},
		{"foo.bar", "foo.bar"},
		{"$.items[0].id", "items.0.id"},
		{"$.items[10].id", "items.10.id"},
		{"$.data[*].name", "data.#.name"},
		{"$", ""},
		{"$.user", "user"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := convertJSONPath(tc.input)
			if result != tc.expected {
				t.Errorf("convertJSONPath(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// Benchmarks

func BenchmarkExtract_Simple(b *testing.B) {
	body := []byte(`{"auth": {"token": "abc123"}}`)
	rules := map[string]string{
		"token": "$.auth.token",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Extract(body, rules)
	}
}

func BenchmarkExtract_Multiple(b *testing.B) {
	body := []byte(`{
		"auth": {"token": "abc123", "expires": 3600},
		"user": {"id": 42, "name": "test"}
	}`)
	rules := map[string]string{
		"token":   "$.auth.token",
		"expires": "$.auth.expires",
		"user_id": "$.user.id",
		"name":    "$.user.name",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Extract(body, rules)
	}
}

func BenchmarkExtract_DeepNested(b *testing.B) {
	body := []byte(`{"level1": {"level2": {"level3": {"level4": {"value": "deep"}}}}}`)
	rules := map[string]string{
		"value": "$.level1.level2.level3.level4.value",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Extract(body, rules)
	}
}
