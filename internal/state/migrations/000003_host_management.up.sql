-- Sprint 3: Host management — extend hosts table and add host_storage_domains

ALTER TABLE hosts
    ADD COLUMN IF NOT EXISTS capability JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS resource_physical JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS overcommit_ratios JSONB NOT NULL DEFAULT '{"vcpus": 4.0, "memory_mb": 1.5, "gpus": 1.0, "local_ssd_gb": 1.0}',
    ADD COLUMN IF NOT EXISTS resource_used JSONB NOT NULL DEFAULT '{"vcpus": 0, "memory_mb": 0}';

CREATE TABLE IF NOT EXISTS host_storage_domains (
    host_id           UUID NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    storage_domain_id UUID NOT NULL,
    PRIMARY KEY (host_id, storage_domain_id)
);

CREATE INDEX IF NOT EXISTS idx_host_storage_domains_storage ON host_storage_domains(storage_domain_id);
