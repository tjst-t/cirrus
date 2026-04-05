package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/quota"
)

// Store implements Service using PostgreSQL.
type Store struct {
	pool     *pgxpool.Pool
	logger   *slog.Logger
	quotaSvc quota.Service
}

// NewStore creates a new network store.
func NewStore(pool *pgxpool.Pool, logger *slog.Logger, quotaSvc quota.Service) *Store {
	return &Store{pool: pool, logger: logger, quotaSvc: quotaSvc}
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
		 RETURNING id, network_id, name`,
		networkID, spec.Name,
	).Scan(&g.ID, &g.NetworkID, &g.Name)
	if err != nil {
		return nil, wrapErr("group: create", err)
	}
	return &g, nil
}

func (s *Store) GetGroup(ctx context.Context, id uuid.UUID) (*Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx,
		`SELECT id, network_id, name FROM groups WHERE id = $1`, id,
	).Scan(&g.ID, &g.NetworkID, &g.Name)
	if err != nil {
		return nil, wrapErr("group: get", err)
	}
	return &g, nil
}

func (s *Store) ListGroups(ctx context.Context, networkID uuid.UUID) ([]Group, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, network_id, name FROM groups WHERE network_id = $1 ORDER BY name`, networkID)
	if err != nil {
		return nil, fmt.Errorf("group: list: %w", err)
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.NetworkID, &g.Name); err != nil {
			return nil, fmt.Errorf("group: list scan: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
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
		 RETURNING id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action`,
		networkID, spec.SrcGroupID, spec.DstGroupID, spec.Protocol, spec.DstPort, priority, action,
	).Scan(&p.ID, &p.NetworkID, &p.SrcGroupID, &p.DstGroupID, &p.Protocol, &p.DstPort, &p.Priority, &p.Action)
	if err != nil {
		return nil, wrapErr("policy: create", err)
	}
	return &p, nil
}

func (s *Store) GetPolicy(ctx context.Context, id uuid.UUID) (*Policy, error) {
	var p Policy
	err := s.pool.QueryRow(ctx,
		`SELECT id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action
		 FROM policies WHERE id = $1`, id,
	).Scan(&p.ID, &p.NetworkID, &p.SrcGroupID, &p.DstGroupID, &p.Protocol, &p.DstPort, &p.Priority, &p.Action)
	if err != nil {
		return nil, wrapErr("policy: get", err)
	}
	return &p, nil
}

func (s *Store) ListPolicies(ctx context.Context, networkID uuid.UUID) ([]Policy, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action
		 FROM policies WHERE network_id = $1 ORDER BY priority, id`, networkID)
	if err != nil {
		return nil, fmt.Errorf("policy: list: %w", err)
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var p Policy
		if err := rows.Scan(&p.ID, &p.NetworkID, &p.SrcGroupID, &p.DstGroupID, &p.Protocol, &p.DstPort, &p.Priority, &p.Action); err != nil {
			return nil, fmt.Errorf("policy: list scan: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
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

	var p Port
	err = s.pool.QueryRow(ctx,
		`INSERT INTO ports (network_id, group_id, tenant_id, host_id, vm_id, mac_address, ip_address, vm_name, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6::macaddr, $7::inet, $8, 'active', NOW())
		 RETURNING id, tenant_id, network_id, group_id, vm_id, vm_name, mac_address::TEXT, host(ip_address), host_id, role, status, created_at`,
		spec.NetworkID, spec.GroupID, spec.TenantID, spec.HostID, spec.VMID, mac, vmIP, spec.VMName,
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
		`INSERT INTO gateway_nodes (host_id, external_ip, internal_ip, status)
		 VALUES ($1, $2::inet, $3::inet, 'active')
		 RETURNING id, host_id, host(external_ip), host(internal_ip), status, created_at`,
		spec.HostID, spec.ExternalIP, spec.InternalIP,
	).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.Status, &gw.CreatedAt)
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
		`SELECT id, host_id, host(external_ip), host(internal_ip), status, created_at
		 FROM gateway_nodes WHERE id = $1`, id,
	).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.Status, &gw.CreatedAt)
	if err != nil {
		return nil, wrapErr("gateway_node: get", err)
	}
	return &gw, nil
}

