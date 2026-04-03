CREATE TABLE drift_events (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    layer       TEXT        NOT NULL,
    type        TEXT        NOT NULL,
    severity    TEXT        NOT NULL,
    resource    TEXT        NOT NULL,
    resource_id TEXT        NOT NULL,
    tenant_id   TEXT,
    host_id     TEXT,
    expected    TEXT,
    actual      TEXT,
    detected_by TEXT        NOT NULL,
    action      TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_drift_events_resource ON drift_events(resource_id, type, created_at DESC);
CREATE INDEX idx_drift_events_host     ON drift_events(host_id, created_at DESC) WHERE host_id IS NOT NULL;
CREATE INDEX idx_drift_events_created  ON drift_events(created_at DESC);
