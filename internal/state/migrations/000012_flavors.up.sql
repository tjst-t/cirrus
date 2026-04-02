-- Sprint S015: Flavors — VM サイズのテンプレート

CREATE TABLE IF NOT EXISTS flavors (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       VARCHAR(63) UNIQUE NOT NULL,
    vcpus      INTEGER NOT NULL,
    ram_mb     BIGINT NOT NULL,
    disk_gb    BIGINT NOT NULL DEFAULT 0,
    is_public  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_flavors_public ON flavors(is_public);
