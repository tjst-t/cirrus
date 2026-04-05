-- Sprint S020-3: Direct IP Ingress — add ip_pool_id and created_at to ingresses

ALTER TABLE ingresses
    ADD COLUMN IF NOT EXISTS ip_pool_id UUID REFERENCES ip_pools(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Ensure public_ip is unique across ingresses
CREATE UNIQUE INDEX IF NOT EXISTS ingresses_public_ip_unique ON ingresses (public_ip);
