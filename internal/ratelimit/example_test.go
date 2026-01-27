package ratelimit_test

import (
	"context"
	"fmt"
	"time"

	"maestro/internal/config"
	"maestro/internal/ratelimit"
)

func ExampleNewRateLimiter() {
	// Create a rate limiter allowing 100 requests per second
	limiter := ratelimit.NewRateLimiter(100)

	ctx := context.Background()

	// Wait for permission before making a request
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := limiter.Wait(ctx); err != nil {
			fmt.Println("Context cancelled")
			return
		}
	}
	elapsed := time.Since(start)

	fmt.Printf("5 requests completed in under 100ms: %v\n", elapsed < 100*time.Millisecond)
	// Output: 5 requests completed in under 100ms: true
}

func ExampleRateLimiter_SetRate() {
	limiter := ratelimit.NewRateLimiter(10)

	// Dynamically adjust rate during test
	limiter.SetRate(50) // Increase to 50 RPS

	fmt.Println("Rate updated to 50 RPS")
	// Output: Rate updated to 50 RPS
}

func ExampleNewPhaseManager() {
	phases := []config.Phase{
		{Name: "ramp_up", Duration: 10 * time.Second, StartActors: 1, EndActors: 10},
		{Name: "steady", Duration: 30 * time.Second, Actors: 10, RPS: 100},
		{Name: "ramp_down", Duration: 5 * time.Second, StartActors: 10, EndActors: 0},
	}

	pm := ratelimit.NewPhaseManager(phases)

	fmt.Printf("Phase: %s, Target actors: %d\n", pm.CurrentPhase().Name, pm.TargetActors())
	// Output: Phase: ramp_up, Target actors: 1
}
