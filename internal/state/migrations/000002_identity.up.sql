CREATE TABLE IF NOT EXISTS organizations (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           VARCHAR(255) NOT NULL UNIQUE,
    quota_vcpus    INT NOT NULL DEFAULT 0,
    quota_ram_mb   INT NOT NULL DEFAULT 0,
    quota_volume_gb INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenants (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    quota_vcpus     INT NOT NULL DEFAULT 0,
    quota_ram_mb    INT NOT NULL DEFAULT 0,
    quota_volume_gb INT NOT NULL DEFAULT 0,
    quota_vms       INT NOT NULL DEFAULT 0,
    quota_volumes   INT NOT NULL DEFAULT 0,
    quota_snapshots INT NOT NULL DEFAULT 0,
    quota_networks  INT NOT NULL DEFAULT 0,
    quota_floating_ips INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id VARCHAR(255) NOT NULL UNIQUE,
    name        VARCHAR(255) NOT NULL DEFAULT '',
    email       VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS role_assignments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope_type VARCHAR(50) NOT NULL CHECK (scope_type IN ('global', 'organization', 'tenant')),
    scope_id   UUID,
    role       VARCHAR(50) NOT NULL CHECK (role IN ('infra_admin', 'org_admin', 'tenant_admin', 'tenant_member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, scope_type, scope_id, role)
);

CREATE INDEX idx_role_assignments_user_id ON role_assignments(user_id);
CREATE INDEX idx_role_assignments_scope ON role_assignments(scope_type, scope_id);
CREATE INDEX idx_tenants_organization_id ON tenants(organization_id);
