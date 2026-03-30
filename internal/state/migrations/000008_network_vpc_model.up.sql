-- Sprint 5N-a: VPC model (Network cidr/vni, Groups, Policies, Egresses, Ingresses, Gateway Nodes)

-- 1. Networks: add cidr and vni columns
ALTER TABLE networks ADD COLUMN cidr CIDR;
ALTER TABLE networks ADD COLUMN vni INTEGER;

-- Backfill existing networks with sequential VNI and default CIDR (/22 = 1024 addrs, 3rd octet increments by 4)
DO $$
DECLARE
    r RECORD;
    seq INTEGER := 1;
    third_octet INTEGER;
    second_octet INTEGER;
BEGIN
    FOR r IN SELECT id FROM networks ORDER BY created_at LOOP
        -- Each /22 block spans 4 values in the 3rd octet
        third_octet := ((seq - 1) * 4) % 256;
        second_octet := 64 + (((seq - 1) * 4) / 256);
        UPDATE networks
        SET vni = seq,
            cidr = ('100.' || second_octet || '.' || third_octet || '.0/22')::CIDR
        WHERE id = r.id;
        seq := seq + 1;
    END LOOP;
END $$;

ALTER TABLE networks ALTER COLUMN cidr SET NOT NULL;
ALTER TABLE networks ALTER COLUMN vni SET NOT NULL;
ALTER TABLE networks ADD CONSTRAINT networks_vni_unique UNIQUE (vni);
ALTER TABLE networks ADD CONSTRAINT networks_tenant_cidr_unique UNIQUE (tenant_id, cidr);

-- VNI sequence for auto-allocation
CREATE SEQUENCE IF NOT EXISTS networks_vni_seq START WITH 1;
-- Set sequence to max existing VNI; is_called=false when no data so first nextval returns 1
SELECT setval('networks_vni_seq',
    GREATEST(COALESCE((SELECT MAX(vni) FROM networks), 0), 1),
    COALESCE((SELECT MAX(vni) FROM networks), 0) > 0);

-- 2. Groups table
CREATE TABLE groups (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID NOT NULL REFERENCES networks(id),
    name TEXT NOT NULL,
    UNIQUE(network_id, name)
);
CREATE INDEX idx_groups_network ON groups(network_id);

-- 3. Ports: add new columns and update constraints
ALTER TABLE ports DROP CONSTRAINT IF EXISTS ports_mac_address_key;

ALTER TABLE ports ADD COLUMN group_id UUID;
ALTER TABLE ports ADD COLUMN host_id UUID;
ALTER TABLE ports ADD COLUMN role TEXT NOT NULL DEFAULT 'default';

ALTER TABLE ports ADD CONSTRAINT ports_group_id_fk FOREIGN KEY (group_id) REFERENCES groups(id);
ALTER TABLE ports ADD CONSTRAINT ports_network_ip_unique UNIQUE (network_id, ip_address);
ALTER TABLE ports ADD CONSTRAINT ports_network_mac_unique UNIQUE (network_id, mac_address);
-- NULLs are distinct in PostgreSQL UNIQUE: unattached ports (vm_id=NULL) are unconstrained;
-- only attached ports enforce one port per (vm_id, role).
ALTER TABLE ports ADD CONSTRAINT ports_vm_role_unique UNIQUE (vm_id, role);

-- 4. Policies table
CREATE TABLE policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID NOT NULL REFERENCES networks(id),
    src_group_id UUID NOT NULL REFERENCES groups(id),
    dst_group_id UUID NOT NULL REFERENCES groups(id),
    protocol TEXT NOT NULL,
    dst_port INTEGER,
    priority INTEGER NOT NULL DEFAULT 1000,
    action TEXT NOT NULL DEFAULT 'allow',
    UNIQUE(network_id, src_group_id, dst_group_id, protocol, dst_port)
);
CREATE INDEX idx_policies_network ON policies(network_id);

-- 5. Egresses table
CREATE TABLE egresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID NOT NULL REFERENCES networks(id),
    type TEXT NOT NULL,
    config JSONB NOT NULL
);

-- 6. Ingresses table
CREATE TABLE ingresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id UUID NOT NULL REFERENCES networks(id),
    type TEXT NOT NULL,
    public_ip INET NOT NULL,
    config JSONB NOT NULL
);

-- 7. Gateway nodes table
CREATE TABLE gateway_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id UUID NOT NULL,
    external_ip INET NOT NULL,
    internal_ip INET NOT NULL,
    status TEXT NOT NULL DEFAULT 'active'
);
