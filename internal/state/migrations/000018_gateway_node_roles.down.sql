-- Sprint S020-1: Gateway Node Roles (down)

DROP TABLE IF EXISTS ip_pools;

ALTER TABLE networks DROP COLUMN IF EXISTS gateway_node_id;

ALTER TABLE gateway_nodes DROP CONSTRAINT IF EXISTS gateway_nodes_host_fk;

ALTER TABLE gateway_nodes DROP COLUMN IF EXISTS created_at;

ALTER TABLE hosts DROP COLUMN IF EXISTS node_roles;
