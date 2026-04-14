package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/quota"
)

// Store implements Service using PostgreSQL.
type Store struct {
	pool       *pgxpool.Pool
	logger     *slog.Logger
	quotaSvc   quota.Service
	secretsKey []byte // AES-GCM key for encrypting WireGuard private keys
}

// NewStore creates a new network store.
func NewStore(pool *pgxpool.Pool, logger *slog.Logger, quotaSvc quota.Service) *Store {
	return &Store{pool: pool, logger: logger, quotaSvc: quotaSvc}
}

// WithSecretsKey sets the AES-GCM key used to encrypt WireGuard private keys.
// key must be 16, 24, or 32 bytes (for AES-128, AES-192, or AES-256).
func (s *Store) WithSecretsKey(key []byte) *Store {
	s.secretsKey = key
	return s
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
	cidr := spec.CIDR
	if cidr == "" {
		// Auto-assign CIDR from default pool
		rows, err := s.pool.Query(ctx,
			`SELECT cidr::TEXT FROM networks WHERE tenant_id = $1`, tenantID)
		if err != nil {
			return nil, fmt.Errorf("network: create: list cidrs: %w", err)
		}
		defer rows.Close()

		var existingCIDRs []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				return nil, fmt.Errorf("network: create: scan cidr: %w", err)
			}
			existingCIDRs = append(existingCIDRs, c)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("network: create: list cidrs: %w", err)
		}

		assigned, err := AssignCIDR(existingCIDRs)
		if err != nil {
			return nil, fmt.Errorf("network: create: %w", err)
		}
		cidr = assigned
	}

	// Pre-generate the network UUID for quota reservation.
	netID := uuid.New()

	// Reserve quota before inserting.
	if s.quotaSvc != nil {
		netDelta := quota.ResourceDelta{Networks: 1}
		if err := s.quotaSvc.Reserve(ctx, tenantID, quota.ResourceTypeNetwork, netID, netDelta); err != nil {
			return nil, wrapErr("network: create: quota reserve", err)
		}
	}

	var n Network
	err := s.pool.QueryRow(ctx,
		`INSERT INTO networks (id, tenant_id, name, cidr, vni, status)
		 VALUES ($1, $2, $3, $4, nextval('networks_vni_seq'), 'active')
		 RETURNING id, tenant_id, name, cidr::TEXT, vni, status, created_at, updated_at`,
		netID, tenantID, spec.Name, cidr,
	).Scan(&n.ID, &n.TenantID, &n.Name, &n.CIDR, &n.VNI, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		if s.quotaSvc != nil {
			_ = s.quotaSvc.Release(ctx, quota.ResourceTypeNetwork, netID)
		}
		return nil, wrapErr("network: create", err)
	}

	if s.quotaSvc != nil {
		if err := s.quotaSvc.Commit(ctx, quota.ResourceTypeNetwork, netID); err != nil {
			s.logger.Warn("quota commit failed after network creation", "network_id", netID, "error", err)
		}
	}
	return &n, nil
}

func (s *Store) GetNetwork(ctx context.Context, id uuid.UUID) (*Network, error) {
	var n Network
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, cidr::TEXT, vni, status, created_at, updated_at
		 FROM networks WHERE id = $1`, id,
	).Scan(&n.ID, &n.TenantID, &n.Name, &n.CIDR, &n.VNI, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, wrapErr("network: get", err)
	}
	return &n, nil
}

func (s *Store) ListNetworks(ctx context.Context, tenantID uuid.UUID) ([]Network, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, cidr::TEXT, vni, status, created_at, updated_at
		 FROM networks WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("network: list: %w", err)
	}
	defer rows.Close()

	var networks []Network
	for rows.Next() {
		var n Network
		if err := rows.Scan(&n.ID, &n.TenantID, &n.Name, &n.CIDR, &n.VNI, &n.Status, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("network: list scan: %w", err)
		}
		networks = append(networks, n)
	}
	return networks, rows.Err()
}

func (s *Store) ListNetworksPage(ctx context.Context, tenantID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Network, error) {
	scanNetworks := func(query string, args ...any) ([]Network, error) {
		rows, err := s.pool.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("network: list page: %w", err)
		}
		defer rows.Close()
		var networks []Network
		for rows.Next() {
			var n Network
			if err := rows.Scan(&n.ID, &n.TenantID, &n.Name, &n.CIDR, &n.VNI, &n.Status, &n.CreatedAt, &n.UpdatedAt); err != nil {
				return nil, fmt.Errorf("network: list page scan: %w", err)
			}
			networks = append(networks, n)
		}
		return networks, rows.Err()
	}
	if afterCreatedAt.IsZero() {
		return scanNetworks(`SELECT id, tenant_id, name, cidr::TEXT, vni, status, created_at, updated_at FROM networks WHERE tenant_id = $1 ORDER BY created_at, id LIMIT $2`, tenantID, limit)
	}
	return scanNetworks(`SELECT id, tenant_id, name, cidr::TEXT, vni, status, created_at, updated_at FROM networks WHERE tenant_id = $1 AND (created_at > $2 OR (created_at = $2 AND id > $3)) ORDER BY created_at, id LIMIT $4`,
		tenantID, afterCreatedAt, afterID, limit)
}

func (s *Store) DeleteNetwork(ctx context.Context, id uuid.UUID) error {
	// Check for dependent groups
	var groupCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM groups WHERE network_id = $1`, id).Scan(&groupCount); err != nil {
		return fmt.Errorf("network: delete: %w", err)
	}
	if groupCount > 0 {
		return fmt.Errorf("network: delete: %d groups still attached: %w", groupCount, ErrHasDependents)
	}

	// Check for dependent ports
	var portCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ports WHERE network_id = $1`, id).Scan(&portCount); err != nil {
		return fmt.Errorf("network: delete: %w", err)
	}
	if portCount > 0 {
		return fmt.Errorf("network: delete: %d ports still attached: %w", portCount, ErrHasDependents)
	}

	// Check for dependent policies
	var policyCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM policies WHERE network_id = $1`, id).Scan(&policyCount); err != nil {
		return fmt.Errorf("network: delete: %w", err)
	}
	if policyCount > 0 {
		return fmt.Errorf("network: delete: %d policies still attached: %w", policyCount, ErrHasDependents)
	}

	var tenantID uuid.UUID
	if err := s.pool.QueryRow(ctx,
		`DELETE FROM networks WHERE id = $1 RETURNING tenant_id`, id,
	).Scan(&tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("network: delete: %w", ErrNotFound)
		}
		return wrapErr("network: delete", err)
	}
	if s.quotaSvc != nil {
		if err := s.quotaSvc.Decommit(ctx, tenantID, quota.ResourceDelta{Networks: 1}); err != nil {
			s.logger.Warn("quota decommit failed after network deletion", "network_id", id, "error", err)
		}
	}
	return nil
}

// --- Groups ---

