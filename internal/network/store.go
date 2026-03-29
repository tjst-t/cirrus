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
	var n Network
	err := s.pool.QueryRow(ctx,
		`INSERT INTO networks (tenant_id, name, status)
		 VALUES ($1, $2, 'active')
		 RETURNING id, tenant_id, name, status, created_at, updated_at`,
		tenantID, spec.Name,
	).Scan(&n.ID, &n.TenantID, &n.Name, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, wrapErr("network: create", err)
	}
	return &n, nil
}

func (s *Store) GetNetwork(ctx context.Context, id uuid.UUID) (*Network, error) {
	var n Network
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, status, created_at, updated_at
		 FROM networks WHERE id = $1`, id,
	).Scan(&n.ID, &n.TenantID, &n.Name, &n.Status, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return nil, wrapErr("network: get", err)
	}
	return &n, nil
}

func (s *Store) ListNetworks(ctx context.Context, tenantID uuid.UUID) ([]Network, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, name, status, created_at, updated_at
		 FROM networks WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("network: list: %w", err)
	}
	defer rows.Close()

	var networks []Network
	for rows.Next() {
		var n Network
		if err := rows.Scan(&n.ID, &n.TenantID, &n.Name, &n.Status, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("network: list scan: %w", err)
		}
		networks = append(networks, n)
	}
	return networks, rows.Err()
}

func (s *Store) DeleteNetwork(ctx context.Context, id uuid.UUID) error {
	// Check for dependent ports
	var portCount int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM ports WHERE network_id = $1`, id).Scan(&portCount); err != nil {
		return fmt.Errorf("network: delete: %w", err)
	}
	if portCount > 0 {
		return fmt.Errorf("network: delete: %d ports still attached: %w", portCount, ErrHasDependents)
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

// --- Ports (read-only) ---

func (s *Store) GetPort(ctx context.Context, id uuid.UUID) (*Port, error) {
	var p Port
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, network_id, vm_id, mac_address::TEXT, host(ip_address), status, created_at
		 FROM ports WHERE id = $1`, id,
	).Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.VMID, &p.MACAddress, &p.IPAddress, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, wrapErr("network: get port", err)
	}
	return &p, nil
}

func (s *Store) ListPorts(ctx context.Context, networkID uuid.UUID) ([]Port, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, network_id, vm_id, mac_address::TEXT, host(ip_address), status, created_at
		 FROM ports WHERE network_id = $1 ORDER BY created_at`, networkID)
	if err != nil {
		return nil, fmt.Errorf("network: list ports: %w", err)
	}
	defer rows.Close()

	var ports []Port
	for rows.Next() {
		var p Port
		if err := rows.Scan(&p.ID, &p.TenantID, &p.NetworkID, &p.VMID, &p.MACAddress, &p.IPAddress, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("network: list ports scan: %w", err)
		}
		ports = append(ports, p)
	}
	return ports, rows.Err()
}
