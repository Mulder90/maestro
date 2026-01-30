package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCSV(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "users.csv")

	content := `username,password,age
alice,secret1,25
bob,secret2,30
charlie,secret3,35`

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src, err := LoadFile("users", csvPath, ModeSequential, "")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if src.Len() != 3 {
		t.Errorf("Len() = %d, want 3", src.Len())
	}

	// Test sequential iteration
	row1 := src.Next()
	if row1["username"] != "alice" {
		t.Errorf("row1[username] = %v, want alice", row1["username"])
	}
	if row1["password"] != "secret1" {
		t.Errorf("row1[password] = %v, want secret1", row1["password"])
	}
	if row1["age"] != "25" {
		t.Errorf("row1[age] = %v, want 25", row1["age"])
	}

	row2 := src.Next()
	if row2["username"] != "bob" {
		t.Errorf("row2[username] = %v, want bob", row2["username"])
	}

	row3 := src.Next()
	if row3["username"] != "charlie" {
		t.Errorf("row3[username] = %v, want charlie", row3["username"])
	}

	// Test wrap-around
	row4 := src.Next()
	if row4["username"] != "alice" {
		t.Errorf("row4[username] = %v, want alice (wrap around)", row4["username"])
	}
}

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "products.json")

	content := `[
		{"id": 1, "name": "Widget", "price": 9.99},
		{"id": 2, "name": "Gadget", "price": 19.99}
	]`

	if err := os.WriteFile(jsonPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src, err := LoadFile("products", jsonPath, ModeSequential, "")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if src.Len() != 2 {
		t.Errorf("Len() = %d, want 2", src.Len())
	}

	row := src.Next()
	if row["id"] != float64(1) {
		t.Errorf("row[id] = %v (%T), want 1", row["id"], row["id"])
	}
	if row["name"] != "Widget" {
		t.Errorf("row[name] = %v, want Widget", row["name"])
	}
	if row["price"] != 9.99 {
		t.Errorf("row[price] = %v, want 9.99", row["price"])
	}
}

func TestRelativePath(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "data.csv")

	content := `col1
value1`

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Load with relative path and config dir
	src, err := LoadFile("test", "data.csv", ModeSequential, dir)
	if err != nil {
		t.Fatalf("LoadFile with relative path: %v", err)
	}

	if src.Len() != 1 {
		t.Errorf("Len() = %d, want 1", src.Len())
	}
}

func TestModeRandom(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "random.csv")

	content := `value
a
b
c
d
e`

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src, err := LoadFile("random", csvPath, ModeRandom, "")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	// With random mode, verify we get valid rows (can't predict order)
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		row := src.Next()
		val := row["value"].(string)
		seen[val] = true
	}

	// Should see multiple different values
	if len(seen) < 2 {
		t.Errorf("Random mode returned only %d unique values in 100 iterations", len(seen))
	}
}

func TestEmptySource(t *testing.T) {
	src := NewSource("empty", nil, ModeSequential)
	if src.Next() != nil {
		t.Error("Next() on empty source should return nil")
	}
}

func TestEmptyFile(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "empty.csv")

	content := `header`

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile("empty", csvPath, ModeSequential, "")
	if err == nil {
		t.Error("LoadFile should fail for CSV with no data rows")
	}
}

func TestUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.xml")

	if err := os.WriteFile(path, []byte("<data/>"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFile("xml", path, ModeSequential, "")
	if err == nil {
		t.Error("LoadFile should fail for unsupported format")
	}
}

func TestInjectVariables(t *testing.T) {
	src := NewSource("users", []map[string]any{
		{"username": "alice", "id": 123},
	}, ModeSequential)

	sources := Sources{"users": src}

	vars := &testVars{data: make(map[string]any)}
	sources.InjectVariables(vars)

	if v, ok := vars.data["data.users.username"]; !ok || v != "alice" {
		t.Errorf("data.users.username = %v, want alice", v)
	}
	if v, ok := vars.data["data.users.id"]; !ok || v != 123 {
		t.Errorf("data.users.id = %v, want 123", v)
	}
}

// testVars implements the interface expected by InjectVariables
type testVars struct {
	data map[string]any
}

func (v *testVars) Set(key string, value any) {
	v.data[key] = value
}

func TestConcurrentAccess(t *testing.T) {
	src := NewSource("test", []map[string]any{
		{"v": 1}, {"v": 2}, {"v": 3},
	}, ModeSequential)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				row := src.Next()
				if row == nil {
					t.Error("Next() returned nil")
				}
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestNextReturnsCopy(t *testing.T) {
	// Verify that Next() returns a copy, not a reference to internal data
	src := NewSource("test", []map[string]any{
		{"key": "original"},
	}, ModeSequential)

	// Get the first row and mutate it
	row1 := src.Next()
	row1["key"] = "mutated"
	row1["new_key"] = "added"

	// Get the same row again (wraps around)
	row2 := src.Next()

	// Original data should be unchanged
	if row2["key"] != "original" {
		t.Errorf("mutation affected original data: got %v, want 'original'", row2["key"])
	}
	if _, exists := row2["new_key"]; exists {
		t.Error("added key should not exist in original data")
	}
}
