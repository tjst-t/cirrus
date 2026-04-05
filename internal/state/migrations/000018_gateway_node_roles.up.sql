-- Sprint S020-1: Gateway Node Roles

-- Add node_roles capability flag to hosts
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS node_roles TEXT[] NOT NULL DEFAULT '{vm}';

-- Fix: add FK constraint to gateway_nodes.host_id
ALTER TABLE gateway_nodes ADD CONSTRAINT IF NOT EXISTS gateway_nodes_host_fk
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE;

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
