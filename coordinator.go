package burstsmith

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultCoordinator spawns actors and assigns unique IDs.
type DefaultCoordinator struct {
	nextID      atomic.Int64
	wg          sync.WaitGroup
	reporter    Reporter
	activeCount atomic.Int32
	stopChans   []chan struct{}
	stopMu      sync.Mutex
}

// NewCoordinator creates a new DefaultCoordinator with the given reporter.
func NewCoordinator(reporter Reporter) *DefaultCoordinator {
	return &DefaultCoordinator{
		reporter: reporter,
	}
}

// Spawn launches count goroutines, each running the given workflow.
// Spawned actors inherit the caller's context (shared deadline/cancellation).
func (c *DefaultCoordinator) Spawn(ctx context.Context, count int, workflow Workflow) {
	for i := 0; i < count; i++ {
		actorID := int(c.nextID.Add(1))
		c.wg.Add(1)
		go func(id int) {
			defer c.wg.Done()
			_ = workflow.Run(ctx, id, c, c.reporter)
		}(actorID)
	}
}

// Wait blocks until all spawned actors have completed.
func (c *DefaultCoordinator) Wait() {
	c.wg.Wait()
}

// ActiveActors returns the current number of active actors.
func (c *DefaultCoordinator) ActiveActors() int {
	return int(c.activeCount.Load())
}

// spawnWithStop launches an actor that can be stopped via a stop channel.
func (c *DefaultCoordinator) spawnWithStop(ctx context.Context, workflow Workflow) chan struct{} {
	stopCh := make(chan struct{})
	actorID := int(c.nextID.Add(1))
	c.activeCount.Add(1)
	c.wg.Add(1)

	c.stopMu.Lock()
	c.stopChans = append(c.stopChans, stopCh)
	c.stopMu.Unlock()

	go func(id int, stop chan struct{}) {
		defer func() {
			c.wg.Done()
			c.activeCount.Add(-1)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			default:
				err := workflow.Run(ctx, id, c, c.reporter)
				if err != nil {
					return
				}
			}
		}
	}(actorID, stopCh)

	return stopCh
}

// stopActors terminates n actors by signaling their stop channels.
func (c *DefaultCoordinator) stopActors(n int) {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()

	toStop := n
	if toStop > len(c.stopChans) {
		toStop = len(c.stopChans)
	}

	for i := 0; i < toStop; i++ {
		close(c.stopChans[i])
	}
	c.stopChans = c.stopChans[toStop:]
}

// RunWithProfile executes a workflow according to a load profile.
// If progress is provided, phase announcements are printed through it to coordinate with progress display.
func (c *DefaultCoordinator) RunWithProfile(ctx context.Context, profile *LoadProfile, workflow Workflow, rateLimiter *RateLimiter, progress *Progress) {
	pm := NewPhaseManager(profile.Phases)

	// Helper function to print messages
	printMsg := func(format string, args ...interface{}) {
		if progress != nil {
			progress.Printf(format, args...)
		} else {
			fmt.Printf(format+"\n", args...)
		}
	}

	printMsg("Starting load profile with %d phases, total duration: %v",
		len(profile.Phases), profile.TotalDuration())

	currentPhaseIdx := -1
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.stopAllActors()
			return
		case <-ticker.C:
			if pm.IsComplete() {
				c.stopAllActors()
				return
			}

			// Check for phase transition
			newPhaseIdx := pm.CurrentPhaseIndex()
			if newPhaseIdx != currentPhaseIdx {
				currentPhaseIdx = newPhaseIdx
				phase := pm.CurrentPhase()
				if phase != nil {
					if phase.RPS > 0 {
						printMsg("Phase: %s (duration: %v, target actors: %d, rps: %d)",
							phase.Name, phase.Duration, pm.TargetActors(), phase.RPS)
					} else {
						printMsg("Phase: %s (duration: %v, target actors: %d)",
							phase.Name, phase.Duration, pm.TargetActors())
					}
				}
			}

			// Adjust actor count
			target := pm.TargetActors()
			current := c.ActiveActors()

			if current < target {
				// Spawn more actors
				for i := current; i < target; i++ {
					c.spawnWithStop(ctx, workflow)
				}
			} else if current > target {
				// Stop excess actors
				c.stopActors(current - target)
			}

			// Update rate limiter if RPS changed
			if rateLimiter != nil {
				rateLimiter.SetRate(pm.CurrentRPS())
			}
		}
	}
}

// stopAllActors terminates all running actors.
func (c *DefaultCoordinator) stopAllActors() {
	c.stopMu.Lock()
	for _, ch := range c.stopChans {
		close(ch)
	}
	c.stopChans = nil
	c.stopMu.Unlock()
}
