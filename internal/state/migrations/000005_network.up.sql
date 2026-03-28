-- Sprint 5: Network — networks, subnets, ports

CREATE TABLE IF NOT EXISTS networks (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id),
    network_domain_id UUID NOT NULL REFERENCES network_domains(id),
    name              VARCHAR(63) NOT NULL,
    status            VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('creating', 'active', 'deleting', 'error')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_networks_tenant ON networks(tenant_id);
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

CREATE TABLE IF NOT EXISTS ports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    network_id  UUID NOT NULL REFERENCES networks(id),
    subnet_id   UUID NOT NULL REFERENCES subnets(id),
    vm_id       UUID,
    mac_address MACADDR NOT NULL UNIQUE,
    ip_address  INET NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'down' CHECK (status IN ('creating', 'down', 'active', 'deleting', 'error')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (subnet_id, ip_address)
);

CREATE INDEX IF NOT EXISTS idx_ports_tenant ON ports(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ports_network ON ports(network_id);
CREATE INDEX IF NOT EXISTS idx_ports_subnet ON ports(subnet_id);
CREATE INDEX IF NOT EXISTS idx_ports_vm ON ports(vm_id);