func (s *Store) CreateGroup(ctx context.Context, networkID uuid.UUID, spec GroupSpec) (*Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx,
		`INSERT INTO groups (network_id, name)
		 VALUES ($1, $2)
		 RETURNING id, network_id, name, created_at`,
		networkID, spec.Name,
	).Scan(&g.ID, &g.NetworkID, &g.Name, &g.CreatedAt)
	if err != nil {
		return nil, wrapErr("group: create", err)
	}
	return &g, nil
}

func (s *Store) GetGroup(ctx context.Context, id uuid.UUID) (*Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx,
		`SELECT id, network_id, name, created_at FROM groups WHERE id = $1`, id,
	).Scan(&g.ID, &g.NetworkID, &g.Name, &g.CreatedAt)
	if err != nil {
		return nil, wrapErr("group: get", err)
	}
	return &g, nil
}

func (s *Store) ListGroups(ctx context.Context, networkID uuid.UUID) ([]Group, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, network_id, name, created_at FROM groups WHERE network_id = $1 ORDER BY created_at, id`, networkID)
	if err != nil {
		return nil, fmt.Errorf("group: list: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.NetworkID, &g.Name, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("group: list scan: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) ListGroupsPage(ctx context.Context, networkID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Group, error) {
	scanGroups := func(query string, args ...any) ([]Group, error) {
		rows, err := s.pool.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("group: list page: %w", err)
		}
		defer rows.Close()
		var groups []Group
		for rows.Next() {
			var g Group
			if err := rows.Scan(&g.ID, &g.NetworkID, &g.Name, &g.CreatedAt); err != nil {
				return nil, fmt.Errorf("group: list page scan: %w", err)
			}
			groups = append(groups, g)
		}
		return groups, rows.Err()
	}
	if afterCreatedAt.IsZero() {
		return scanGroups(`SELECT id, network_id, name, created_at FROM groups WHERE network_id = $1 ORDER BY created_at, id LIMIT $2`, networkID, limit)
	}
	return scanGroups(`SELECT id, network_id, name, created_at FROM groups WHERE network_id = $1 AND (created_at > $2 OR (created_at = $2 AND id > $3)) ORDER BY created_at, id LIMIT $4`,
		networkID, afterCreatedAt, afterID, limit)
}

func (s *Store) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	// Check for dependent ports
	var portCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ports WHERE group_id = $1`, id).Scan(&portCount); err != nil {
		return fmt.Errorf("group: delete: %w", err)
	}
	if portCount > 0 {
		return fmt.Errorf("group: delete: %d ports still assigned: %w", portCount, ErrHasDependents)
	}

	// Check for dependent policies
	var policyCount int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM policies WHERE src_group_id = $1 OR dst_group_id = $1`, id,
	).Scan(&policyCount); err != nil {
		return fmt.Errorf("group: delete: %w", err)
	}
	if policyCount > 0 {
		return fmt.Errorf("group: delete: %d policies reference this group: %w", policyCount, ErrHasDependents)
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	if err != nil {
		return wrapErr("group: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("group: delete: %w", ErrNotFound)
	}
	return nil
}

// --- Policies ---

func (s *Store) CreatePolicy(ctx context.Context, networkID uuid.UUID, spec PolicySpec) (*Policy, error) {
	// Validate that both groups belong to the specified network
	var srcNetworkID, dstNetworkID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT network_id FROM groups WHERE id = $1`, spec.SrcGroupID).Scan(&srcNetworkID)
	if err != nil {
		return nil, wrapErr("policy: create: src group lookup", err)
	}
	err = s.pool.QueryRow(ctx, `SELECT network_id FROM groups WHERE id = $1`, spec.DstGroupID).Scan(&dstNetworkID)
	if err != nil {
		return nil, wrapErr("policy: create: dst group lookup", err)
	}
	if srcNetworkID != networkID || dstNetworkID != networkID {
		return nil, fmt.Errorf("policy: create: groups must belong to the same network: %w", ErrInvalidState)
	}

	// Default values
	priority := spec.Priority
	if priority == 0 {
		priority = 1000
	}
	action := spec.Action
	if action == "" {
		action = "allow"
	}

	var p Policy
	err = s.pool.QueryRow(ctx,
		`INSERT INTO policies (network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action, created_at`,
		networkID, spec.SrcGroupID, spec.DstGroupID, spec.Protocol, spec.DstPort, priority, action,
	).Scan(&p.ID, &p.NetworkID, &p.SrcGroupID, &p.DstGroupID, &p.Protocol, &p.DstPort, &p.Priority, &p.Action, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("policy: create", err)
	}
	return &p, nil
}

func (s *Store) GetPolicy(ctx context.Context, id uuid.UUID) (*Policy, error) {
	var p Policy
	err := s.pool.QueryRow(ctx,
		`SELECT id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action, created_at
		 FROM policies WHERE id = $1`, id,
	).Scan(&p.ID, &p.NetworkID, &p.SrcGroupID, &p.DstGroupID, &p.Protocol, &p.DstPort, &p.Priority, &p.Action, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("policy: get", err)
	}
	return &p, nil
}

func (s *Store) ListPolicies(ctx context.Context, networkID uuid.UUID) ([]Policy, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action, created_at
		 FROM policies WHERE network_id = $1 ORDER BY created_at, id`, networkID)
	if err != nil {
		return nil, fmt.Errorf("policy: list: %w", err)
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.NetworkID, &p.SrcGroupID, &p.DstGroupID, &p.Protocol, &p.DstPort, &p.Priority, &p.Action, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("policy: list scan: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) ListPoliciesPage(ctx context.Context, networkID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Policy, error) {
	scanPolicies := func(query string, args ...any) ([]Policy, error) {
		rows, err := s.pool.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("policy: list page: %w", err)
		}
		defer rows.Close()
		var policies []Policy
		for rows.Next() {
			var p Policy
			if err := rows.Scan(&p.ID, &p.NetworkID, &p.SrcGroupID, &p.DstGroupID, &p.Protocol, &p.DstPort, &p.Priority, &p.Action, &p.CreatedAt); err != nil {
				return nil, fmt.Errorf("policy: list page scan: %w", err)
			}
			policies = append(policies, p)
		}
		return policies, rows.Err()
	}
	if afterCreatedAt.IsZero() {
		return scanPolicies(`SELECT id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action, created_at FROM policies WHERE network_id = $1 ORDER BY created_at, id LIMIT $2`, networkID, limit)
	}
	return scanPolicies(`SELECT id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action, created_at FROM policies WHERE network_id = $1 AND (created_at > $2 OR (created_at = $2 AND id > $3)) ORDER BY created_at, id LIMIT $4`,
		networkID, afterCreatedAt, afterID, limit)
}

func (s *Store) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM policies WHERE id = $1`, id)
	if err != nil {
		return wrapErr("policy: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("policy: delete: %w", ErrNotFound)
	}
	return nil
}

// --- Ports ---

