package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements Service using PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new host store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func wrapErr(msg string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", msg, ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%s: %w", msg, ErrConflict)
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func (s *Store) Register(ctx context.Context, name, address string) (*Host, error) {
	var h Host
	err := s.pool.QueryRow(ctx,
		`INSERT INTO hosts (name, address, operational_state)
		 VALUES ($1, $2, 'registering')
		 RETURNING id, name, address, operational_state, capability, resource_physical,
		           overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at`,
		name, address,
	).Scan(&h.ID, &h.Name, &h.Address, &h.OperationalState, &h.Capability,
		&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
		&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, wrapErr("host: register", err)
	}
	return &h, nil
}

func (s *Store) GetHost(ctx context.Context, id uuid.UUID) (*Host, error) {
	var h Host
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, address, operational_state, capability, resource_physical,
		        overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at
		 FROM hosts WHERE id = $1`,
		id,
	).Scan(&h.ID, &h.Name, &h.Address, &h.OperationalState, &h.Capability,
		&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
		&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, wrapErr("host: get", err)
	}
	return &h, nil
}

func (s *Store) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, address, operational_state, capability, resource_physical,
		        overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at
		 FROM hosts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("host: list: %w", err)
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.ID, &h.Name, &h.Address, &h.OperationalState, &h.Capability,
			&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
			&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("host: list scan: %w", err)
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (s *Store) UpdateCapability(ctx context.Context, hostID uuid.UUID, capability []byte) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE hosts SET capability = $1, updated_at = now() WHERE id = $2`,
		capability, hostID)
	if err != nil {
		return fmt.Errorf("host: update capability: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("host: update capability: %w", ErrNotFound)
	}
	return nil
}

func (s *Store) UpdateResourcePhysical(ctx context.Context, hostID uuid.UUID, resources []byte) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE hosts SET resource_physical = $1, updated_at = now() WHERE id = $2`,
		resources, hostID)
	if err != nil {
		return fmt.Errorf("host: update resource_physical: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("host: update resource_physical: %w", ErrNotFound)
	}
	return nil
}

func (s *Store) UpdateOvercommitRatios(ctx context.Context, hostID uuid.UUID, ratios []byte) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE hosts SET overcommit_ratios = $1, updated_at = now() WHERE id = $2`,
		ratios, hostID)
	if err != nil {
		return fmt.Errorf("host: update overcommit_ratios: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("host: update overcommit_ratios: %w", ErrNotFound)
	}
	return nil
}

func (s *Store) SetOperationalState(ctx context.Context, hostID uuid.UUID, state OperationalState) error {
	switch state {
	case StateActive, StateMaintenance, StateDraining, StateFaulty, StateRetiring:
		// valid
	default:
		return fmt.Errorf("host: set operational state: %w: %s", ErrInvalidState, state)
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE hosts SET operational_state = $1, updated_at = now() WHERE id = $2`,
		state, hostID)
	if err != nil {
		return fmt.Errorf("host: set operational state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("host: set operational state: %w", ErrNotFound)
	}
	return nil
}

func (s *Store) Heartbeat(ctx context.Context, hostID string, report ResourceReport) error {
	now := time.Now()
	resourceUsed, _ := json.Marshal(map[string]any{
		"vcpus":     report.UsedVcpus,
		"memory_mb": report.UsedRAMMB,
	})

	// Try matching by UUID first, fall back to name
	var query string
	var param any
	if id, err := uuid.Parse(hostID); err == nil {
		query = `UPDATE hosts SET last_heartbeat = $1, resource_used = $2, updated_at = $1 WHERE id = $3`
		param = id
	} else {
		query = `UPDATE hosts SET last_heartbeat = $1, resource_used = $2, updated_at = $1 WHERE name = $3`
		param = hostID
	}

	_, err := s.pool.Exec(ctx, query, now, resourceUsed, param)
	if err != nil {
		return fmt.Errorf("host: heartbeat: %w", err)
	}
	return nil
}

func (s *Store) GetAllocatable(ctx context.Context, hostID uuid.UUID) (*AllocatableResources, error) {
	h, err := s.GetHost(ctx, hostID)
	if err != nil {
		return nil, err
	}

	var physical PhysicalResources
	var overcommit OvercommitRatios
	var used ResourceReport

	if err := json.Unmarshal(h.ResourcePhysical, &physical); err != nil {
		physical = PhysicalResources{}
	}
	if err := json.Unmarshal(h.OvercommitRatios, &overcommit); err != nil {
		overcommit = OvercommitRatios{Vcpus: 4.0, MemoryMB: 1.5}
	}
	if err := json.Unmarshal(h.ResourceUsed, &used); err != nil {
		used = ResourceReport{}
	}

	allocatable := &AllocatableResources{
		Vcpus:    float64(physical.Vcpus)*overcommit.Vcpus - float64(used.UsedVcpus),
		MemoryMB: float64(physical.MemoryMB)*overcommit.MemoryMB - float64(used.UsedRAMMB),
	}

	return allocatable, nil
}
