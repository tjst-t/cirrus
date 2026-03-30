package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements Service using PostgreSQL.
type Store struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewStore creates a new network store.
func NewStore(pool *pgxpool.Pool, logger *slog.Logger) *Store {
	return &Store{pool: pool, logger: logger}
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

	var n Network
	err := s.pool.QueryRow(ctx,
		`INSERT INTO networks (tenant_id, name, cidr, vni, status)
		 VALUES ($1, $2, $3, nextval('networks_vni_seq'), 'active')
		 RETURNING id, tenant_id, name, cidr::TEXT, vni, status, created_at, updated_at`,
		tenantID, spec.Name, cidr,
	).Scan(&n.ID, &n.TenantID, &n.Name, &n.CIDR, &n.VNI, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, wrapErr("network: create", err)
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

	tag, err := s.pool.Exec(ctx, `DELETE FROM networks WHERE id = $1`, id)
	if err != nil {
		return wrapErr("network: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("network: delete: %w", ErrNotFound)
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

// --- Ports (read-only) ---

func (s *Store) GetPort(ctx context.Context, id uuid.UUID) (*Port, error) {
	var p Port
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, network_id, group_id, vm_id, mac_address::TEXT, host(ip_address), host_id, role, status, created_at
		 FROM ports WHERE id = $1`, id,
	).Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.GroupID, &p.VMID, &p.MACAddress, &p.IPAddress, &p.HostID, &p.Role, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: get port", err)
	}
	return &p, nil
}

func (s *Store) ListPorts(ctx context.Context, networkID uuid.UUID) ([]Port, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, network_id, group_id, vm_id, mac_address::TEXT, host(ip_address), host_id, role, status, created_at
		 FROM ports WHERE network_id = $1 ORDER BY created_at`, networkID)
	if err != nil {
		return nil, fmt.Errorf("network: list ports: %w", err)
	}
	defer rows.Close()

	var ports []Port
	for rows.Next() {
		var p Port
		if err := rows.Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.GroupID, &p.VMID, &p.MACAddress, &p.IPAddress, &p.HostID, &p.Role, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("network: list ports scan: %w", err)
		}
		ports = append(ports, p)
	}
	return ports, rows.Err()
}
