CREATE TABLE IF NOT EXISTS vms (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name           VARCHAR(63) NOT NULL,
    flavor_id      UUID REFERENCES flavors(id),
    az_id          UUID,
    network_id     UUID,
    host_id        UUID REFERENCES hosts(id),
    status         VARCHAR(32) NOT NULL DEFAULT 'pending',
    error_message  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

CREATE TABLE IF NOT EXISTS vm_volumes (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id     UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    volume_id UUID NOT NULL,
    device    VARCHAR(16) NOT NULL DEFAULT 'vda'
);
