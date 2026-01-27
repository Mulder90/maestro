// Package coordinator manages actor lifecycle and orchestration.
package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"maestro/internal/config"
	"maestro/internal/core"
	"maestro/internal/progress"
	"maestro/internal/ratelimit"
)

const (
	// phaseTickInterval is how often we check for phase transitions
	// and adjust actor counts during load profile execution.
	phaseTickInterval = 100 * time.Millisecond
)

type Coordinator struct {
	nextID      atomic.Int64
	wg          sync.WaitGroup
	reporter    core.Reporter
	activeCount atomic.Int32
	stopChans   []chan struct{}
	stopMu      sync.Mutex
}

func NewCoordinator(reporter core.Reporter) *Coordinator {
	return &Coordinator{
		reporter: reporter,
	}
}

func (c *Coordinator) Spawn(ctx context.Context, count int, workflow core.Workflow) {
	for i := 0; i < count; i++ {
		actorID := int(c.nextID.Add(1))
		c.wg.Add(1)
		go func(id int) {
			defer c.wg.Done()
			defer c.recoverPanic(id)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					if err := workflow.Run(ctx, id, c, c.reporter); err != nil {
						return
					}
				}
			}
		}(actorID)
	}
}

// SpawnWithConfig spawns actors using Runner for iteration-level control.
func (c *Coordinator) SpawnWithConfig(ctx context.Context, count int, workflow core.Workflow, config core.RunnerConfig) {
	for i := 0; i < count; i++ {
		actorID := int(c.nextID.Add(1))
		c.wg.Add(1)
		go func(id int) {
			defer c.wg.Done()
			defer c.recoverPanic(id)
			runner := core.NewRunner(workflow, c.reporter, c, id, config)
			for {
				select {
				case <-ctx.Done():
					return
				default:
					err := runner.RunIteration(ctx)
					if err != nil {
						if errors.Is(err, core.ErrMaxIterationsReached) {
							return // Clean exit
						}
						return // Workflow error
					}
				}
			}
		}(actorID)
	}
}

func (c *Coordinator) Wait() {
	c.wg.Wait()
}

func (c *Coordinator) ActiveActors() int {
	return int(c.activeCount.Load())
}

func (c *Coordinator) spawnWithStop(ctx context.Context, workflow core.Workflow) chan struct{} {
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
		defer c.recoverPanic(id)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			default:
				if err := workflow.Run(ctx, id, c, c.reporter); err != nil {
					return
				}
			}
		}
	}(actorID, stopCh)

	return stopCh
}

func (c *Coordinator) spawnWithStopConfig(ctx context.Context, workflow core.Workflow, config core.RunnerConfig) chan struct{} {
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
		defer c.recoverPanic(id)
		runner := core.NewRunner(workflow, c.reporter, c, id, config)
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			default:
				err := runner.RunIteration(ctx)
				if err != nil {
					if errors.Is(err, core.ErrMaxIterationsReached) {
						return // Clean exit
					}
					return // Workflow error
				}
			}
		}
	}(actorID, stopCh)

	return stopCh
}

// recoverPanic recovers from panics in actor goroutines and reports them as failed events.
func (c *Coordinator) recoverPanic(actorID int) {
	if r := recover(); r != nil {
		c.reporter.Report(core.Event{
			ActorID: actorID,
			Step:    "panic",
			Success: false,
			Error:   fmt.Sprintf("panic: %v", r),
		})
	}
}

func (c *Coordinator) stopActors(n int) {
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

func (c *Coordinator) stopAllActors() {
	c.stopMu.Lock()
	for _, ch := range c.stopChans {
		close(ch)
	}
	c.stopChans = nil
	c.stopMu.Unlock()
}

func (c *Coordinator) RunWithProfile(ctx context.Context, profile *config.LoadProfile, workflow core.Workflow, rateLimiter *ratelimit.RateLimiter, prog *progress.Progress) {
	c.RunWithProfileConfig(ctx, profile, workflow, rateLimiter, prog, core.RunnerConfig{})
}

func (c *Coordinator) RunWithProfileConfig(ctx context.Context, profile *config.LoadProfile, workflow core.Workflow, rateLimiter *ratelimit.RateLimiter, prog *progress.Progress, config core.RunnerConfig) {
	pm := ratelimit.NewPhaseManager(profile.Phases)

	printMsg := func(format string, args ...interface{}) {
		if prog != nil {
			prog.Printf(format, args...)
		} else {
			fmt.Printf(format+"\n", args...)
		}
	}

	printMsg("Starting load profile with %d phases, total duration: %v",
		len(profile.Phases), profile.TotalDuration())

	useRunner := config.MaxIterations > 0 || config.WarmupIters > 0

	currentPhaseIdx := -1
	ticker := time.NewTicker(phaseTickInterval)
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
			target := pm.TargetActors()
			current := c.ActiveActors()
			if current < target {
				for i := current; i < target; i++ {
					if useRunner {
						c.spawnWithStopConfig(ctx, workflow, config)
					} else {
						c.spawnWithStop(ctx, workflow)
					}
				}
			} else if current > target {
				c.stopActors(current - target)
			}
			if rateLimiter != nil {
				rateLimiter.SetRate(pm.CurrentRPS())
			}
		}
	}
}
