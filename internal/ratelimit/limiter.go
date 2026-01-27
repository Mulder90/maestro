// Package ratelimit provides rate limiting and load profile phase management.
package ratelimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	limiter *rate.Limiter
	mu      sync.RWMutex
}

func NewRateLimiter(rps int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(rps), rps),
	}
}

func (r *RateLimiter) Wait(ctx context.Context) error {
	r.mu.RLock()
	limiter := r.limiter
	limit := limiter.Limit()
	r.mu.RUnlock()

	// If rate limit is 0, don't wait (no rate limiting)
	if limit == 0 {
		return nil
	}
	return limiter.Wait(ctx)
}

func (r *RateLimiter) SetRate(rps int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limiter.SetLimit(rate.Limit(rps))
	r.limiter.SetBurst(rps)
}
