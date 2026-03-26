DROP TABLE IF EXISTS host_storage_domains;

ALTER TABLE hosts
    DROP COLUMN IF EXISTS capability,
    DROP COLUMN IF EXISTS resource_physical,
    DROP COLUMN IF EXISTS overcommit_ratios,
    DROP COLUMN IF EXISTS resource_used;