// CreatePort creates a port with auto-allocated IP and MAC.
// This is an internal operation used by the VM lifecycle and integration tests.
func (s *Store) CreatePort(ctx context.Context, spec PortSpec) (*Port, error) {
	// Get network CIDR for IP allocation
	net, err := s.GetNetwork(ctx, spec.NetworkID)
	if err != nil {
		return nil, fmt.Errorf("network: create port: get network: %w", err)
	}

	// Collect existing IPs for IPAM
	existingPorts, err := s.ListPorts(ctx, spec.NetworkID)
	if err != nil {
		return nil, fmt.Errorf("network: create port: list ports: %w", err)
	}
	var existingIPs []string
	for _, p := range existingPorts {
		existingIPs = append(existingIPs, p.IPAddress)
	}

	// Allocate /30 block
	vmIP, _, err := AllocateBlock(net.CIDR, existingIPs)
	if err != nil {
		return nil, fmt.Errorf("network: create port: allocate ip: %w", err)
	}

	// Generate MAC
	mac, err := GenerateMAC()
	if err != nil {
		return nil, fmt.Errorf("network: create port: generate mac: %w", err)
	}

	var groupID *uuid.UUID
	if spec.GroupID != uuid.Nil {
		groupID = &spec.GroupID
	}

	var p Port
	err = s.pool.QueryRow(ctx,
		`INSERT INTO ports (network_id, group_id, tenant_id, host_id, vm_id, mac_address, ip_address, vm_name, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6::macaddr, $7::inet, $8, 'active', NOW())
		 RETURNING id, tenant_id, network_id, group_id, vm_id, vm_name, mac_address::TEXT, host(ip_address), host_id, role, status, created_at`,
		spec.NetworkID, groupID, spec.TenantID, spec.HostID, spec.VMID, mac, vmIP, spec.VMName,
	).Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.GroupID, &p.VMID, &p.VMName, &p.MACAddress, &p.IPAddress, &p.HostID, &p.Role, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("network: create port: %w", err)
	}
	return &p, nil
}

// --- Ports ---

func (s *Store) GetPort(ctx context.Context, id uuid.UUID) (*Port, error) {
	var p Port
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, network_id, group_id, vm_id, vm_name, mac_address::TEXT, host(ip_address), host_id, role, status, created_at
		 FROM ports WHERE id = $1`, id,
	).Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.GroupID, &p.VMID, &p.VMName, &p.MACAddress, &p.IPAddress, &p.HostID, &p.Role, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: get port", err)
	}
	return &p, nil
}

func (s *Store) ListPorts(ctx context.Context, networkID uuid.UUID) ([]Port, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, network_id, group_id, vm_id, vm_name, mac_address::TEXT, host(ip_address), host_id, role, status, created_at
		 FROM ports WHERE network_id = $1 ORDER BY created_at`, networkID)
	if err != nil {
		return nil, fmt.Errorf("network: list ports: %w", err)
	}
	defer rows.Close()

	var ports []Port
	for rows.Next() {
		var p Port
		if err := rows.Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.GroupID, &p.VMID, &p.VMName, &p.MACAddress, &p.IPAddress, &p.HostID, &p.Role, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("network: list ports scan: %w", err)
		}
		ports = append(ports, p)
	}
	return ports, rows.Err()
}

func (s *Store) GetPortByVMID(ctx context.Context, vmID uuid.UUID) (*Port, error) {
	var p Port
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, network_id, group_id, vm_id, vm_name, mac_address::TEXT, host(ip_address), host_id, role, status, created_at
		 FROM ports WHERE vm_id = $1 LIMIT 1`, vmID,
	).Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.GroupID, &p.VMID, &p.VMName, &p.MACAddress, &p.IPAddress, &p.HostID, &p.Role, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: get port by vm_id", err)
	}
	return &p, nil
}

func (s *Store) DeletePort(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ports WHERE id = $1`, id)
	return err
}

// --- Gateway Nodes ---

func (s *Store) CreateGatewayNode(ctx context.Context, spec GatewayNodeSpec) (*GatewayNode, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("gateway_node: create: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var gw GatewayNode
	err = tx.QueryRow(ctx,
		`INSERT INTO gateway_nodes (host_id, external_ip, internal_ip, uplink_port, status)
		 VALUES ($1, $2::inet, $3::inet, $4, 'active')
		 RETURNING id, host_id, host(external_ip), host(internal_ip), uplink_port, status, created_at`,
		spec.HostID, spec.ExternalIP, spec.InternalIP, spec.UplinkPort,
	).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.UplinkPort, &gw.Status, &gw.CreatedAt)
	if err != nil {
		return nil, wrapErr("gateway_node: create", err)
	}

	// Add 'gateway' to node_roles if not already present
	_, err = tx.Exec(ctx,
		`UPDATE hosts SET node_roles = array_append(node_roles, 'gateway')
		 WHERE id = $1 AND NOT ('gateway' = ANY(node_roles))`,
		spec.HostID,
	)
	if err != nil {
		return nil, fmt.Errorf("gateway_node: create: update host roles: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("gateway_node: create: commit: %w", err)
	}
	return &gw, nil
}

func (s *Store) GetGatewayNode(ctx context.Context, id uuid.UUID) (*GatewayNode, error) {
	var gw GatewayNode
	err := s.pool.QueryRow(ctx,
		`SELECT id, host_id, host(external_ip), host(internal_ip), uplink_port, status, created_at
		 FROM gateway_nodes WHERE id = $1`, id,
	).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.UplinkPort, &gw.Status, &gw.CreatedAt)
	if err != nil {
		return nil, wrapErr("gateway_node: get", err)
	}
	return &gw, nil
}

func (s *Store) ListGatewayNodes(ctx context.Context) ([]GatewayNode, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, host_id, host(external_ip), host(internal_ip), uplink_port, status, created_at
		 FROM gateway_nodes ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("gateway_node: list: %w", err)
	}
	defer rows.Close()

	var nodes []GatewayNode
	for rows.Next() {
		var gw GatewayNode
		if err := rows.Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.UplinkPort, &gw.Status, &gw.CreatedAt); err != nil {
			return nil, fmt.Errorf("gateway_node: list scan: %w", err)
		}
		nodes = append(nodes, gw)
	}
	return nodes, rows.Err()
}

func (s *Store) DeleteGatewayNode(ctx context.Context, id uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("gateway_node: delete: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Fetch host_id before deleting
	var hostID uuid.UUID
	err = tx.QueryRow(ctx, `DELETE FROM gateway_nodes WHERE id = $1 RETURNING host_id`, id).Scan(&hostID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("gateway_node: delete: %w", ErrNotFound)
		}
		return wrapErr("gateway_node: delete", err)
	}

	// Check if host still has another gateway_node entry before removing role
	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM gateway_nodes WHERE host_id = $1`, hostID).Scan(&count); err != nil {
		return fmt.Errorf("gateway_node: delete: count remaining: %w", err)
	}
	if count == 0 {
		// Remove 'gateway' from node_roles
		_, err = tx.Exec(ctx,
			`UPDATE hosts SET node_roles = array_remove(node_roles, 'gateway') WHERE id = $1`,
			hostID,
		)
		if err != nil {
			return fmt.Errorf("gateway_node: delete: update host roles: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("gateway_node: delete: commit: %w", err)
	}
	return nil
}

func (s *Store) AssignGatewayNode(ctx context.Context, networkID uuid.UUID, gatewayNodeID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE networks SET gateway_node_id = $1 WHERE id = $2`,
		gatewayNodeID, networkID,
	)
	if err != nil {
		return wrapErr("gateway_node: assign", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("gateway_node: assign: %w", ErrNotFound)
	}
	return nil
}

