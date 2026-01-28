package template

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// funcPattern matches function calls like uuid(), random(1,100), date(2006-01-02)
var funcRegistry = map[string]func(args string) (string, error){
	"uuid":          fnUUID,
	"timestamp":     fnTimestamp,
	"timestamp_ms":  fnTimestampMs,
	"random":        fnRandom,
	"random_string": fnRandomString,
	"date":          fnDate,
}

// evalFunction evaluates a built-in function call.
// Returns the result string, or empty string and false if not a function.
func evalFunction(expr string) (string, bool, error) {
	// Check if it looks like a function call (contains parentheses)
	parenIdx := strings.Index(expr, "(")
	if parenIdx == -1 || !strings.HasSuffix(expr, ")") {
		return "", false, nil
	}

	funcName := expr[:parenIdx]
	args := expr[parenIdx+1 : len(expr)-1]

	fn, ok := funcRegistry[funcName]
	if !ok {
		return "", false, nil
	}

	result, err := fn(args)
	if err != nil {
		return "", true, fmt.Errorf("function %s: %w", funcName, err)
	}
	return result, true, nil
}

// fnUUID generates a UUID v4.
func fnUUID(args string) (string, error) {
	if args != "" {
		return "", fmt.Errorf("uuid() takes no arguments")
	}

	uuid := make([]byte, 16)
	if _, err := rand.Read(uuid); err != nil {
		return "", err
	}

	// Set version (4) and variant (RFC 4122)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}

// fnTimestamp returns the current Unix timestamp in seconds.
func fnTimestamp(args string) (string, error) {
	if args != "" {
		return "", fmt.Errorf("timestamp() takes no arguments")
	}
	return strconv.FormatInt(time.Now().Unix(), 10), nil
}

// fnTimestampMs returns the current Unix timestamp in milliseconds.
func fnTimestampMs(args string) (string, error) {
	if args != "" {
		return "", fmt.Errorf("timestamp_ms() takes no arguments")
	}
	return strconv.FormatInt(time.Now().UnixMilli(), 10), nil
}

// fnRandom generates a random integer between min and max (inclusive).
// Usage: random(min,max)
func fnRandom(args string) (string, error) {
	parts := strings.Split(args, ",")
	if len(parts) != 2 {
		return "", fmt.Errorf("random(min,max) requires exactly 2 arguments")
	}

	min, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid min value: %w", err)
	}

	max, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid max value: %w", err)
	}

	if min > max {
		return "", fmt.Errorf("min (%d) must be <= max (%d)", min, max)
	}

	// Generate random number in range [min, max]
	rangeSize := max - min + 1
	n, err := rand.Int(rand.Reader, big.NewInt(rangeSize))
	if err != nil {
		return "", err
	}

	return strconv.FormatInt(min+n.Int64(), 10), nil
}

// fnRandomString generates a random alphanumeric string of the specified length.
// Usage: random_string(length)
func fnRandomString(args string) (string, error) {
	length, err := strconv.Atoi(strings.TrimSpace(args))
	if err != nil {
		return "", fmt.Errorf("invalid length: %w", err)
	}
	if length <= 0 {
		return "", fmt.Errorf("length must be positive")
	}
	if length > 1000 {
		return "", fmt.Errorf("length must be <= 1000")
	}

	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}

	return string(result), nil
}

// fnDate formats the current time using Go's time format.
// Usage: date(format) where format uses Go's reference time (2006-01-02 15:04:05)
// Common formats:
//   - date(2006-01-02) -> 2024-01-15
//   - date(15:04:05) -> 14:30:00
//   - date(2006-01-02T15:04:05Z07:00) -> ISO 8601
func fnDate(args string) (string, error) {
	format := strings.TrimSpace(args)
	if format == "" {
		format = time.RFC3339
	}
	return time.Now().Format(format), nil
}
