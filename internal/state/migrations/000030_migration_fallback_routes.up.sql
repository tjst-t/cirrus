-- migration_fallback_routes holds transient records created by MigrateVM
-- to instruct the source host to forward traffic for a migrating VM to the
-- destination host via Geneve tunnel (zero-packet-loss live migration).
-- Records are deleted by MigrateVM after migration completes.
CREATE TABLE migration_fallback_routes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    port_id     UUID NOT NULL REFERENCES ports(id) ON DELETE CASCADE,
    src_host_id UUID NOT NULL REFERENCES hosts(id),
    dest_host_id UUID NOT NULL REFERENCES hosts(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_migration_fallback_routes_src_host ON migration_fallback_routes (src_host_id);
CREATE INDEX idx_migration_fallback_routes_port ON migration_fallback_routes (port_id);