func (s *Store) GetNetworkGatewayNode(ctx context.Context, networkID uuid.UUID) (*GatewayNode, error) {
	var gw GatewayNode
	err := s.pool.QueryRow(ctx,
		`SELECT gn.id, gn.host_id, host(gn.external_ip), host(gn.internal_ip), gn.uplink_port, gn.status, gn.created_at
		 FROM gateway_nodes gn
		 JOIN networks n ON n.gateway_node_id = gn.id
		 WHERE n.id = $1`, networkID,
	).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.UplinkPort, &gw.Status, &gw.CreatedAt)
	if err != nil {
		return nil, wrapErr("gateway_node: get network gateway", err)
	}
	return &gw, nil
}

// --- Egresses ---

// validateEgressSpec validates the required fields of an EgressSpec for its type
// and performs any cryptographic pre-processing (key generation, encryption).
// It returns ErrInvalidState for missing/invalid fields, or an error from key operations.
// Note: direct_connect uplink_port population requires a DB call and is handled separately in CreateEgress.
func (s *Store) validateEgressSpec(spec *EgressSpec) error {
	switch spec.Type {
	case EgressTypeNATGateway:
		if spec.Config.PublicIP == "" {
			return fmt.Errorf("config.public_ip is required for nat_gateway: %w", ErrInvalidState)
		}
	case EgressTypeVPNIPsec:
		if spec.Config.VPNIPsec == nil {
			return fmt.Errorf("config.vpn_ipsec is required for vpn_ipsec: %w", ErrInvalidState)
		}
		if spec.Config.VPNIPsec.PeerIP == "" || spec.Config.VPNIPsec.PreSharedKey == "" ||
			spec.Config.VPNIPsec.LocalCIDR == "" || spec.Config.VPNIPsec.RemoteCIDR == "" {
			return fmt.Errorf("vpn_ipsec requires peer_ip, pre_shared_key, local_cidr, remote_cidr: %w", ErrInvalidState)
		}
		return s.encryptIPsecPSK(&spec.Config)
	case EgressTypeVPNWireGuard:
		if spec.Config.VPNWireGuard == nil {
			return fmt.Errorf("config.vpn_wireguard is required for vpn_wireguard: %w", ErrInvalidState)
		}
		if spec.Config.VPNWireGuard.PeerPublicKey == "" || spec.Config.VPNWireGuard.PeerEndpoint == "" {
			return fmt.Errorf("vpn_wireguard requires peer_public_key, peer_endpoint: %w", ErrInvalidState)
		}
		return s.generateWireGuardKeys(&spec.Config)
	case EgressTypeDirectConnect:
		if spec.Config.DirectConnect == nil {
			return fmt.Errorf("config.direct_connect is required for direct_connect: %w", ErrInvalidState)
		}
		if spec.Config.DirectConnect.VLANID < 1 || spec.Config.DirectConnect.VLANID > 4094 {
			return fmt.Errorf("direct_connect vlan_id must be between 1 and 4094: %w", ErrInvalidState)
		}
	default:
		return fmt.Errorf("unsupported type %q: %w", spec.Type, ErrInvalidState)
	}
	return nil
}

func (s *Store) CreateEgress(ctx context.Context, networkID uuid.UUID, spec EgressSpec) (*Egress, error) {
	if err := s.validateEgressSpec(&spec); err != nil {
		return nil, fmt.Errorf("egress: create: %w", err)
	}

	// For direct_connect, uplink_port is populated from the network's assigned GW node.
	if spec.Type == EgressTypeDirectConnect {
		if err := s.populateDirectConnectUplinkPort(ctx, networkID, &spec.Config); err != nil {
			return nil, fmt.Errorf("egress: create: %w", err)
		}
	}

	// Fetch tenant_id for quota tracking.
	var tenantID uuid.UUID
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id FROM networks WHERE id = $1`, networkID).Scan(&tenantID); err != nil {
		return nil, wrapErr("egress: create: get tenant", err)
	}

	egressID := uuid.New()

	// Reserve quota.
	if s.quotaSvc != nil {
		if err := s.quotaSvc.Reserve(ctx, tenantID, quota.ResourceTypeEgress, egressID, quota.ResourceDelta{Egresses: 1}); err != nil {
			return nil, wrapErr("egress: create: quota reserve", err)
		}
	}

	configJSON, err := json.Marshal(spec.Config)
	if err != nil {
		if s.quotaSvc != nil {
			_ = s.quotaSvc.Release(ctx, quota.ResourceTypeEgress, egressID)
		}
		return nil, fmt.Errorf("egress: create: marshal config: %w", err)
	}

	var e Egress
	e.NetworkID = networkID
	e.Type = spec.Type
	err = s.pool.QueryRow(ctx,
		`INSERT INTO egresses (id, network_id, type, config)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, network_id, type, config`,
		egressID, networkID, spec.Type, configJSON,
	).Scan(&e.ID, &e.NetworkID, &e.Type, &configJSON)
	if err != nil {
		if s.quotaSvc != nil {
			_ = s.quotaSvc.Release(ctx, quota.ResourceTypeEgress, egressID)
		}
		return nil, wrapErr("egress: create", err)
	}

	if s.quotaSvc != nil {
		if err := s.quotaSvc.Commit(ctx, quota.ResourceTypeEgress, egressID); err != nil {
			s.logger.Warn("quota commit failed after egress creation", "egress_id", egressID, "error", err)
		}
	}

	if err := json.Unmarshal(configJSON, &e.Config); err != nil {
		return nil, fmt.Errorf("egress: create: unmarshal config: %w", err)
	}
	return &e, nil
}

// encryptIPsecPSK encrypts the plaintext PreSharedKey in the VPNIPsecConfig using
// AES-GCM, stores the result as PreSharedKeyEnc (base64), and clears PreSharedKey.
func (s *Store) encryptIPsecPSK(cfg *EgressConfig) error {
	if len(s.secretsKey) == 0 {
		return fmt.Errorf("ipsec psk encryption: secrets key not configured")
	}
	encPSK, err := EncryptAESGCM(s.secretsKey, []byte(cfg.VPNIPsec.PreSharedKey))
	if err != nil {
		return fmt.Errorf("ipsec psk encryption: %w", err)
	}
	cfg.VPNIPsec.PreSharedKeyEnc = encodeBase64(encPSK)
	cfg.VPNIPsec.PreSharedKey = "" // clear plaintext
	return nil
}

// populateDirectConnectUplinkPort fetches the GW node assigned to the network and
// copies its uplink_port into the DirectConnectConfig. Users do not specify uplink_port
// directly; it is derived from the gateway node's registration configuration.
func (s *Store) populateDirectConnectUplinkPort(ctx context.Context, networkID uuid.UUID, cfg *EgressConfig) error {
	var uplinkPort string
	err := s.pool.QueryRow(ctx,
		`SELECT gn.uplink_port
		 FROM gateway_nodes gn
		 JOIN networks n ON n.gateway_node_id = gn.id
		 WHERE n.id = $1 AND gn.status = 'active'
		 LIMIT 1`, networkID,
	).Scan(&uplinkPort)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("direct_connect: no active gateway node assigned to network %s: %w", networkID, ErrInvalidState)
		}
		return fmt.Errorf("direct_connect: lookup gateway node: %w", err)
	}
	if uplinkPort == "" {
		return fmt.Errorf("direct_connect: gateway node for network %s has no uplink_port configured: set gw_uplink_port in the GW worker's cirrus.yaml: %w", networkID, ErrInvalidState)
	}
	cfg.DirectConnect.UplinkPort = uplinkPort
	return nil
}

// generateWireGuardKeys generates a WireGuard keypair, encrypts the private key,
// and stores both keys in spec.Config.VPNWireGuard.
func (s *Store) generateWireGuardKeys(cfg *EgressConfig) error {
	if len(s.secretsKey) == 0 {
		return fmt.Errorf("wireguard key generation: secrets key not configured")
	}
	priv, pub, err := GenerateWireGuardKeypair()
	if err != nil {
		return fmt.Errorf("wireguard key generation: %w", err)
	}
	encPriv, err := EncryptAESGCM(s.secretsKey, priv)
	if err != nil {
		return fmt.Errorf("wireguard key generation: encrypt private key: %w", err)
	}
	cfg.VPNWireGuard.PrivateKeyEnc = encodeBase64(encPriv)
	cfg.VPNWireGuard.PublicKey = encodeBase64(pub)
	return nil
}

func (s *Store) GetEgress(ctx context.Context, id uuid.UUID) (*Egress, error) {
	var e Egress
	var configJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, network_id, type, config FROM egresses WHERE id = $1`, id,
	).Scan(&e.ID, &e.NetworkID, &e.Type, &configJSON)
	if err != nil {
		return nil, wrapErr("egress: get", err)
	}
	if err := json.Unmarshal(configJSON, &e.Config); err != nil {
		return nil, fmt.Errorf("egress: get: unmarshal config: %w", err)
	}
	return &e, nil
}

