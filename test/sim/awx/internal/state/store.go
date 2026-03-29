// Package state provides thread-safe in-memory storage for AWX simulator state.
package state

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"encoding/json"
)

// JobTemplate represents an AWX job template.
type JobTemplate struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	ExpectedDurationMs int64  `json:"expected_duration_ms"`
	FailureRate        float64 `json:"failure_rate"`
}

// Job represents an AWX job instance.
type Job struct {
	ID            int64                  `json:"id"`
	JobTemplate   int64                  `json:"job_template"`
	Status        string                 `json:"status"`
	ExtraVars     map[string]interface{} `json:"extra_vars"`
	Created       time.Time              `json:"created"`
	Started       *time.Time             `json:"started"`
	Finished      *time.Time             `json:"finished"`
}

// CallbackConfig holds the callback configuration for job completion notifications.
type CallbackConfig struct {
	Enabled     bool   `json:"enabled"`
	CallbackURL string `json:"callback_url"`
	AuthToken   string `json:"auth_token"`
}

// Stats holds aggregated job statistics.
type Stats struct {
	Pending    int `json:"pending"`
	Running    int `json:"running"`
	Successful int `json:"successful"`
	Failed     int `json:"failed"`
	Canceled   int `json:"canceled"`
}

// Store provides thread-safe in-memory storage for AWX simulator state.
type Store struct {
	mu             sync.RWMutex
	templates      map[int64]*JobTemplate
	jobs           map[int64]*Job
	nextTemplateID int64
	nextJobID      int64
	callback       CallbackConfig
	httpClient     *http.Client
	timers         []*time.Timer
}

// NewStore creates a new Store with initialized maps.
func NewStore() *Store {
	return &Store{
		templates:      make(map[int64]*JobTemplate),
		jobs:           make(map[int64]*Job),
		nextTemplateID: 1,
		nextJobID:      1,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateTemplate adds a new job template and returns it.
func (s *Store) CreateTemplate(_ context.Context, name, description string, durationMs int64, failureRate float64) (*JobTemplate, error) {
	if name == "" {
		return nil, fmt.Errorf("create template: name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t := &JobTemplate{
		ID:                 s.nextTemplateID,
		Name:               name,
		Description:        description,
		ExpectedDurationMs: durationMs,
		FailureRate:        failureRate,
	}
	s.templates[t.ID] = t
	s.nextTemplateID++

	return t, nil
}

// GetTemplate retrieves a job template by ID.
func (s *Store) GetTemplate(_ context.Context, id int64) (*JobTemplate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.templates[id]
	if !ok {
		return nil, fmt.Errorf("get template: template %d not found", id)
	}

	return t, nil
}

// ListTemplates returns all job templates.
func (s *Store) ListTemplates(_ context.Context) []*JobTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*JobTemplate, 0, len(s.templates))
	for _, t := range s.templates {
		result = append(result, t)
	}

	return result
}

// LaunchJob creates a new job from a template and starts async execution.
func (s *Store) LaunchJob(ctx context.Context, templateID int64, extraVars map[string]interface{}) (*Job, error) {
	s.mu.Lock()

	t, ok := s.templates[templateID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("launch job: template %d not found", templateID)
	}

	now := time.Now().UTC()
	job := &Job{
		ID:          s.nextJobID,
		JobTemplate: templateID,
		Status:      "pending",
		ExtraVars:   extraVars,
		Created:     now,
	}
	s.jobs[job.ID] = job
	s.nextJobID++

	duration := time.Duration(t.ExpectedDurationMs) * time.Millisecond
	failureRate := t.FailureRate
	jobID := job.ID
	tmplID := t.ID

	s.mu.Unlock()

	// Transition to running immediately.
	s.mu.Lock()
	if job.Status == "pending" {
		job.Status = "running"
		started := time.Now().UTC()
		job.Started = &started
	}
	s.mu.Unlock()

	// Schedule completion after expected duration.
	timer := time.AfterFunc(duration, func() {
		s.completeJob(ctx, jobID, tmplID, failureRate)
	})

	s.mu.Lock()
	s.timers = append(s.timers, timer)
	s.mu.Unlock()

	return job, nil
}

// completeJob transitions a running job to successful or failed.
func (s *Store) completeJob(ctx context.Context, jobID, templateID int64, failureRate float64) {
	s.mu.Lock()

	job, ok := s.jobs[jobID]
	if !ok {
		s.mu.Unlock()
		return
	}

	if job.Status != "running" {
		s.mu.Unlock()
		return
	}

	now := time.Now().UTC()
	job.Finished = &now

	//nolint:gosec // G404: use of weak random is acceptable for simulator failure rate
	if rand.Float64() < failureRate {
		job.Status = "failed"
	} else {
		job.Status = "successful"
	}

	status := job.Status
	extraVars := job.ExtraVars
	finished := now
	cb := s.callback

	s.mu.Unlock()

	if cb.Enabled && cb.CallbackURL != "" {
		s.sendCallback(ctx, cb, jobID, templateID, status, extraVars, finished)
	}
}

// sendCallback sends a job completion notification to the configured callback URL.
func (s *Store) sendCallback(ctx context.Context, cb CallbackConfig, jobID, templateID int64, status string, extraVars map[string]interface{}, finished time.Time) {
	payload := map[string]interface{}{
		"job_id":          jobID,
		"job_template_id": templateID,
		"status":          status,
		"extra_vars":      extraVars,
		"finished":        finished.Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.WarnContext(ctx, "failed to marshal callback payload", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cb.CallbackURL, strings.NewReader(string(body)))
	if err != nil {
		slog.WarnContext(ctx, "failed to create callback request", "error", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cb.AuthToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		slog.WarnContext(ctx, "callback request failed", "error", err, "url", cb.CallbackURL)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.WarnContext(ctx, "failed to close callback response body", "error", closeErr)
		}
	}()

	slog.InfoContext(ctx, "callback sent", "job_id", jobID, "status", status, "response_code", resp.StatusCode)
}

// GetJob retrieves a job by ID.
func (s *Store) GetJob(_ context.Context, id int64) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("get job: job %d not found", id)
	}

	return job, nil
}

// CancelJob cancels a pending or running job.
func (s *Store) CancelJob(_ context.Context, id int64) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return nil, fmt.Errorf("cancel job: job %d not found", id)
	}

	if job.Status != "pending" && job.Status != "running" {
		return nil, fmt.Errorf("cancel job: job %d is in %q state and cannot be canceled", id, job.Status)
	}

	now := time.Now().UTC()
	job.Status = "canceled"
	job.Finished = &now

	return job, nil
}

// SetCallback configures the callback settings.
func (s *Store) SetCallback(_ context.Context, cfg CallbackConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callback = cfg
}

// GetCallback returns the current callback configuration.
func (s *Store) GetCallback(_ context.Context) CallbackConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.callback
}

// GetStats returns aggregated job statistics.
func (s *Store) GetStats(_ context.Context) Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats Stats
	for _, job := range s.jobs {
		switch job.Status {
		case "pending":
			stats.Pending++
		case "running":
			stats.Running++
		case "successful":
			stats.Successful++
		case "failed":
			stats.Failed++
		case "canceled":
			stats.Canceled++
		}
	}

	return stats
}

// Reset clears all state and stops pending timers.
func (s *Store) Reset(_ context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, timer := range s.timers {
		timer.Stop()
	}

	s.templates = make(map[int64]*JobTemplate)
	s.jobs = make(map[int64]*Job)
	s.nextTemplateID = 1
	s.nextJobID = 1
	s.callback = CallbackConfig{}
	s.timers = nil
}
