package state

import (
	"context"
	"testing"
	"time"
)

func TestCreateTemplate(t *testing.T) {
	tests := []struct {
		name        string
		tmplName    string
		description string
		durationMs  int64
		failureRate float64
		wantErr     bool
	}{
		{
			name:        "valid template",
			tmplName:    "host-provision",
			description: "Provision a host",
			durationMs:  30000,
			failureRate: 0.0,
		},
		{
			name:        "template with failure rate",
			tmplName:    "deploy",
			description: "Deploy app",
			durationMs:  5000,
			failureRate: 0.1,
		},
		{
			name:    "empty name",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStore()
			ctx := context.Background()

			tmpl, err := s.CreateTemplate(ctx, tt.tmplName, tt.description, tt.durationMs, tt.failureRate)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tmpl.Name != tt.tmplName {
				t.Errorf("name = %q, want %q", tmpl.Name, tt.tmplName)
			}
			if tmpl.ID < 1 {
				t.Errorf("expected positive ID, got %d", tmpl.ID)
			}
		})
	}
}

func TestGetTemplate(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	tmpl, err := s.CreateTemplate(ctx, "test", "desc", 1000, 0.0)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tests := []struct {
		name    string
		id      int64
		wantErr bool
	}{
		{name: "existing", id: tmpl.ID},
		{name: "not found", id: 999, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.GetTemplate(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ID != tmpl.ID {
				t.Errorf("ID = %d, want %d", got.ID, tmpl.ID)
			}
		})
	}
}

func TestListTemplates(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	if got := s.ListTemplates(ctx); len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}

	if _, err := s.CreateTemplate(ctx, "t1", "", 100, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTemplate(ctx, "t2", "", 200, 0); err != nil {
		t.Fatal(err)
	}

	got := s.ListTemplates(ctx)
	if len(got) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(got))
	}
}

func TestLaunchJobAndCompletion(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	tmpl, err := s.CreateTemplate(ctx, "fast-job", "completes quickly", 10, 0.0)
	if err != nil {
		t.Fatalf("create template: %v", err)
	}

	extras := map[string]interface{}{"host_id": "host-1"}
	job, err := s.LaunchJob(ctx, tmpl.ID, extras)
	if err != nil {
		t.Fatalf("launch job: %v", err)
	}

	if job.Status != "running" {
		t.Errorf("initial status = %q, want %q", job.Status, "running")
	}
	if job.Started == nil {
		t.Error("expected Started to be set")
	}

	// Wait for completion.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := s.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if got.Status == "successful" {
			if got.Finished == nil {
				t.Error("expected Finished to be set")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not complete within deadline")
}

func TestLaunchJobTemplateNotFound(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	_, err := s.LaunchJob(ctx, 999, nil)
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestLaunchJobWithFailureRate(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	// 100% failure rate.
	tmpl, err := s.CreateTemplate(ctx, "always-fail", "", 10, 1.0)
	if err != nil {
		t.Fatal(err)
	}

	job, err := s.LaunchJob(ctx, tmpl.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := s.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "failed" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not fail within deadline")
}

func TestCancelJob(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	// Long running job so we can cancel it.
	tmpl, err := s.CreateTemplate(ctx, "long-job", "", 60000, 0.0)
	if err != nil {
		t.Fatal(err)
	}

	job, err := s.LaunchJob(ctx, tmpl.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		id      int64
		wantErr bool
	}{
		{name: "cancel running job", id: job.ID},
		{name: "cancel already canceled", id: job.ID, wantErr: true},
		{name: "cancel not found", id: 999, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.CancelJob(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Status != "canceled" {
				t.Errorf("status = %q, want %q", got.Status, "canceled")
			}
			if got.Finished == nil {
				t.Error("expected Finished to be set")
			}
		})
	}
}

func TestCancelCompletedJob(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	tmpl, err := s.CreateTemplate(ctx, "fast", "", 10, 0.0)
	if err != nil {
		t.Fatal(err)
	}

	job, err := s.LaunchJob(ctx, tmpl.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for completion.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err := s.GetJob(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "successful" || got.Status == "failed" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	_, err = s.CancelJob(ctx, job.ID)
	if err == nil {
		t.Fatal("expected error canceling completed job")
	}
}

func TestCallbackConfig(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	cfg := CallbackConfig{
		Enabled:     true,
		CallbackURL: "http://localhost:8080/callback",
		AuthToken:   "test-token",
	}
	s.SetCallback(ctx, cfg)

	got := s.GetCallback(ctx)
	if got.Enabled != cfg.Enabled {
		t.Errorf("Enabled = %v, want %v", got.Enabled, cfg.Enabled)
	}
	if got.CallbackURL != cfg.CallbackURL {
		t.Errorf("CallbackURL = %q, want %q", got.CallbackURL, cfg.CallbackURL)
	}
	if got.AuthToken != cfg.AuthToken {
		t.Errorf("AuthToken = %q, want %q", got.AuthToken, cfg.AuthToken)
	}
}

func TestStats(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	stats := s.GetStats(ctx)
	if stats.Running != 0 || stats.Successful != 0 || stats.Failed != 0 || stats.Canceled != 0 {
		t.Fatalf("expected all zeros, got %+v", stats)
	}

	// Create a long-running template and launch a job.
	tmpl, err := s.CreateTemplate(ctx, "long", "", 60000, 0.0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.LaunchJob(ctx, tmpl.ID, nil)
	if err != nil {
		t.Fatal(err)
	}

	stats = s.GetStats(ctx)
	if stats.Running != 1 {
		t.Errorf("Running = %d, want 1", stats.Running)
	}
}

func TestReset(t *testing.T) {
	s := NewStore()
	ctx := context.Background()

	tmpl, err := s.CreateTemplate(ctx, "t1", "", 60000, 0.0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LaunchJob(ctx, tmpl.ID, nil); err != nil {
		t.Fatal(err)
	}

	s.SetCallback(ctx, CallbackConfig{Enabled: true, CallbackURL: "http://x", AuthToken: "tok"})
	s.Reset(ctx)

	if len(s.ListTemplates(ctx)) != 0 {
		t.Error("expected no templates after reset")
	}
	stats := s.GetStats(ctx)
	if stats.Running != 0 {
		t.Errorf("Running = %d after reset, want 0", stats.Running)
	}
	cb := s.GetCallback(ctx)
	if cb.Enabled {
		t.Error("expected callback disabled after reset")
	}
}
