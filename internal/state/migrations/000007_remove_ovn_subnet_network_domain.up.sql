-- Sprint 5N prep: Remove OVN/subnet/network_domain artifacts

-- Drop ports.subnet_id FK and subnets table
ALTER TABLE ports DROP COLUMN IF EXISTS subnet_id;
DROP INDEX IF EXISTS idx_ports_subnet;
DROP TABLE IF EXISTS subnets;
DROP INDEX IF EXISTS idx_subnets_network;

-- Remove network_domain_id from networks
ALTER TABLE networks DROP COLUMN IF EXISTS network_domain_id;
DROP INDEX IF EXISTS idx_networks_domain;

-- Remove network_domain_id from availability_zones
ALTER TABLE availability_zones DROP COLUMN IF EXISTS network_domain_id;

-- Remove network_domain_id from hosts
ALTER TABLE hosts DROP COLUMN IF EXISTS network_domain_id;
DROP INDEX IF EXISTS idx_hosts_network_domain;

-- Drop network_domains table
DROP TABLE IF EXISTS network_domains;
