package http

import (
	"context"
	"net/http"
	"sync"
	"time"

	"maestro/internal/config"
	"maestro/internal/core"
	"maestro/internal/ratelimit"
)

type Workflow struct {
	Config      config.WorkflowConfig
	Client      *http.Client
	RateLimiter *ratelimit.RateLimiter
	Debug       *DebugLogger

	steps     []core.Step
	stepsOnce sync.Once
}

func (w *Workflow) Run(ctx context.Context, actorID int, coord core.Coordinator, rep core.Reporter) error {
	if w.RateLimiter != nil {
		if err := w.RateLimiter.Wait(ctx); err != nil {
			return err
		}
	}

	w.stepsOnce.Do(func() {
		w.steps = make([]core.Step, len(w.Config.Steps))
		for i, cfg := range w.Config.Steps {
			w.steps[i] = NewStep(cfg, w.Client, w.Debug)
		}
	})

	ctx = core.ContextWithActorID(ctx, actorID)
	vars := core.NewVariables()

	for _, step := range w.steps {
		result, err := step.Execute(ctx, vars)

		rep.Report(core.Event{
			ActorID:    actorID,
			Timestamp:  time.Now(),
			Step:       step.Name(),
			Protocol:   "http",
			Duration:   result.Duration,
			Success:    result.Success,
			Error:      result.Error,
			StatusCode: result.StatusCode,
			BytesSent:  result.BytesSent,
			BytesRecv:  result.BytesRecv,
		})

		if result.Extract != nil {
			for k, v := range result.Extract {
				vars.Set(k, v)
			}
		}

		if err != nil {
			return err
		}
	}

	return nil
}
