-- Revert Sprint S020-3 ingress changes
DROP INDEX IF EXISTS ingresses_public_ip_unique;

ALTER TABLE ingresses
    DROP COLUMN IF EXISTS ip_pool_id,
    DROP COLUMN IF EXISTS created_at;