func (s *Store) ListEgresses(ctx context.Context, networkID uuid.UUID) ([]Egress, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, network_id, type, config FROM egresses WHERE network_id = $1 ORDER BY id`, networkID)
	if err != nil {
		return nil, fmt.Errorf("egress: list: %w", err)
	}
	defer rows.Close()

	var egresses []Egress
	for rows.Next() {
		var e Egress
		var configJSON []byte
		if err := rows.Scan(&e.ID, &e.NetworkID, &e.Type, &configJSON); err != nil {
			return nil, fmt.Errorf("egress: list scan: %w", err)
		}
		if err := json.Unmarshal(configJSON, &e.Config); err != nil {
			return nil, fmt.Errorf("egress: list: unmarshal config: %w", err)
		}
		egresses = append(egresses, e)
	}
	return egresses, rows.Err()
}

func (s *Store) UpdateEgressConfig(ctx context.Context, egressID uuid.UUID, config EgressConfig) (*Egress, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("egress: update: marshal config: %w", err)
	}
	var e Egress
	var rawConfig []byte
	err = s.pool.QueryRow(ctx,
		`UPDATE egresses SET config = $1 WHERE id = $2
		 RETURNING id, network_id, type, config`,
		configJSON, egressID,
	).Scan(&e.ID, &e.NetworkID, &e.Type, &rawConfig)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("egress: update: %w", ErrNotFound)
		}
		return nil, wrapErr("egress: update", err)
	}
	if err := json.Unmarshal(rawConfig, &e.Config); err != nil {
		return nil, fmt.Errorf("egress: update: unmarshal config: %w", err)
	}
	return &e, nil
}

func (s *Store) DeleteEgress(ctx context.Context, id uuid.UUID) error {
	// Delete and fetch tenant_id atomically via CTE to avoid a separate round-trip.
	var tenantID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		WITH deleted AS (
			DELETE FROM egresses WHERE id = $1 RETURNING network_id
		)
		SELECT n.tenant_id FROM deleted d JOIN networks n ON n.id = d.network_id
	`, id).Scan(&tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("egress: delete: %w", ErrNotFound)
		}
		return wrapErr("egress: delete", err)
	}
	if s.quotaSvc != nil {
		if err := s.quotaSvc.Decommit(ctx, tenantID, quota.ResourceDelta{Egresses: 1}); err != nil {
			s.logger.Warn("quota decommit failed after egress deletion", "egress_id", id, "error", err)
		}
	}
	return nil
}

// --- IP Pools ---

func (s *Store) CreateIPPool(ctx context.Context, spec IPPoolSpec) (*IPPool, error) {
	var p IPPool
	err := s.pool.QueryRow(ctx,
		`INSERT INTO ip_pools (name, cidr, description)
		 VALUES ($1, $2::cidr, $3)
		 RETURNING id, name, cidr::TEXT, description, created_at`,
		spec.Name, spec.CIDR, spec.Description,
	).Scan(&p.ID, &p.Name, &p.CIDR, &p.Description, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("ip_pool: create", err)
	}
	return &p, nil
}

func (s *Store) GetIPPool(ctx context.Context, id uuid.UUID) (*IPPool, error) {
	var p IPPool
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, cidr::TEXT, description, created_at FROM ip_pools WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.CIDR, &p.Description, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("ip_pool: get", err)
	}
	return &p, nil
}

func (s *Store) ListIPPools(ctx context.Context) ([]IPPool, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, cidr::TEXT, description, created_at FROM ip_pools ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("ip_pool: list: %w", err)
	}
	defer rows.Close()

	var pools []IPPool
	for rows.Next() {
		var p IPPool
		if err := rows.Scan(&p.ID, &p.Name, &p.CIDR, &p.Description, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("ip_pool: list scan: %w", err)
		}
		pools = append(pools, p)
	}
	return pools, rows.Err()
}

