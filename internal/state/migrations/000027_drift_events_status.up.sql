-- Migration 000027: add status and resolved_at to drift_events
ALTER TABLE drift_events
    ADD COLUMN IF NOT EXISTS status       TEXT        NOT NULL DEFAULT 'open',
    ADD COLUMN IF NOT EXISTS resolved_at  TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_drift_events_status ON drift_events(status, created_at DESC);
