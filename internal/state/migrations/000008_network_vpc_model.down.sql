-- Reverse Sprint 5N-a: VPC model

DROP TABLE IF EXISTS gateway_nodes;
DROP TABLE IF EXISTS ingresses;
DROP TABLE IF EXISTS egresses;
DROP TABLE IF EXISTS policies;

-- Remove ports constraints and columns
ALTER TABLE ports DROP CONSTRAINT IF EXISTS ports_vm_role_unique;
ALTER TABLE ports DROP CONSTRAINT IF EXISTS ports_network_mac_unique;
ALTER TABLE ports DROP CONSTRAINT IF EXISTS ports_network_ip_unique;
ALTER TABLE ports DROP CONSTRAINT IF EXISTS ports_group_id_fk;
ALTER TABLE ports DROP COLUMN IF EXISTS role;
ALTER TABLE ports DROP COLUMN IF EXISTS host_id;
ALTER TABLE ports DROP COLUMN IF EXISTS group_id;
ALTER TABLE ports ADD CONSTRAINT ports_mac_address_key UNIQUE (mac_address);

DROP TABLE IF EXISTS groups;

-- Remove VNI sequence and network columns
DROP SEQUENCE IF EXISTS networks_vni_seq;
ALTER TABLE networks DROP CONSTRAINT IF EXISTS networks_tenant_cidr_unique;
ALTER TABLE networks DROP CONSTRAINT IF EXISTS networks_vni_unique;
ALTER TABLE networks DROP COLUMN IF EXISTS vni;
ALTER TABLE networks DROP COLUMN IF EXISTS cidr;
