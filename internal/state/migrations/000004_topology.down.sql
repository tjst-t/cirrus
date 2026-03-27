DROP INDEX IF EXISTS idx_locations_root_name;
DROP INDEX IF EXISTS idx_hosts_location;
DROP INDEX IF EXISTS idx_hosts_network_domain;

ALTER TABLE hosts
    DROP COLUMN IF EXISTS network_domain_id,
    DROP COLUMN IF EXISTS location_id;

ALTER TABLE host_storage_domains
    DROP CONSTRAINT IF EXISTS fk_host_storage_domains_storage;

DROP TABLE IF EXISTS locations;
DROP TABLE IF EXISTS network_domains;
DROP TABLE IF EXISTS storage_domains;
