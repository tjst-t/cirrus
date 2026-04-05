ALTER TABLE tenants
    DROP COLUMN IF EXISTS quota_egresses,
    DROP COLUMN IF EXISTS quota_ingresses;

ALTER TABLE quota_usage
    DROP COLUMN IF EXISTS egresses_count,
    DROP COLUMN IF EXISTS ingresses_count;

ALTER TABLE quota_reserves
    DROP COLUMN IF EXISTS egresses,
    DROP COLUMN IF EXISTS ingresses;
