ALTER TABLE drift_events
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS status;
