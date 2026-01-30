package http

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"maestro/internal/config"
	"maestro/internal/core"
	"maestro/internal/template"
)

const (
	// maxDebugBodySize limits response body logged in verbose mode.
	maxDebugBodySize = 4096
	// maxExtractBodySize limits response body read for variable extraction.
	// Larger than debug to support extracting from bigger JSON responses.
	maxExtractBodySize = 10 * 1024 * 1024 // 10MB
)

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

	// Substitute variables in URL
	url, err := template.Substitute(s.config.URL, vars)
	if err != nil {
		duration := time.Since(start)
		s.debug.LogError(actorID, s.config.Name, err.Error(), duration)
		return core.Result{
			Duration: duration,
			Success:  false,
			Error:    err.Error(),
		}, err
	}

	// Substitute variables in body
	body, err := template.Substitute(s.config.Body, vars)
	if err != nil {
		duration := time.Since(start)
		s.debug.LogError(actorID, s.config.Name, err.Error(), duration)
		return core.Result{
			Duration: duration,
			Success:  false,
			Error:    err.Error(),
		}, err
	}

	// Substitute variables in headers
	headers, err := template.SubstituteMap(s.config.Headers, vars)
	if err != nil {
		duration := time.Since(start)
		s.debug.LogError(actorID, s.config.Name, err.Error(), duration)
		return core.Result{
			Duration: duration,
			Success:  false,
			Error:    err.Error(),
		}, err
	}

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

	for k, v := range headers {
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

	// Read body if needed for debug OR extraction
	needsExtract := len(s.config.Extract) > 0
	needsDebug := s.debug != nil
	var respBody []byte
	if needsExtract || needsDebug {
		// Use larger limit when extraction is needed
		limit := int64(maxDebugBodySize)
		if needsExtract {
			limit = maxExtractBodySize
		}
		respBody, _ = io.ReadAll(io.LimitReader(resp.Body, limit))
		_, _ = io.Copy(io.Discard, resp.Body) // drain remaining body
	} else {
		_, _ = io.Copy(io.Discard, resp.Body) // drain errors are ignorable
	}

	success := resp.StatusCode < 400
	errStr := ""
	if !success {
		errStr = resp.Status
	}

	// For debug logging, truncate body if needed
	debugBody := respBody
	if len(debugBody) > maxDebugBodySize {
		debugBody = debugBody[:maxDebugBodySize]
	}
	s.debug.LogResponse(actorID, s.config.Name, resp, debugBody, duration)

	// Extract variables from response (if extract rules defined and request succeeded)
	var extracted map[string]any
	if success && len(s.config.Extract) > 0 {
		extracted, err = template.Extract(respBody, s.config.Extract)
		if err != nil {
			success = false
			errStr = err.Error()
		}
	}

	return core.Result{
		Duration:   duration,
		Success:    success,
		Error:      errStr,
		StatusCode: resp.StatusCode,
		BytesSent:  int64(len(body)),
		BytesRecv:  int64(len(respBody)),
		Extract:    extracted,
	}, nil
}
