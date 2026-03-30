-- Sprint 5N-c: Add fabric_ip to hosts for Geneve tunnel endpoints.
-- This is the IP address used for overlay network tunnels between worker hosts.
ALTER TABLE hosts ADD COLUMN fabric_ip TEXT NOT NULL DEFAULT '';
