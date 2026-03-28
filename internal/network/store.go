package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tjst-t/cirrus/internal/network/ipam"
	"github.com/tjst-t/cirrus/internal/network/ovn"
)

// Store implements Service using PostgreSQL, OVN, and IPAM.
type Store struct {
	pool   *pgxpool.Pool
	ovn    ovn.Client
	ipam   ipam.IPAM
	logger *slog.Logger
}

// NewStore creates a new network store.
func NewStore(pool *pgxpool.Pool, ovnClient ovn.Client, ipam ipam.IPAM, logger *slog.Logger) *Store {
	return &Store{pool: pool, ovn: ovnClient, ipam: ipam, logger: logger}
}

func wrapErr(msg string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", msg, ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%s: %w", msg, ErrConflict)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%s: %w", msg, ErrNotFound)
		}
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// --- Networks ---

func (s *Store) CreateNetwork(ctx context.Context, tenantID uuid.UUID, spec NetworkSpec) (*Network, error) {
	var n Network
	err := s.pool.QueryRow(ctx,
		`INSERT INTO networks (tenant_id, network_domain_id, name, status)
		 VALUES ($1, $2, $3, 'creating')
		 RETURNING id, tenant_id, network_domain_id, name, status, created_at, updated_at`,
		tenantID, spec.NetworkDomainID, spec.Name,
	).Scan(&n.ID, &n.TenantID, &n.NetworkDomainID, &n.Name, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, wrapErr("network: create", err)
	}

	// Create Logical Switch in OVN
	if s.ovn != nil {
		if err := s.ovn.CreateLogicalSwitch(ctx, n.ID.String()); err != nil {
			s.logger.Error("OVN: failed to create logical switch", "network_id", n.ID, "error", err)
			if _, execErr := s.pool.Exec(ctx, `UPDATE networks SET status = 'error', updated_at = now() WHERE id = $1`, n.ID); execErr != nil {
				s.logger.Error("failed to update network status to error", "network_id", n.ID, "error", execErr)
			}
			n.Status = NetworkStatusError
			return &n, nil
		}
	}

	// Mark as active after OVN success
	err = s.pool.QueryRow(ctx,
		`UPDATE networks SET status = 'active', updated_at = now() WHERE id = $1
		 RETURNING id, tenant_id, network_domain_id, name, status, created_at, updated_at`,
		n.ID,
	).Scan(&n.ID, &n.TenantID, &n.NetworkDomainID, &n.Name, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("network: create: update status: %w", err)
	}

	return &n, nil
}