func (s *Store) DeleteIPPool(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ip_pools WHERE id = $1`, id)
	if err != nil {
		return wrapErr("ip_pool: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ip_pool: delete: %w", ErrNotFound)
	}
	return nil
}

// --- Ingresses ---

func (s *Store) CreateIngress(ctx context.Context, networkID uuid.UUID, spec IngressSpec) (*Ingress, error) {
	switch spec.Type {
	case IngressTypeDirectIP, IngressTypeL4LB:
		// supported
	default:
		return nil, fmt.Errorf("ingress: create: unsupported type %q: %w", spec.Type, ErrInvalidState)
	}

	// Validate public_ip is within the specified ip_pool's CIDR
	var poolCIDR string
	err := s.pool.QueryRow(ctx, `SELECT cidr::TEXT FROM ip_pools WHERE id = $1`, spec.IPPoolID).Scan(&poolCIDR)
	if err != nil {
		return nil, wrapErr("ingress: create: ip_pool lookup", err)
	}

	// Check public_ip is within pool CIDR
	if !ipInCIDR(spec.PublicIP, poolCIDR) {
		return nil, fmt.Errorf("ingress: create: public_ip %s is not within pool CIDR %s: %w", spec.PublicIP, poolCIDR, ErrInvalidState)
	}

	if spec.Type == IngressTypeDirectIP {
		// Resolve target_ip from the VM's port if not provided.
		if spec.Config.TargetVMID != "" && spec.Config.TargetIP == "" {
			vmUUID, err := uuid.Parse(spec.Config.TargetVMID)
			if err != nil {
				return nil, fmt.Errorf("ingress: create: invalid target_vm_id: %w", ErrInvalidState)
			}
			port, err := s.GetPortByVMID(ctx, vmUUID)
			if err != nil {
				return nil, fmt.Errorf("ingress: create: resolve target VM port: %w", err)
			}
			spec.Config.TargetIP = port.IPAddress
		}
		if spec.Config.TargetIP == "" {
			return nil, fmt.Errorf("ingress: create: target_ip is required (provide target_vm_id or target_ip): %w", ErrInvalidState)
		}
	} else if spec.Type == IngressTypeL4LB {
		// Validate L4LB config
		if spec.L4LBConfig == nil {
			return nil, fmt.Errorf("ingress: create: l4lb_config is required for l4_lb type: %w", ErrInvalidState)
		}
		if len(spec.L4LBConfig.Backends) == 0 {
			return nil, fmt.Errorf("ingress: create: l4lb_config.backends must not be empty: %w", ErrInvalidState)
		}
		if spec.L4LBConfig.ListenerPort <= 0 || spec.L4LBConfig.ListenerPort > 65535 {
			return nil, fmt.Errorf("ingress: create: invalid listener_port %d: %w", spec.L4LBConfig.ListenerPort, ErrInvalidState)
		}
		if spec.L4LBConfig.Protocol == "" {
			spec.L4LBConfig.Protocol = "tcp"
		}
		if spec.L4LBConfig.Protocol != "tcp" {
			return nil, fmt.Errorf("ingress: create: unsupported protocol %q (only tcp supported): %w", spec.L4LBConfig.Protocol, ErrInvalidState)
		}
		if spec.L4LBConfig.SessionAffinity == "" {
			spec.L4LBConfig.SessionAffinity = "none"
		}
		// Resolve backend IPs from VM ports when IP not provided
		for i := range spec.L4LBConfig.Backends {
			b := &spec.L4LBConfig.Backends[i]
			if b.Weight == 0 {
				b.Weight = 1
			}
			b.Healthy = true // newly created backends start healthy
			if b.IP == "" && b.VMID != "" {
				vmUUID, err := uuid.Parse(b.VMID)
				if err != nil {
					return nil, fmt.Errorf("ingress: create: invalid backend vm_id %q: %w", b.VMID, ErrInvalidState)
				}
				port, err := s.GetPortByVMID(ctx, vmUUID)
				if err != nil {
					return nil, fmt.Errorf("ingress: create: resolve backend VM port for %s: %w", b.VMID, err)
				}
				b.IP = port.IPAddress
			}
			if b.IP == "" {
				return nil, fmt.Errorf("ingress: create: backend ip is required (provide vm_id or ip): %w", ErrInvalidState)
			}
		}
	}

	// Fetch tenant_id for quota tracking.
	var tenantID uuid.UUID
	if err := s.pool.QueryRow(ctx, `SELECT tenant_id FROM networks WHERE id = $1`, networkID).Scan(&tenantID); err != nil {
		return nil, wrapErr("ingress: create: get tenant", err)
	}

	ingressID := uuid.New()

	// Reserve quota.
	if s.quotaSvc != nil {
		if err := s.quotaSvc.Reserve(ctx, tenantID, quota.ResourceTypeIngress, ingressID, quota.ResourceDelta{Ingresses: 1}); err != nil {
			return nil, wrapErr("ingress: create: quota reserve", err)
		}
	}

	// For l4_lb, store the full l4lb config inside the config JSONB field under the key "l4lb".
	// For direct_ip, store the IngressConfig as before.
	var configToStore interface{}
	if spec.Type == IngressTypeL4LB {
		configToStore = struct {
			L4LB *L4LBConfig `json:"l4lb"`
		}{L4LB: spec.L4LBConfig}
	} else {
		configToStore = spec.Config
	}

	configJSON, err := json.Marshal(configToStore)
	if err != nil {
		if s.quotaSvc != nil {
			_ = s.quotaSvc.Release(ctx, quota.ResourceTypeIngress, ingressID)
		}
		return nil, fmt.Errorf("ingress: create: marshal config: %w", err)
	}

	var ing Ingress
	var rawConfig []byte
	err = s.pool.QueryRow(ctx,
		`INSERT INTO ingresses (id, network_id, type, public_ip, ip_pool_id, config)
		 VALUES ($1, $2, $3, $4::inet, $5, $6)
		 RETURNING id, network_id, type, host(public_ip), ip_pool_id, config, created_at`,
		ingressID, networkID, spec.Type, spec.PublicIP, spec.IPPoolID, configJSON,
	).Scan(&ing.ID, &ing.NetworkID, &ing.Type, &ing.PublicIP, &ing.IPPoolID, &rawConfig, &ing.CreatedAt)
	if err != nil {
		if s.quotaSvc != nil {
			_ = s.quotaSvc.Release(ctx, quota.ResourceTypeIngress, ingressID)
		}
		return nil, wrapErr("ingress: create", err)
	}

	if s.quotaSvc != nil {
		if err := s.quotaSvc.Commit(ctx, quota.ResourceTypeIngress, ingressID); err != nil {
			s.logger.Warn("quota commit failed after ingress creation", "ingress_id", ingressID, "error", err)
		}
	}

	if err := s.unmarshalIngressConfig(&ing, rawConfig); err != nil {
		return nil, fmt.Errorf("ingress: create: unmarshal config: %w", err)
	}

	// For l4_lb, insert initial health rows.
	if spec.Type == IngressTypeL4LB && spec.L4LBConfig != nil {
		for _, b := range spec.L4LBConfig.Backends {
			if b.VMID == "" {
				continue
			}
			vmUUID, err := uuid.Parse(b.VMID)
			if err != nil {
				continue
			}
			_, _ = s.pool.Exec(ctx, `
				INSERT INTO l4lb_backend_health (ingress_id, vm_id, healthy, last_checked_at)
				VALUES ($1, $2, true, NOW())
				ON CONFLICT (ingress_id, vm_id) DO NOTHING
			`, ingressID, vmUUID)
		}
	}

	return &ing, nil
}

// unmarshalIngressConfig decodes the raw config JSONB into the appropriate fields.
func (s *Store) unmarshalIngressConfig(ing *Ingress, raw []byte) error {
	if ing.Type == IngressTypeL4LB {
		var wrapper struct {
			L4LB *L4LBConfig `json:"l4lb"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return err
		}
		ing.L4LBConfig = wrapper.L4LB
		return nil
	}
	return json.Unmarshal(raw, &ing.Config)
}

