package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(100)
	if rl == nil {
		t.Error("expected non-nil rate limiter")
	}
}

func TestNewRateLimiter_ZeroRPS(t *testing.T) {
	rl := NewRateLimiter(0)
	if rl == nil {
		t.Error("expected non-nil rate limiter even with zero RPS")
	}

	// Should not block with zero RPS
	ctx := context.Background()
	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if elapsed > 10*time.Millisecond {
		t.Errorf("zero RPS should not block, took %v", elapsed)
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(1000) // 1000 RPS - should be fast
	ctx := context.Background()

	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should be nearly instant with 1000 RPS
	if elapsed > 50*time.Millisecond {
		t.Errorf("wait took too long: %v", elapsed)
	}
}

func TestRateLimiter_ContextCancelled(t *testing.T) {
	rl := NewRateLimiter(1) // Very slow - 1 RPS

	// Exhaust the burst
	ctx := context.Background()
	_ = rl.Wait(ctx)

	// Now with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestRateLimiter_SetRate(t *testing.T) {
	rl := NewRateLimiter(100)

	// Should not panic
	rl.SetRate(200)
	rl.SetRate(0) // Disable limiting
}

func TestRateLimiter_SetRateToZero(t *testing.T) {
	rl := NewRateLimiter(100)
	rl.SetRate(0)

	// Should not block after setting rate to 0
	ctx := context.Background()
	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if elapsed > 10*time.Millisecond {
		t.Errorf("zero rate should not block, took %v", elapsed)
	}
}

func TestRateLimiter_RateLimiting(t *testing.T) {
	rl := NewRateLimiter(10) // 10 RPS

	ctx := context.Background()
	start := time.Now()

	// Make 15 requests - first 10 should be instant (burst), next 5 should wait
	for i := 0; i < 15; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("wait failed: %v", err)
		}
	}

	elapsed := time.Since(start)

	// With 10 RPS and 15 requests: first 10 instant, next 5 need 500ms
	// Allow some tolerance
	if elapsed < 400*time.Millisecond {
		t.Errorf("rate limiting doesn't appear to be working, elapsed: %v", elapsed)
	}
}

func TestRateLimiter_DynamicRateChange(t *testing.T) {
	rl := NewRateLimiter(1000) // Start fast

	ctx := context.Background()

	// Fast requests initially
	for i := 0; i < 5; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("wait failed: %v", err)
		}
	}

	// Change to unlimited
	rl.SetRate(0)

	// Should still be fast
	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("wait failed: %v", err)
		}
	}
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("unlimited rate should be fast, took %v", elapsed)
	}
}

func TestRateLimiter_ConcurrentWait(t *testing.T) {
	rl := NewRateLimiter(100)
	ctx := context.Background()

	// Multiple goroutines waiting concurrently should not panic
	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 10; j++ {
				if err := rl.Wait(ctx); err != nil {
					return
				}
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
