-- Sprint S020-1: Gateway Node Roles

-- Add node_roles capability flag to hosts
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS node_roles TEXT[] NOT NULL DEFAULT '{vm}';

-- Add created_at to gateway_nodes (missing from original schema in migration 000008)
ALTER TABLE gateway_nodes ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Fix: add FK constraint to gateway_nodes.host_id (skip if already exists)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'gateway_nodes_host_fk'
          AND table_name = 'gateway_nodes'
    ) THEN
        ALTER TABLE gateway_nodes ADD CONSTRAINT gateway_nodes_host_fk
            FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;
    END IF;
END $$;

-- Network-to-GW assignment: which gateway_node handles this network's Egress/Ingress
ALTER TABLE networks ADD COLUMN IF NOT EXISTS gateway_node_id UUID REFERENCES gateway_nodes(id) ON DELETE SET NULL;

-- IP pools for public IP management (needed by Ingress in S020-3)
CREATE TABLE IF NOT EXISTS ip_pools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    cidr CIDR NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