func (s *Store) ListGatewayNodes(ctx context.Context) ([]GatewayNode, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, host_id, host(external_ip), host(internal_ip), status, created_at
		 FROM gateway_nodes ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("gateway_node: list: %w", err)
	}
	defer rows.Close()

	var nodes []GatewayNode
	for rows.Next() {
		var gw GatewayNode
		if err := rows.Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.Status, &gw.CreatedAt); err != nil {
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
		`SELECT gn.id, gn.host_id, host(gn.external_ip), host(gn.internal_ip), gn.status, gn.created_at
		 FROM gateway_nodes gn
		 JOIN networks n ON n.gateway_node_id = gn.id
		 WHERE n.id = $1`, networkID,
	).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.Status, &gw.CreatedAt)
	if err != nil {
		return nil, wrapErr("gateway_node: get network gateway", err)
	}
	return &gw, nil
}

// --- Egresses ---

func (s *Store) CreateEgress(ctx context.Context, networkID uuid.UUID, spec EgressSpec) (*Egress, error) {
	if spec.Type != "nat_gateway" {
		return nil, fmt.Errorf("egress: create: unsupported type %q: %w", spec.Type, ErrInvalidState)
	}

	configJSON, err := json.Marshal(spec.Config)
	if err != nil {
		return nil, fmt.Errorf("egress: create: marshal config: %w", err)
	}

	var e Egress
	e.NetworkID = networkID
	e.Type = spec.Type
	err = s.pool.QueryRow(ctx,
		`INSERT INTO egresses (network_id, type, config)
		 VALUES ($1, $2, $3)
		 RETURNING id, network_id, type, config`,
		networkID, spec.Type, configJSON,
	).Scan(&e.ID, &e.NetworkID, &e.Type, &configJSON)
	if err != nil {
		return nil, wrapErr("egress: create", err)
	}

	if err := json.Unmarshal(configJSON, &e.Config); err != nil {
		return nil, fmt.Errorf("egress: create: unmarshal config: %w", err)
	}
	return &e, nil
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

func (s *Store) DeleteEgress(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM egresses WHERE id = $1`, id)
	if err != nil {
		return wrapErr("egress: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("egress: delete: %w", ErrNotFound)
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
	if spec.Type != "direct_ip" {
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

	configJSON, err := json.Marshal(spec.Config)
	if err != nil {
		return nil, fmt.Errorf("ingress: create: marshal config: %w", err)
	}

	var ing Ingress
	err = s.pool.QueryRow(ctx,
		`INSERT INTO ingresses (network_id, type, public_ip, ip_pool_id, config)
		 VALUES ($1, $2, $3::inet, $4, $5)
		 RETURNING id, network_id, type, host(public_ip), ip_pool_id, config, created_at`,
		networkID, spec.Type, spec.PublicIP, spec.IPPoolID, configJSON,
	).Scan(&ing.ID, &ing.NetworkID, &ing.Type, &ing.PublicIP, &ing.IPPoolID, &configJSON, &ing.CreatedAt)
	if err != nil {
		return nil, wrapErr("ingress: create", err)
	}
	if err := json.Unmarshal(configJSON, &ing.Config); err != nil {
		return nil, fmt.Errorf("ingress: create: unmarshal config: %w", err)
	}
	return &ing, nil
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
	if err := json.Unmarshal(configJSON, &ing.Config); err != nil {
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
		if err := json.Unmarshal(configJSON, &ing.Config); err != nil {
			return nil, fmt.Errorf("ingress: list: unmarshal config: %w", err)
		}
		ingresses = append(ingresses, ing)
	}
	return ingresses, rows.Err()
}

func (s *Store) DeleteIngress(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ingresses WHERE id = $1`, id)
	if err != nil {
		return wrapErr("ingress: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ingress: delete: %w", ErrNotFound)
	}
	return nil
}

// ipInCIDR returns true if ip is within the given cidr string (e.g. "203.0.113.0/24").
func ipInCIDR(ipStr, cidrStr string) bool {
	_, network, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return network.Contains(ip)
}
