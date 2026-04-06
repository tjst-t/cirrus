-- S045-1-1: Create jobs table for async job queue.
CREATE TABLE IF NOT EXISTS jobs (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    type         TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    payload      JSONB,
    tenant_id    UUID,
    created_by   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error        TEXT
);

-- Efficient polling: find next pending job ordered by creation time.
CREATE INDEX IF NOT EXISTS idx_jobs_status_created_at ON jobs(status, created_at);

-- Authorization queries: find jobs by tenant / creator.
CREATE INDEX IF NOT EXISTS idx_jobs_tenant_created_by ON jobs(tenant_id, created_by);
