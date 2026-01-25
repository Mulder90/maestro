package coordinator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"burstsmith/internal/config"
	"burstsmith/internal/core"
	"burstsmith/internal/progress"
	"burstsmith/internal/ratelimit"
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
					c.spawnWithStop(ctx, workflow)
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
