package template

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"maestro/internal/core"
)

func TestFnUUID(t *testing.T) {
	result, err := fnUUID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// UUID v4 format: 8-4-4-4-12 hex digits
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(result) {
		t.Errorf("invalid UUID format: %s", result)
	}

	// Generate another UUID, should be different
	result2, _ := fnUUID("")
	if result == result2 {
		t.Error("UUIDs should be unique")
	}
}

func TestFnUUID_WithArgs(t *testing.T) {
	_, err := fnUUID("extra")
	if err == nil {
		t.Error("expected error for uuid() with arguments")
	}
}

func TestFnTimestamp(t *testing.T) {
	before := time.Now().Unix()
	result, err := fnTimestamp("")
	after := time.Now().Unix()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ts, err := strconv.ParseInt(result, 10, 64)
	if err != nil {
		t.Fatalf("invalid timestamp: %v", err)
	}

	if ts < before || ts > after {
		t.Errorf("timestamp %d not in expected range [%d, %d]", ts, before, after)
	}
}

func TestFnTimestampMs(t *testing.T) {
	before := time.Now().UnixMilli()
	result, err := fnTimestampMs("")
	after := time.Now().UnixMilli()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ts, err := strconv.ParseInt(result, 10, 64)
	if err != nil {
		t.Fatalf("invalid timestamp: %v", err)
	}

	if ts < before || ts > after {
		t.Errorf("timestamp_ms %d not in expected range [%d, %d]", ts, before, after)
	}
}

func TestFnRandom(t *testing.T) {
	result, err := fnRandom("1,10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	n, err := strconv.Atoi(result)
	if err != nil {
		t.Fatalf("invalid number: %v", err)
	}

	if n < 1 || n > 10 {
		t.Errorf("random number %d not in range [1, 10]", n)
	}
}

func TestFnRandom_WithSpaces(t *testing.T) {
	result, err := fnRandom(" 5 , 15 ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	n, err := strconv.Atoi(result)
	if err != nil {
		t.Fatalf("invalid number: %v", err)
	}

	if n < 5 || n > 15 {
		t.Errorf("random number %d not in range [5, 15]", n)
	}
}

func TestFnRandom_SingleValue(t *testing.T) {
	result, err := fnRandom("42,42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "42" {
		t.Errorf("expected 42, got %s", result)
	}
}

func TestFnRandom_InvalidArgs(t *testing.T) {
	tests := []struct {
		args string
		desc string
	}{
		{"", "empty args"},
		{"1", "single arg"},
		{"1,2,3", "too many args"},
		{"a,b", "non-numeric"},
		{"10,5", "min > max"},
	}

	for _, tc := range tests {
		_, err := fnRandom(tc.args)
		if err == nil {
			t.Errorf("expected error for %s: %q", tc.desc, tc.args)
		}
	}
}

func TestFnRandomString(t *testing.T) {
	result, err := fnRandomString("16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 16 {
		t.Errorf("expected length 16, got %d", len(result))
	}

	// Should be alphanumeric
	for _, c := range result {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			t.Errorf("unexpected character: %c", c)
		}
	}
}

func TestFnRandomString_InvalidArgs(t *testing.T) {
	tests := []struct {
		args string
		desc string
	}{
		{"", "empty"},
		{"abc", "non-numeric"},
		{"0", "zero length"},
		{"-5", "negative"},
		{"1001", "too long"},
	}

	for _, tc := range tests {
		_, err := fnRandomString(tc.args)
		if err == nil {
			t.Errorf("expected error for %s: %q", tc.desc, tc.args)
		}
	}
}

func TestFnDate(t *testing.T) {
	result, err := fnDate("2006-01-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := time.Now().Format("2006-01-02")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestFnDate_EmptyFormat(t *testing.T) {
	result, err := fnDate("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use RFC3339 format
	_, err = time.Parse(time.RFC3339, result)
	if err != nil {
		t.Errorf("expected RFC3339 format, got %s", result)
	}
}

func TestSubstitute_Functions(t *testing.T) {
	vars := core.NewVariables()

	tests := []struct {
		input   string
		pattern string // regex pattern to match result
	}{
		{"${uuid()}", `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`},
		{"${timestamp()}", `^\d{10}$`},
		{"${timestamp_ms()}", `^\d{13}$`},
		{"${random(1,100)}", `^\d{1,3}$`},
		{"${random_string(8)}", `^[a-zA-Z0-9]{8}$`},
		{"${date(2006-01-02)}", `^\d{4}-\d{2}-\d{2}$`},
	}

	for _, tc := range tests {
		result, err := Substitute(tc.input, vars)
		if err != nil {
			t.Errorf("Substitute(%q) error: %v", tc.input, err)
			continue
		}

		matched, _ := regexp.MatchString(tc.pattern, result)
		if !matched {
			t.Errorf("Substitute(%q) = %q, doesn't match pattern %s", tc.input, result, tc.pattern)
		}
	}
}

func TestSubstitute_FunctionInText(t *testing.T) {
	vars := core.NewVariables()

	result, err := Substitute("id-${uuid()}-suffix", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(result, "id-") || !strings.HasSuffix(result, "-suffix") {
		t.Errorf("unexpected result: %s", result)
	}

	// Middle part should be a UUID
	middle := result[3 : len(result)-7]
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidPattern.MatchString(middle) {
		t.Errorf("middle part is not a valid UUID: %s", middle)
	}
}

func TestSubstitute_MixedFunctionsAndVariables(t *testing.T) {
	vars := core.NewVariables()
	vars.Set("user", "alice")

	result, err := Substitute("user=${user}&session=${uuid()}", vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(result, "user=alice&session=") {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestSubstitute_InvalidFunction(t *testing.T) {
	vars := core.NewVariables()

	_, err := Substitute("${random(abc)}", vars)
	if err == nil {
		t.Error("expected error for invalid function args")
	}
}

func TestSubstitute_UnknownFunction(t *testing.T) {
	vars := core.NewVariables()

	// unknown_func() should be treated as a missing variable
	_, err := Substitute("${unknown_func()}", vars)
	if err == nil {
		t.Error("expected error for unknown function")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// Benchmarks

func BenchmarkFnUUID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = fnUUID("")
	}
}

func BenchmarkFnRandom(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = fnRandom("1,1000000")
	}
}

func BenchmarkFnRandomString(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = fnRandomString("32")
	}
}

func BenchmarkSubstitute_WithFunction(b *testing.B) {
	vars := core.NewVariables()
	text := "id=${uuid()}&ts=${timestamp()}"

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Substitute(text, vars)
	}
}
