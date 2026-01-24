package burstsmith

import (
	"context"

	"golang.org/x/time/rate"
)

// RateLimiter wraps a token bucket rate limiter for controlling request rates.
type RateLimiter struct {
	limiter *rate.Limiter
}

// NewRateLimiter creates a rate limiter that allows rps requests per second.
// If rps is 0 or negative, returns nil (no rate limiting).
func NewRateLimiter(rps int) *RateLimiter {
	if rps <= 0 {
		return nil
	}
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), rps), // burst size = rps
	}
}

// Wait blocks until the rate limiter allows an event or ctx is cancelled.
func (r *RateLimiter) Wait(ctx context.Context) error {
	if r == nil || r.limiter == nil {
		return nil
	}
	return r.limiter.Wait(ctx)
}

// SetRate updates the rate limit to a new RPS value.
func (r *RateLimiter) SetRate(rps int) {
	if r == nil || r.limiter == nil {
		return
	}
	if rps <= 0 {
		// Set a very high limit effectively disabling rate limiting
		r.limiter.SetLimit(rate.Inf)
		return
	}
	r.limiter.SetLimit(rate.Limit(rps))
	r.limiter.SetBurst(rps)
}
