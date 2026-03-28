-- Sprint 5.5: Availability Zones — tenant-facing placement abstraction

CREATE TABLE IF NOT EXISTS availability_zones (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(63) UNIQUE NOT NULL,
    description       TEXT,
    location_id       UUID NOT NULL REFERENCES locations(id),
    network_domain_id UUID NOT NULL UNIQUE REFERENCES network_domains(id),
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- AZ to Storage Domain mapping (N:M)
CREATE TABLE IF NOT EXISTS az_storage_domains (
    az_id             UUID NOT NULL REFERENCES availability_zones(id) ON DELETE CASCADE,
    storage_domain_id UUID NOT NULL REFERENCES storage_domains(id) ON DELETE CASCADE,
    PRIMARY KEY (az_id, storage_domain_id)
);