func (s *Store) GetNetwork(ctx context.Context, id uuid.UUID) (*Network, error) {
	var n Network
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, network_domain_id, name, status, created_at, updated_at
		 FROM networks WHERE id = $1`, id,
	).Scan(&n.ID, &n.TenantID, &n.NetworkDomainID, &n.Name, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, wrapErr("network: get", err)
	}
	return &n, nil
}

func (s *Store) ListNetworks(ctx context.Context, tenantID uuid.UUID) ([]Network, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, network_domain_id, name, status, created_at, updated_at
		 FROM networks WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("network: list: %w", err)
	}
	defer rows.Close()

	var networks []Network
	for rows.Next() {
		var n Network
		if err := rows.Scan(&n.ID, &n.TenantID, &n.NetworkDomainID, &n.Name, &n.Status, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("network: list scan: %w", err)
		}
		networks = append(networks, n)
	}
	return networks, rows.Err()
}

func (s *Store) DeleteNetwork(ctx context.Context, id uuid.UUID) error {
	// Check for dependent subnets
	var subnetCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM subnets WHERE network_id = $1`, id).Scan(&subnetCount); err != nil {
		return fmt.Errorf("network: delete: %w", err)
	}
	if subnetCount > 0 {
		return fmt.Errorf("network: delete: %d subnets still attached: %w", subnetCount, ErrHasDependents)
	}

	// Delete from OVN first
	if s.ovn != nil {
		if err := s.ovn.DeleteLogicalSwitch(ctx, id.String()); err != nil {
			return fmt.Errorf("network: delete: OVN logical switch: %w", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM networks WHERE id = $1`, id)
	if err != nil {
		return wrapErr("network: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("network: delete: %w", ErrNotFound)
	}
	return nil
}

// --- Subnets ---

func (s *Store) CreateSubnet(ctx context.Context, networkID uuid.UUID, spec SubnetSpec) (*Subnet, error) {
	// Validate CIDR
	_, ipNet, err := net.ParseCIDR(spec.CIDR)
	if err != nil {
		return nil, fmt.Errorf("network: create subnet: %w: %s", ErrInvalidCIDR, spec.CIDR)
	}

	// Validate gateway is within CIDR
	gw := net.ParseIP(spec.Gateway)
	if gw == nil || !ipNet.Contains(gw) {
		return nil, fmt.Errorf("network: create subnet: %w: %s not in %s", ErrInvalidGateway, spec.Gateway, spec.CIDR)
	}

	// Validate DHCP range
	start := net.ParseIP(spec.DHCPRangeStart)
	end := net.ParseIP(spec.DHCPRangeEnd)
	if start == nil || end == nil || !ipNet.Contains(start) || !ipNet.Contains(end) {
		return nil, fmt.Errorf("network: create subnet: %w", ErrInvalidRange)
	}

	if spec.DNSServers == nil {
		spec.DNSServers = []string{}
	}

	var sub Subnet
	err = s.pool.QueryRow(ctx,
		`INSERT INTO subnets (network_id, cidr, gateway, dhcp_range_start, dhcp_range_end, dns_servers)
		 VALUES ($1, $2::CIDR, $3::INET, $4::INET, $5::INET, $6::INET[])
		 RETURNING id, network_id, cidr::TEXT, host(gateway), host(dhcp_range_start), host(dhcp_range_end),
		           ARRAY(SELECT host(unnest(dns_servers))), created_at`,
		networkID, spec.CIDR, spec.Gateway, spec.DHCPRangeStart, spec.DHCPRangeEnd, spec.DNSServers,
	).Scan(&sub.ID, &sub.NetworkID, &sub.CIDR, &sub.Gateway, &sub.DHCPRangeStart, &sub.DHCPRangeEnd, &sub.DNSServers, &sub.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: create subnet", err)
	}

	// Create DHCP Options in OVN
	if s.ovn != nil {
		dnsStr := ""
		if len(spec.DNSServers) > 0 {
			for i, d := range spec.DNSServers {
				if i > 0 {
					dnsStr += ","
				}
				dnsStr += d
			}
		}
		opts := ovn.DHCPOptions{
			CIDR:       spec.CIDR,
			ExternalID: sub.ID.String(),
			Options: map[string]string{
				"server_id":  spec.Gateway,
				"server_mac": "02:00:00:00:00:01",
				"lease_time": "3600",
			},
		}
		if dnsStr != "" {
			opts.Options["dns_server"] = dnsStr
		}
		if _, err := s.ovn.CreateDHCPOptions(ctx, opts); err != nil {
			s.logger.Error("OVN: failed to create DHCP options", "subnet_id", sub.ID, "error", err)
		}
	}

	return &sub, nil
}

func (s *Store) GetSubnet(ctx context.Context, id uuid.UUID) (*Subnet, error) {
	var sub Subnet
	err := s.pool.QueryRow(ctx,
		`SELECT id, network_id, cidr::TEXT, host(gateway), host(dhcp_range_start), host(dhcp_range_end),
		        ARRAY(SELECT host(unnest(dns_servers))), created_at
		 FROM subnets WHERE id = $1`, id,
	).Scan(&sub.ID, &sub.NetworkID, &sub.CIDR, &sub.Gateway, &sub.DHCPRangeStart, &sub.DHCPRangeEnd, &sub.DNSServers, &sub.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: get subnet", err)
	}
	return &sub, nil
}

func (s *Store) ListSubnets(ctx context.Context, networkID uuid.UUID) ([]Subnet, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, network_id, cidr::TEXT, host(gateway), host(dhcp_range_start), host(dhcp_range_end),
		        ARRAY(SELECT host(unnest(dns_servers))), created_at
		 FROM subnets WHERE network_id = $1 ORDER BY created_at`, networkID)
	if err != nil {
		return nil, fmt.Errorf("network: list subnets: %w", err)
	}
	defer rows.Close()

	var subnets []Subnet
	for rows.Next() {
		var sub Subnet
		if err := rows.Scan(&sub.ID, &sub.NetworkID, &sub.CIDR, &sub.Gateway, &sub.DHCPRangeStart, &sub.DHCPRangeEnd, &sub.DNSServers, &sub.CreatedAt); err != nil {
			return nil, fmt.Errorf("network: list subnets scan: %w", err)
		}
		subnets = append(subnets, sub)
	}
	return subnets, rows.Err()
}

func (s *Store) DeleteSubnet(ctx context.Context, id uuid.UUID) error {
	// Check for dependent ports
	var portCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ports WHERE subnet_id = $1`, id).Scan(&portCount); err != nil {
		return fmt.Errorf("network: delete subnet: %w", err)
	}
	if portCount > 0 {
		return fmt.Errorf("network: delete subnet: %d ports still attached: %w", portCount, ErrHasDependents)
	}

	// Delete DHCP Options in OVN
	if s.ovn != nil {
		if err := s.ovn.DeleteDHCPOptions(ctx, id.String()); err != nil {
			return fmt.Errorf("network: delete subnet: OVN DHCP options: %w", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM subnets WHERE id = $1`, id)
	if err != nil {
		return wrapErr("network: delete subnet", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("network: delete subnet: %w", ErrNotFound)
	}
	return nil
}

// --- Ports ---

func (s *Store) CreatePort(ctx context.Context, tenantID uuid.UUID, spec PortSpec) (*Port, error) {
	// Find the first subnet of this network to allocate from
	var subnetID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM subnets WHERE network_id = $1 ORDER BY created_at LIMIT 1`,
		spec.NetworkID,
	).Scan(&subnetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("network: create port: no subnet in network: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("network: create port: find subnet: %w", err)
	}

	// Allocate IP
	ip, err := s.ipam.AllocateIP(ctx, subnetID)
	if err != nil {
		return nil, fmt.Errorf("network: create port: %w", err)
	}

	// Allocate MAC
	mac, err := s.ipam.AllocateMAC(ctx)
	if err != nil {
		return nil, fmt.Errorf("network: create port: %w", err)
	}

	var p Port
	err = s.pool.QueryRow(ctx,
		`INSERT INTO ports (tenant_id, network_id, subnet_id, mac_address, ip_address, status)
		 VALUES ($1, $2, $3, $4, $5, 'down')
		 RETURNING id, tenant_id, network_id, subnet_id, vm_id, mac_address::TEXT, host(ip_address), status, created_at`,
		tenantID, spec.NetworkID, subnetID, mac.String(), ip.String(),
	).Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.SubnetID, &p.VMID, &p.MACAddress, &p.IPAddress, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: create port", err)
	}

	// Create Logical Switch Port in OVN
	if s.ovn != nil {
		// Get the network ID (used as LS name)
		lsp := ovn.LogicalSwitchPort{
			Name:       p.ID.String(),
			MACAddress: p.MACAddress,
			IPAddress:  p.IPAddress,
		}
		if err := s.ovn.CreateLogicalSwitchPort(ctx, spec.NetworkID.String(), lsp); err != nil {
			s.logger.Error("OVN: failed to create LSP", "port_id", p.ID, "error", err)
			if _, execErr := s.pool.Exec(ctx, `UPDATE ports SET status = 'error' WHERE id = $1`, p.ID); execErr != nil {
				s.logger.Error("failed to update port status to error", "port_id", p.ID, "error", execErr)
			}
			p.Status = PortStatusError
			return &p, nil
		}
	}

	return &p, nil
}

func (s *Store) GetPort(ctx context.Context, id uuid.UUID) (*Port, error) {
	var p Port
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, network_id, subnet_id, vm_id, mac_address::TEXT, host(ip_address), status, created_at
		 FROM ports WHERE id = $1`, id,
	).Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.SubnetID, &p.VMID, &p.MACAddress, &p.IPAddress, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: get port", err)
	}
	return &p, nil
}

func (s *Store) ListPorts(ctx context.Context, networkID uuid.UUID) ([]Port, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, network_id, subnet_id, vm_id, mac_address::TEXT, host(ip_address), status, created_at
		 FROM ports WHERE network_id = $1 ORDER BY created_at`, networkID)
	if err != nil {
		return nil, fmt.Errorf("network: list ports: %w", err)
	}
	defer rows.Close()

	var ports []Port
	for rows.Next() {
		var p Port
		if err := rows.Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.SubnetID, &p.VMID, &p.MACAddress, &p.IPAddress, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("network: list ports scan: %w", err)
		}
		ports = append(ports, p)
	}
	return ports, rows.Err()
}

func (s *Store) DeletePort(ctx context.Context, id uuid.UUID) error {
	// Delete LSP from OVN first
	if s.ovn != nil {
		if err := s.ovn.DeleteLogicalSwitchPort(ctx, id.String()); err != nil {
			return fmt.Errorf("network: delete port: OVN LSP: %w", err)
		}
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM ports WHERE id = $1`, id)
	if err != nil {
		return wrapErr("network: delete port", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("network: delete port: %w", ErrNotFound)
	}
	return nil
}
