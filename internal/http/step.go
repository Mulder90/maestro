package http

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"maestro/internal/config"
	"maestro/internal/core"
)

const maxDebugBodySize = 4096

type Step struct {
	config config.StepConfig
	client *http.Client
	debug  *DebugLogger
}

func NewStep(cfg config.StepConfig, client *http.Client, debug *DebugLogger) *Step {
	return &Step{
		config: cfg,
		client: client,
		debug:  debug,
	}
}

func (s *Step) Name() string {
	return s.config.Name
}

func (s *Step) Execute(ctx context.Context, vars core.Variables) (core.Result, error) {
	actorID := core.ActorIDFromContext(ctx)
	start := time.Now()

	url := s.config.URL
	body := s.config.Body

	req, err := http.NewRequestWithContext(ctx, s.config.Method, url, strings.NewReader(body))
	if err != nil {
		duration := time.Since(start)
		s.debug.LogError(actorID, s.config.Name, err.Error(), duration)
		return core.Result{
			Duration: duration,
			Success:  false,
			Error:    err.Error(),
		}, err
	}

	for k, v := range s.config.Headers {
		req.Header.Set(k, v)
	}

	s.debug.LogRequest(actorID, s.config.Name, req)

	resp, err := s.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		s.debug.LogError(actorID, s.config.Name, err.Error(), duration)
		return core.Result{
			Duration: duration,
			Success:  false,
			Error:    err.Error(),
		}, err
	}
	defer resp.Body.Close()

	var respBody []byte
	if s.debug != nil {
		respBody, _ = io.ReadAll(io.LimitReader(resp.Body, maxDebugBodySize))
		_, _ = io.Copy(io.Discard, resp.Body) // drain errors are ignorable
	} else {
		_, _ = io.Copy(io.Discard, resp.Body) // drain errors are ignorable
	}

	success := resp.StatusCode < 400
	errStr := ""
	if !success {
		errStr = resp.Status
	}

	s.debug.LogResponse(actorID, s.config.Name, resp, respBody, duration)

	return core.Result{
		Duration:   duration,
		Success:    success,
		Error:      errStr,
		StatusCode: resp.StatusCode,
		BytesSent:  int64(len(body)),
		BytesRecv:  int64(len(respBody)),
	}, nil
}
