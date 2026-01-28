package template

import (
	"os"
	"strings"
	"testing"

	"maestro/internal/core"
)

func TestSubstitute_NoPlaceholders(t *testing.T) {
	vars := core.NewVariables()
	text := "Bearer static-token"

	result, err := Substitute(text, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != text {
		t.Errorf("expected %q, got %q", text, result)
	}
}

func TestSubstitute_SingleVariable(t *testing.T) {
	vars := core.NewVariables()
	vars.Set("token", "abc123")

	result, err := Substitute("Bearer ${token}", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Bearer abc123" {
		t.Errorf("expected 'Bearer abc123', got %q", result)
	}
}

func TestSubstitute_MultipleVariables(t *testing.T) {
	vars := core.NewVariables()
	vars.Set("user_id", "42")
	vars.Set("token", "secret")

	result, err := Substitute("/users/${user_id}?auth=${token}", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "/users/42?auth=secret" {
		t.Errorf("expected '/users/42?auth=secret', got %q", result)
	}
}

func TestSubstitute_EnvironmentVariable(t *testing.T) {
	os.Setenv("TEST_API_BASE", "https://api.example.com")
	defer os.Unsetenv("TEST_API_BASE")

	vars := core.NewVariables()
	result, err := Substitute("${env:TEST_API_BASE}/users", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "https://api.example.com/users" {
		t.Errorf("expected 'https://api.example.com/users', got %q", result)
	}
}

func TestSubstitute_MixedVariables(t *testing.T) {
	os.Setenv("TEST_BASE_URL", "https://api.example.com")
	defer os.Unsetenv("TEST_BASE_URL")

	vars := core.NewVariables()
	vars.Set("user_id", "123")
	vars.Set("token", "bearer-token")

	result, err := Substitute("${env:TEST_BASE_URL}/users/${user_id}?token=${token}", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "https://api.example.com/users/123?token=bearer-token"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSubstitute_MissingVariable(t *testing.T) {
	vars := core.NewVariables()

	_, err := Substitute("Bearer ${missing_token}", vars)
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
	if !strings.Contains(err.Error(), `variable "missing_token" not found`) {
		t.Errorf("expected error mentioning missing variable, got: %v", err)
	}
}

func TestSubstitute_MissingEnvVariable(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR")
	vars := core.NewVariables()

	_, err := Substitute("${env:NONEXISTENT_VAR}/path", vars)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), `env var "NONEXISTENT_VAR" not set`) {
		t.Errorf("expected error mentioning missing env var, got: %v", err)
	}
}

func TestSubstitute_MultipleErrors(t *testing.T) {
	vars := core.NewVariables()

	_, err := Substitute("${missing1} and ${missing2}", vars)
	if err == nil {
		t.Fatal("expected errors for missing variables")
	}
	// errors.Join combines errors; both should be mentioned
	errStr := err.Error()
	if !strings.Contains(errStr, "missing1") || !strings.Contains(errStr, "missing2") {
		t.Errorf("expected both missing variables in error, got: %v", err)
	}
}

func TestSubstitute_NumericValue(t *testing.T) {
	vars := core.NewVariables()
	vars.Set("count", 42)

	result, err := Substitute("count=${count}", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "count=42" {
		t.Errorf("expected 'count=42', got %q", result)
	}
}

func TestSubstitute_FloatValue(t *testing.T) {
	vars := core.NewVariables()
	vars.Set("price", 19.99)

	result, err := Substitute("price=${price}", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "price=19.99" {
		t.Errorf("expected 'price=19.99', got %q", result)
	}
}

func TestSubstitute_EmptyString(t *testing.T) {
	vars := core.NewVariables()

	result, err := Substitute("", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestSubstituteMap_Success(t *testing.T) {
	vars := core.NewVariables()
	vars.Set("token", "abc123")
	vars.Set("content_type", "application/json")

	headers := map[string]string{
		"Authorization": "Bearer ${token}",
		"Content-Type":  "${content_type}",
		"X-Static":      "no-substitution",
	}

	result, err := SubstituteMap(headers, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["Authorization"] != "Bearer abc123" {
		t.Errorf("expected 'Bearer abc123', got %q", result["Authorization"])
	}
	if result["Content-Type"] != "application/json" {
		t.Errorf("expected 'application/json', got %q", result["Content-Type"])
	}
	if result["X-Static"] != "no-substitution" {
		t.Errorf("expected 'no-substitution', got %q", result["X-Static"])
	}
}

func TestSubstituteMap_NilMap(t *testing.T) {
	vars := core.NewVariables()

	result, err := SubstituteMap(nil, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSubstituteMap_Error(t *testing.T) {
	vars := core.NewVariables()

	headers := map[string]string{
		"Authorization": "Bearer ${missing}",
	}

	_, err := SubstituteMap(headers, vars)
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
}

// Benchmarks

func BenchmarkSubstitute(b *testing.B) {
	vars := core.NewVariables()
	vars.Set("token", "abc123")
	text := "Bearer ${token}"

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Substitute(text, vars)
	}
}

func BenchmarkSubstitute_NoVars(b *testing.B) {
	vars := core.NewVariables()
	text := "Bearer static-token"

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Substitute(text, vars)
	}
}

func BenchmarkSubstitute_MultipleVars(b *testing.B) {
	vars := core.NewVariables()
	vars.Set("base", "https://api.example.com")
	vars.Set("user_id", "12345")
	vars.Set("token", "abcdef123456")
	text := "${base}/users/${user_id}?token=${token}"

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Substitute(text, vars)
	}
}

func BenchmarkSubstituteMap(b *testing.B) {
	vars := core.NewVariables()
	vars.Set("token", "abc123")
	vars.Set("content_type", "application/json")

	headers := map[string]string{
		"Authorization": "Bearer ${token}",
		"Content-Type":  "${content_type}",
		"X-Request-ID":  "static-id",
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = SubstituteMap(headers, vars)
	}
}
