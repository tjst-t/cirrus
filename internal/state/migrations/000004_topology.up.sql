-- Sprint 4: Topology — storage domains, network domains, locations

CREATE TABLE IF NOT EXISTS storage_domains (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(63) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS network_domains (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(63) UNIQUE NOT NULL,
    ovn_nb_connection VARCHAR(255) NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS locations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id        UUID REFERENCES locations(id),
    name             VARCHAR(63) NOT NULL,
    type             VARCHAR(20) NOT NULL CHECK (type IN ('site', 'floor', 'row', 'rack', 'unit')),
    fault_attributes JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (parent_id, name)
);

CREATE INDEX IF NOT EXISTS idx_locations_parent ON locations(parent_id);
CREATE INDEX IF NOT EXISTS idx_locations_type ON locations(type);

-- UNIQUE (parent_id, name) does not cover NULL parent_id (SQL NULL != NULL),
-- so add a partial unique index for root-level locations.
CREATE UNIQUE INDEX IF NOT EXISTS idx_locations_root_name ON locations (name) WHERE parent_id IS NULL;

-- Add FK to existing host_storage_domains junction table
ALTER TABLE host_storage_domains
    ADD CONSTRAINT fk_host_storage_domains_storage
    FOREIGN KEY (storage_domain_id) REFERENCES storage_domains(id) ON DELETE CASCADE;

-- Add domain/location columns to hosts
ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS network_domain_id UUID REFERENCES network_domains(id),
    ADD COLUMN IF NOT EXISTS location_id UUID REFERENCES locations(id);

CREATE INDEX IF NOT EXISTS idx_hosts_network_domain ON hosts(network_domain_id);
CREATE INDEX IF NOT EXISTS idx_hosts_location ON hosts(location_id);
