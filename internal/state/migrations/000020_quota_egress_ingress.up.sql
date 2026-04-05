-- Add egresses and ingresses count columns to quota tables.

ALTER TABLE quota_usage
    ADD COLUMN IF NOT EXISTS egresses_count  INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ingresses_count INT NOT NULL DEFAULT 0;

ALTER TABLE quota_reserves
    ADD COLUMN IF NOT EXISTS egresses  INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ingresses INT NOT NULL DEFAULT 0;

-- Add quota limit columns to tenants table.
ALTER TABLE tenants
    ADD COLUMN IF NOT EXISTS quota_egresses  INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS quota_ingresses INT NOT NULL DEFAULT 0;
