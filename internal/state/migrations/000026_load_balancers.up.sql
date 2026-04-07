-- Migration 000026: load_balancers table for internal (tenant-network) L4 LB
-- VIP is auto-allocated from the network CIDR (no IP pool required).
-- OVS select groups are installed on ALL hosts in the network (not just GW nodes).

CREATE TABLE IF NOT EXISTS load_balancers (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(id),
    network_id  UUID        NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    vip         INET        NOT NULL,
    config      JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (network_id, vip),
    UNIQUE (network_id, name)
);

CREATE INDEX IF NOT EXISTS idx_load_balancers_network ON load_balancers(network_id);
CREATE INDEX IF NOT EXISTS idx_load_balancers_tenant  ON load_balancers(tenant_id);

CREATE TABLE IF NOT EXISTS lb_backend_health (
    lb_id           UUID        NOT NULL REFERENCES load_balancers(id) ON DELETE CASCADE,
    vm_id           UUID        NOT NULL,
    healthy         BOOLEAN     NOT NULL DEFAULT true,
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (lb_id, vm_id)
);

CREATE INDEX IF NOT EXISTS idx_lb_backend_health_lb ON lb_backend_health(lb_id);
