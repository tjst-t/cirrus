-- Migration 000025: l4lb_backend_health table
-- Tracks per-backend health state for l4_lb ingress rules.
-- The full l4_lb config (backends, ports, etc.) is stored in ingresses.config JSONB.
-- This table is updated atomically when the Worker Agent reports health check results.

CREATE TABLE IF NOT EXISTS l4lb_backend_health (
    ingress_id      UUID        NOT NULL REFERENCES ingresses(id) ON DELETE CASCADE,
    vm_id           UUID        NOT NULL,
    healthy         BOOLEAN     NOT NULL DEFAULT true,
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (ingress_id, vm_id)
);

CREATE INDEX IF NOT EXISTS idx_l4lb_backend_health_ingress ON l4lb_backend_health(ingress_id);
