package state

const schema = `
CREATE TABLE IF NOT EXISTS projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(63) NOT NULL UNIQUE,
    quota_vcpus INTEGER NOT NULL DEFAULT 20,
    quota_ram_mb INTEGER NOT NULL DEFAULT 51200,
    quota_vms   INTEGER NOT NULL DEFAULT 10,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id),
    name        VARCHAR(63) NOT NULL,
    key_hash    VARCHAR(128) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS workers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(63) NOT NULL UNIQUE,
    address     VARCHAR(255) NOT NULL,
    total_vcpus INTEGER NOT NULL,
    total_ram_mb INTEGER NOT NULL,
    total_disk_gb INTEGER NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'unknown',
    last_heartbeat TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS images (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(127) NOT NULL,
    project_id  UUID REFERENCES projects(id),
    format      VARCHAR(10) NOT NULL,
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    path        VARCHAR(512) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'creating',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS networks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id),
    name        VARCHAR(63) NOT NULL,
    cidr        CIDR NOT NULL,
    gateway     INET NOT NULL,
    vni         INTEGER NOT NULL UNIQUE,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS vms (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id),
    name        VARCHAR(63) NOT NULL,
    worker_id   UUID REFERENCES workers(id),
    image_id    UUID NOT NULL REFERENCES images(id),
    vcpus       INTEGER NOT NULL,
    ram_mb      INTEGER NOT NULL,
    disk_gb     INTEGER NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'scheduling',
    error_msg   TEXT,
    storage_data JSONB,
    compute_data JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  UUID NOT NULL REFERENCES projects(id),
    network_id  UUID NOT NULL REFERENCES networks(id),
    vm_id       UUID REFERENCES vms(id),
    mac_address MACADDR NOT NULL UNIQUE,
    ip_address  INET NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'down',
    network_data JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_vms_project ON vms(project_id);
CREATE INDEX IF NOT EXISTS idx_vms_worker ON vms(worker_id);
CREATE INDEX IF NOT EXISTS idx_vms_status ON vms(status);
CREATE INDEX IF NOT EXISTS idx_ports_vm ON ports(vm_id);
CREATE INDEX IF NOT EXISTS idx_ports_network ON ports(network_id);
CREATE INDEX IF NOT EXISTS idx_networks_project ON networks(project_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_vms_project_name ON vms(project_id, name) WHERE status != 'deleted';
CREATE UNIQUE INDEX IF NOT EXISTS idx_networks_project_name ON networks(project_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ports_network_ip ON ports(network_id, ip_address);
`
