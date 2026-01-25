package core

import "sync"

// MockWriter is a thread-safe io.Writer for testing.
type MockWriter struct {
	mu   sync.Mutex
	data []byte
}

func (w *MockWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *MockWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.data)
}
