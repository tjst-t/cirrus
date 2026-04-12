package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const awxSchemaSQL = `
CREATE TABLE IF NOT EXISTS awx_sim_templates (
	id                   BIGINT  PRIMARY KEY,
	name                 TEXT    NOT NULL,
	description          TEXT    NOT NULL DEFAULT '',
	expected_duration_ms BIGINT  NOT NULL DEFAULT 0,
	failure_rate         FLOAT   NOT NULL DEFAULT 0.0
);

CREATE TABLE IF NOT EXISTS awx_sim_jobs (
	id           BIGINT      PRIMARY KEY,
	job_template BIGINT      NOT NULL,
	status       TEXT        NOT NULL,
	extra_vars   JSONB       NOT NULL DEFAULT '{}',
	created      TIMESTAMPTZ NOT NULL,
	started      TIMESTAMPTZ,
	finished     TIMESTAMPTZ
);
`

// SetupSchema creates the AWX sim tables if they don't exist.
func SetupSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, awxSchemaSQL); err != nil {
		return fmt.Errorf("awx sim: setup schema: %w", err)
	}
	return nil
}

// SetDB attaches a postgres pool for persistence. Must be called before LoadFromDB.
func (s *Store) SetDB(pool *pgxpool.Pool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = pool
}

// LoadFromDB loads templates and jobs from the database into the in-memory maps.
// Jobs that were pending or running at shutdown are marked "failed" since their
// timers cannot be resumed.
func (s *Store) LoadFromDB(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}

	// ── Load templates ──────────────────────────────────────────────────────
	rows, err := s.db.Query(ctx,
		`SELECT id, name, description, expected_duration_ms, failure_rate FROM awx_sim_templates`)
	if err != nil {
		return fmt.Errorf("load templates: %w", err)
	}
	defer rows.Close()

	maxTemplateID := int64(0)
	for rows.Next() {
		var t JobTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.ExpectedDurationMs, &t.FailureRate); err != nil {
			return fmt.Errorf("scan template: %w", err)
		}
		stored := t
		s.templates[t.ID] = &stored
		if t.ID >= maxTemplateID {
			maxTemplateID = t.ID + 1
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load templates rows: %w", err)
	}
	if maxTemplateID > s.nextTemplateID {
		s.nextTemplateID = maxTemplateID
	}
	rows.Close()

	// ── Load jobs ───────────────────────────────────────────────────────────
	rows, err = s.db.Query(ctx,
		`SELECT id, job_template, status, extra_vars, created, started, finished FROM awx_sim_jobs`)
	if err != nil {
		return fmt.Errorf("load jobs: %w", err)
	}
	defer rows.Close()

	maxJobID := int64(0)
	now := time.Now().UTC()
	for rows.Next() {
		var j Job
		var extraJSON []byte
		if err := rows.Scan(
			&j.ID, &j.JobTemplate, &j.Status, &extraJSON, &j.Created, &j.Started, &j.Finished,
		); err != nil {
			return fmt.Errorf("scan job: %w", err)
		}
		if extraJSON != nil {
			json.Unmarshal(extraJSON, &j.ExtraVars) //nolint:errcheck
		}
		if j.ExtraVars == nil {
			j.ExtraVars = map[string]interface{}{}
		}
		// In-flight jobs can't be resumed — mark them failed.
		if j.Status == "pending" || j.Status == "running" {
			j.Status = "failed"
			j.Finished = &now
		}
		stored := j
		s.jobs[j.ID] = &stored
		if j.ID >= maxJobID {
			maxJobID = j.ID + 1
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load jobs rows: %w", err)
	}
	if maxJobID > s.nextJobID {
		s.nextJobID = maxJobID
	}

	slog.Info("awx-sim: state loaded from DB",
		"templates", len(s.templates),
		"jobs", len(s.jobs),
	)
	return nil
}

// dbSaveTemplate upserts a job template.
func (s *Store) dbSaveTemplate(t *JobTemplate) {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `
		INSERT INTO awx_sim_templates (id, name, description, expected_duration_ms, failure_rate)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (id) DO UPDATE SET
			name                 = EXCLUDED.name,
			description          = EXCLUDED.description,
			expected_duration_ms = EXCLUDED.expected_duration_ms,
			failure_rate         = EXCLUDED.failure_rate`,
		t.ID, t.Name, t.Description, t.ExpectedDurationMs, t.FailureRate,
	)
	if err != nil {
		slog.Error("awx-sim: persist template", "id", t.ID, "error", err)
	}
}

// dbSaveJob upserts a job.
func (s *Store) dbSaveJob(j *Job) {
	if s.db == nil {
		return
	}
	extraJSON, _ := json.Marshal(j.ExtraVars)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.Exec(ctx, `
		INSERT INTO awx_sim_jobs (id, job_template, status, extra_vars, created, started, finished)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET
			job_template = EXCLUDED.job_template,
			status       = EXCLUDED.status,
			extra_vars   = EXCLUDED.extra_vars,
			created      = EXCLUDED.created,
			started      = EXCLUDED.started,
			finished     = EXCLUDED.finished`,
		j.ID, j.JobTemplate, j.Status, extraJSON, j.Created, j.Started, j.Finished,
	)
	if err != nil {
		slog.Error("awx-sim: persist job", "id", j.ID, "error", err)
	}
}

// dbClearAll truncates all AWX sim tables.
func (s *Store) dbClearAll() {
	if s.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := s.db.Exec(ctx, `TRUNCATE awx_sim_templates, awx_sim_jobs`); err != nil {
		slog.Error("awx-sim: clear tables", "error", err)
	}
}
