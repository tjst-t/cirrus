-- quota_usage tracks committed resource consumption per tenant.
CREATE TABLE IF NOT EXISTS quota_usage (
    tenant_id      UUID PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    vcpus_used     INT NOT NULL DEFAULT 0,
    ram_mb_used    INT NOT NULL DEFAULT 0,
    volume_gb_used INT NOT NULL DEFAULT 0,
    vms_count      INT NOT NULL DEFAULT 0,
    volumes_count  INT NOT NULL DEFAULT 0,
    snapshots_count INT NOT NULL DEFAULT 0,
    networks_count INT NOT NULL DEFAULT 0,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- quota_reserves tracks in-flight reservations (between Reserve and Commit/Release).
CREATE TABLE IF NOT EXISTS quota_reserves (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    resource_type   VARCHAR(50) NOT NULL,
    resource_id     UUID NOT NULL,
    vcpus           INT NOT NULL DEFAULT 0,
    ram_mb          INT NOT NULL DEFAULT 0,
    volume_gb       INT NOT NULL DEFAULT 0,
    vms             INT NOT NULL DEFAULT 0,
    volumes         INT NOT NULL DEFAULT 0,
    snapshots       INT NOT NULL DEFAULT 0,
    networks        INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_quota_reserves_tenant ON quota_reserves(tenant_id);
CREATE UNIQUE INDEX idx_quota_reserves_resource ON quota_reserves(resource_type, resource_id);
