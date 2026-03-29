-- Reverse of Sprint 5N prep cleanup (best-effort, data is lost)

CREATE TABLE IF NOT EXISTS network_domains (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              VARCHAR(63) UNIQUE NOT NULL,
    ovn_nb_connection VARCHAR(255) NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE hosts ADD COLUMN IF NOT EXISTS network_domain_id UUID REFERENCES network_domains(id);
CREATE INDEX IF NOT EXISTS idx_hosts_network_domain ON hosts(network_domain_id);

ALTER TABLE availability_zones ADD COLUMN IF NOT EXISTS network_domain_id UUID UNIQUE REFERENCES network_domains(id);

ALTER TABLE networks ADD COLUMN IF NOT EXISTS network_domain_id UUID REFERENCES network_domains(id);
CREATE INDEX IF NOT EXISTS idx_networks_domain ON networks(network_domain_id);

CREATE TABLE IF NOT EXISTS subnets (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    network_id       UUID NOT NULL REFERENCES networks(id) ON DELETE CASCADE,
    cidr             CIDR NOT NULL,
    gateway          INET NOT NULL,
    dhcp_range_start INET NOT NULL,
    dhcp_range_end   INET NOT NULL,
    dns_servers      INET[] NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_subnets_network ON subnets(network_id);

ALTER TABLE ports ADD COLUMN IF NOT EXISTS subnet_id UUID REFERENCES subnets(id);
CREATE INDEX IF NOT EXISTS idx_ports_subnet ON ports(subnet_id);