func (s *Store) GetIngress(ctx context.Context, id uuid.UUID) (*Ingress, error) {
	var ing Ingress
	var configJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, network_id, type, host(public_ip), ip_pool_id, config, created_at
		 FROM ingresses WHERE id = $1`, id,
	).Scan(&ing.ID, &ing.NetworkID, &ing.Type, &ing.PublicIP, &ing.IPPoolID, &configJSON, &ing.CreatedAt)
	if err != nil {
		return nil, wrapErr("ingress: get", err)
	}
	if err := s.unmarshalIngressConfig(&ing, configJSON); err != nil {
		return nil, fmt.Errorf("ingress: get: unmarshal config: %w", err)
	}
	return &ing, nil
}

func (s *Store) ListIngresses(ctx context.Context, networkID uuid.UUID) ([]Ingress, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, network_id, type, host(public_ip), ip_pool_id, config, created_at
		 FROM ingresses WHERE network_id = $1 ORDER BY created_at`, networkID)
	if err != nil {
		return nil, fmt.Errorf("ingress: list: %w", err)
	}
	defer rows.Close()

	var ingresses []Ingress
	for rows.Next() {
		var ing Ingress
		var configJSON []byte
		if err := rows.Scan(&ing.ID, &ing.NetworkID, &ing.Type, &ing.PublicIP, &ing.IPPoolID, &configJSON, &ing.CreatedAt); err != nil {
			return nil, fmt.Errorf("ingress: list scan: %w", err)
		}
		if err := s.unmarshalIngressConfig(&ing, configJSON); err != nil {
			return nil, fmt.Errorf("ingress: list: unmarshal config: %w", err)
		}
		ingresses = append(ingresses, ing)
	}
	return ingresses, rows.Err()
}

func (s *Store) UpdateIngressConfig(ctx context.Context, ingressID uuid.UUID, config IngressConfig) (*Ingress, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("ingress: update: marshal config: %w", err)
	}
	var ing Ingress
	var rawConfig []byte
	err = s.pool.QueryRow(ctx,
		`UPDATE ingresses SET config = $1 WHERE id = $2
		 RETURNING id, network_id, type, host(public_ip), ip_pool_id, config, created_at`,
		configJSON, ingressID,
	).Scan(&ing.ID, &ing.NetworkID, &ing.Type, &ing.PublicIP, &ing.IPPoolID, &rawConfig, &ing.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("ingress: update: %w", ErrNotFound)
		}
		return nil, wrapErr("ingress: update", err)
	}
	if err := s.unmarshalIngressConfig(&ing, rawConfig); err != nil {
		return nil, fmt.Errorf("ingress: update: unmarshal config: %w", err)
	}
	return &ing, nil
}

