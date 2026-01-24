package burstsmith

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPWorkflow executes a sequence of HTTP requests defined in a config.
type HTTPWorkflow struct {
	Config      WorkflowConfig
	Client      *http.Client
	RateLimiter *RateLimiter
}

// Run executes all steps in the workflow sequentially.
func (w *HTTPWorkflow) Run(ctx context.Context, actorID int, coord Coordinator, rep Reporter) error {
	// Wait for rate limiter before starting workflow iteration
	if w.RateLimiter != nil {
		if err := w.RateLimiter.Wait(ctx); err != nil {
			return err
		}
	}

	for _, step := range w.Config.Steps {
		if err := w.runStep(ctx, actorID, step, rep); err != nil {
			return err
		}
	}
	return nil
}

// runStep executes a single HTTP request step.
func (w *HTTPWorkflow) runStep(ctx context.Context, actorID int, step StepConfig, rep Reporter) error {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, step.Method, step.URL, strings.NewReader(step.Body))
	if err != nil {
		rep.Report(Event{
			ActorID:   actorID,
			Timestamp: time.Now(),
			Step:      step.Name,
			Duration:  time.Since(start),
			Success:   false,
			Error:     err.Error(),
		})
		return err
	}

	for k, v := range step.Headers {
		req.Header.Set(k, v)
	}

	resp, err := w.Client.Do(req)
	duration := time.Since(start)

	if err != nil {
		rep.Report(Event{
			ActorID:   actorID,
			Timestamp: time.Now(),
			Step:      step.Name,
			Duration:  duration,
			Success:   false,
			Error:     err.Error(),
		})
		return err
	}
	defer resp.Body.Close()

	// Drain body to allow connection reuse
	io.Copy(io.Discard, resp.Body)

	success := resp.StatusCode < 400
	errStr := ""
	if !success {
		errStr = resp.Status
	}

	rep.Report(Event{
		ActorID:   actorID,
		Timestamp: time.Now(),
		Step:      step.Name,
		Duration:  duration,
		Success:   success,
		Error:     errStr,
	})

	if !success {
		return nil // Continue workflow even on HTTP errors
	}

	return nil
}
