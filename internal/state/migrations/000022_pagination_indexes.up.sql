-- S021-2-3: Add (created_at, id) indexes for cursor-based pagination on list endpoints.
-- Tables filtered by tenant_id use a 3-column index for better query plan efficiency.
CREATE INDEX IF NOT EXISTS idx_hosts_created_at_id        ON hosts(created_at, id);
CREATE INDEX IF NOT EXISTS idx_flavors_created_at_id      ON flavors(created_at, id);
CREATE INDEX IF NOT EXISTS idx_organizations_created_at_id ON organizations(created_at, id);
CREATE INDEX IF NOT EXISTS idx_tenants_created_at_id      ON tenants(organization_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_networks_created_at_id     ON networks(tenant_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_volumes_created_at_id      ON volumes(tenant_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_vms_created_at_id          ON vms(tenant_id, created_at, id);
