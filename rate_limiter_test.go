package burstsmith

import (
	"context"
	"testing"
	"time"
)

func TestNewRateLimiter_ZeroRPS(t *testing.T) {
	rl := NewRateLimiter(0)
	if rl != nil {
		t.Error("expected nil for zero RPS")
	}
}

func TestNewRateLimiter_NegativeRPS(t *testing.T) {
	rl := NewRateLimiter(-10)
	if rl != nil {
		t.Error("expected nil for negative RPS")
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

func TestRateLimiter_WaitNil(t *testing.T) {
	var rl *RateLimiter
	ctx := context.Background()

	// Should not panic and return nil
	err := rl.Wait(ctx)
	if err != nil {
		t.Errorf("expected nil error for nil limiter, got: %v", err)
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

func TestRateLimiter_SetRateNil(t *testing.T) {
	var rl *RateLimiter
	// Should not panic
	rl.SetRate(100)
}