func (s *Store) DeleteIngress(ctx context.Context, id uuid.UUID) error {
	// Delete and fetch tenant_id atomically via CTE to avoid a separate round-trip.
	var tenantID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		WITH deleted AS (
			DELETE FROM ingresses WHERE id = $1 RETURNING network_id
		)
		SELECT n.tenant_id FROM deleted d JOIN networks n ON n.id = d.network_id
	`, id).Scan(&tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("ingress: delete: %w", ErrNotFound)
		}
		return wrapErr("ingress: delete", err)
	}
	if s.quotaSvc != nil {
		if err := s.quotaSvc.Decommit(ctx, tenantID, quota.ResourceDelta{Ingresses: 1}); err != nil {
			s.logger.Warn("quota decommit failed after ingress deletion", "ingress_id", id, "error", err)
		}
	}
	return nil
}

// UpdateBackendHealth updates the healthy state of a backend VM in an l4_lb ingress.
// It upserts into l4lb_backend_health and refreshes the backends[].healthy field in the
// ingresses.config JSONB.
func (s *Store) UpdateBackendHealth(ctx context.Context, ingressID uuid.UUID, vmID uuid.UUID, healthy bool) error {
	// Upsert the health row.
	_, err := s.pool.Exec(ctx, `
		INSERT INTO l4lb_backend_health (ingress_id, vm_id, healthy, last_checked_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (ingress_id, vm_id) DO UPDATE
		  SET healthy = EXCLUDED.healthy,
		      last_checked_at = EXCLUDED.last_checked_at
	`, ingressID, vmID, healthy)
	if err != nil {
		return fmt.Errorf("ingress: update backend health: upsert: %w", err)
	}
	return nil
}

// --- Internal Load Balancers ---

// CreateLoadBalancer allocates a VIP from the network CIDR and creates an
// internal load balancer. OVS flows are installed on every host in the network.
func (s *Store) CreateLoadBalancer(ctx context.Context, tenantID, networkID uuid.UUID, spec LoadBalancerSpec) (*LoadBalancer, error) {
	if spec.Name == "" {
		return nil, fmt.Errorf("lb: create: name is required: %w", ErrInvalidState)
	}
	if spec.Config.ListenerPort <= 0 || spec.Config.ListenerPort > 65535 {
		return nil, fmt.Errorf("lb: create: invalid listener_port %d: %w", spec.Config.ListenerPort, ErrInvalidState)
	}
	if len(spec.Config.Backends) == 0 {
		return nil, fmt.Errorf("lb: create: backends must not be empty: %w", ErrInvalidState)
	}
	if spec.Config.Protocol == "" {
		spec.Config.Protocol = "tcp"
	}
	if spec.Config.Protocol != "tcp" {
		return nil, fmt.Errorf("lb: create: unsupported protocol %q (only tcp supported): %w", spec.Config.Protocol, ErrInvalidState)
	}
	if spec.Config.SessionAffinity == "" {
		spec.Config.SessionAffinity = "none"
	}
	if spec.Config.SessionAffinity != "none" && spec.Config.SessionAffinity != "source_ip" {
		return nil, fmt.Errorf("lb: create: invalid session_affinity %q (must be none or source_ip): %w", spec.Config.SessionAffinity, ErrInvalidState)
	}

	// Get network CIDR.
	nw, err := s.GetNetwork(ctx, networkID)
	if err != nil {
		return nil, fmt.Errorf("lb: create: get network: %w", err)
	}

	// Resolve backend IPs from VM ports when IP is not provided.
	for i := range spec.Config.Backends {
		b := &spec.Config.Backends[i]
		if b.Weight == 0 {
			b.Weight = 1
		}
		b.Healthy = true
		if b.IP == "" && b.VMID != "" {
			vmUUID, err := uuid.Parse(b.VMID)
			if err != nil {
				return nil, fmt.Errorf("lb: create: invalid backend vm_id %q: %w", b.VMID, ErrInvalidState)
			}
			port, err := s.GetPortByVMID(ctx, vmUUID)
			if err != nil {
				return nil, fmt.Errorf("lb: create: resolve backend VM port for %s: %w", b.VMID, err)
			}
			b.IP = port.IPAddress
		}
		if b.IP == "" {
			return nil, fmt.Errorf("lb: create: backend ip is required (provide vm_id or ip): %w", ErrInvalidState)
		}
	}

	// Collect existing IPs (port IPs + existing VIPs) to avoid conflicts.
	portIPRows, err := s.pool.Query(ctx, `SELECT host(ip_address) FROM ports WHERE network_id = $1`, networkID)
	if err != nil {
		return nil, fmt.Errorf("lb: create: list port ips: %w", err)
	}
	var existingIPs []string
	for portIPRows.Next() {
		var ip string
		if err := portIPRows.Scan(&ip); err != nil {
			portIPRows.Close()
			return nil, fmt.Errorf("lb: create: scan port ip: %w", err)
		}
		existingIPs = append(existingIPs, ip)
	}
	portIPRows.Close()
	if err := portIPRows.Err(); err != nil {
		return nil, fmt.Errorf("lb: create: port ip rows: %w", err)
	}

	existingVIPRows, err := s.pool.Query(ctx, `SELECT host(vip) FROM load_balancers WHERE network_id = $1`, networkID)
	if err != nil {
		return nil, fmt.Errorf("lb: create: list existing vips: %w", err)
	}
	for existingVIPRows.Next() {
		var vip string
		if err := existingVIPRows.Scan(&vip); err != nil {
			existingVIPRows.Close()
			return nil, fmt.Errorf("lb: create: scan vip: %w", err)
		}
		existingIPs = append(existingIPs, vip)
	}
	existingVIPRows.Close()
	if err := existingVIPRows.Err(); err != nil {
		return nil, fmt.Errorf("lb: create: vip rows: %w", err)
	}

	vip, err := AllocateVIP(nw.CIDR, existingIPs)
	if err != nil {
		return nil, fmt.Errorf("lb: create: allocate vip: %w", err)
	}

	configJSON, err := json.Marshal(spec.Config)
	if err != nil {
		return nil, fmt.Errorf("lb: create: marshal config: %w", err)
	}

	var lb LoadBalancer
	var rawConfig []byte
	err = s.pool.QueryRow(ctx,
		`INSERT INTO load_balancers (tenant_id, network_id, name, vip, config)
		 VALUES ($1, $2, $3, $4::inet, $5)
		 RETURNING id, tenant_id, network_id, name, host(vip), config, created_at, updated_at`,
		tenantID, networkID, spec.Name, vip, configJSON,
	).Scan(&lb.ID, &lb.TenantID, &lb.NetworkID, &lb.Name, &lb.VIP, &rawConfig, &lb.CreatedAt, &lb.UpdatedAt)
	if err != nil {
		return nil, wrapErr("lb: create", err)
	}
	if err := json.Unmarshal(rawConfig, &lb.Config); err != nil {
		return nil, fmt.Errorf("lb: create: unmarshal config: %w", err)
	}

	// Insert initial health rows for backends that have a VMID.
	for _, b := range spec.Config.Backends {
		if b.VMID == "" {
			continue
		}
		vmUUID, err := uuid.Parse(b.VMID)
		if err != nil {
			continue
		}
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO lb_backend_health (lb_id, vm_id, healthy, last_checked_at)
			VALUES ($1, $2, true, NOW())
			ON CONFLICT (lb_id, vm_id) DO NOTHING
		`, lb.ID, vmUUID)
	}

	return &lb, nil
}

func (s *Store) GetLoadBalancer(ctx context.Context, id uuid.UUID) (*LoadBalancer, error) {
	var lb LoadBalancer
	var rawConfig []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, network_id, name, host(vip), config, created_at, updated_at
		 FROM load_balancers WHERE id = $1`, id,
	).Scan(&lb.ID, &lb.TenantID, &lb.NetworkID, &lb.Name, &lb.VIP, &rawConfig, &lb.CreatedAt, &lb.UpdatedAt)
	if err != nil {
		return nil, wrapErr("lb: get", err)
	}
	if err := json.Unmarshal(rawConfig, &lb.Config); err != nil {
		return nil, fmt.Errorf("lb: get: unmarshal config: %w", err)
	}
	return &lb, nil
}

func (s *Store) ListLoadBalancers(ctx context.Context, networkID uuid.UUID) ([]LoadBalancer, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, network_id, name, host(vip), config, created_at, updated_at
		 FROM load_balancers WHERE network_id = $1 ORDER BY created_at`, networkID)
	if err != nil {
		return nil, fmt.Errorf("lb: list: %w", err)
	}
	defer rows.Close()

	var lbs []LoadBalancer
	for rows.Next() {
		var lb LoadBalancer
		var rawConfig []byte
		if err := rows.Scan(&lb.ID, &lb.TenantID, &lb.NetworkID, &lb.Name, &lb.VIP, &rawConfig, &lb.CreatedAt, &lb.UpdatedAt); err != nil {
			return nil, fmt.Errorf("lb: list: scan: %w", err)
		}
		if err := json.Unmarshal(rawConfig, &lb.Config); err != nil {
			return nil, fmt.Errorf("lb: list: unmarshal config: %w", err)
		}
		lbs = append(lbs, lb)
	}
	return lbs, rows.Err()
}

func (s *Store) DeleteLoadBalancer(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM load_balancers WHERE id = $1`, id)
	if err != nil {
		return wrapErr("lb: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("lb: delete: %w", ErrNotFound)
	}
	return nil
}

// UpdateLBBackendHealth upserts the health state for a backend VM in an internal LB.
func (s *Store) UpdateLBBackendHealth(ctx context.Context, lbID uuid.UUID, vmID uuid.UUID, healthy bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO lb_backend_health (lb_id, vm_id, healthy, last_checked_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (lb_id, vm_id) DO UPDATE
		  SET healthy = EXCLUDED.healthy,
		      last_checked_at = EXCLUDED.last_checked_at
	`, lbID, vmID, healthy)
	if err != nil {
		return fmt.Errorf("lb: update backend health: %w", err)
	}
	return nil
}

// ipInCIDR returns true if ip is within the given cidr string (e.g. "203.0.113.0/24").
func ipInCIDR(ipStr, cidrStr string) bool {
	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ipNet.Contains(ip)
}
