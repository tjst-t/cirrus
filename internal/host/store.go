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

func (s *Store) RegisterOrGet(ctx context.Context, name, address, workerGRPCAddr, fabricIP, capability string) (*Host, bool, error) {
	var h Host
	cap := []byte(capability)
	if capability == "" {
		cap = []byte("{}")
	}

	// First, try to find an existing host with this name.
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, address, worker_grpc_addr, fabric_ip, operational_state, capability, resource_physical,
		        overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at
		 FROM hosts WHERE name = $1`, name,
	).Scan(&h.ID, &h.Name, &h.Address, &h.WorkerGRPCAddr, &h.FabricIP, &h.OperationalState, &h.Capability,
		&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
		&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt)

	if err == nil {
		// Host exists. If already activated and address differs, reject (different machine).
		if h.OperationalState != StateRegistering && h.Address != address {
			return nil, false, fmt.Errorf("host: register_or_get: hostname %q already registered from different address (%s): %w",
				name, h.Address, ErrConflict)
		}
		// Same host re-registering: update address/capability/worker_grpc_addr/fabric_ip if still registering.
		if h.OperationalState == StateRegistering {
			_, _ = s.pool.Exec(ctx,
				`UPDATE hosts SET address = $1, capability = $2, fabric_ip = $3, worker_grpc_addr = $4, updated_at = now() WHERE id = $5`,
				address, cap, fabricIP, workerGRPCAddr, h.ID)
			h.Address = address
			h.Capability = cap
			h.FabricIP = fabricIP
			h.WorkerGRPCAddr = workerGRPCAddr
		}
		return &h, false, nil
	}

	// Host does not exist: create new.
	err = s.pool.QueryRow(ctx,
		`INSERT INTO hosts (name, address, worker_grpc_addr, fabric_ip, operational_state, capability)
		 VALUES ($1, $2, $3, $4, 'registering', $5)
		 ON CONFLICT (name) DO UPDATE SET updated_at = now()
		 RETURNING id, name, address, worker_grpc_addr, fabric_ip, operational_state, capability, resource_physical,
		           overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at`,
		name, address, workerGRPCAddr, fabricIP, cap,
	).Scan(&h.ID, &h.Name, &h.Address, &h.WorkerGRPCAddr, &h.FabricIP, &h.OperationalState, &h.Capability,
		&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
		&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, false, wrapErr("host: register_or_get", err)
	}
	return &h, true, nil
}

func (s *Store) ListHostsByState(ctx context.Context, state OperationalState) ([]Host, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, address, worker_grpc_addr, fabric_ip, operational_state, capability, resource_physical,
		        overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at
		 FROM hosts WHERE operational_state = $1 ORDER BY name`, state)
	if err != nil {
		return nil, fmt.Errorf("host: list by state: %w", err)
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.ID, &h.Name, &h.Address, &h.WorkerGRPCAddr, &h.FabricIP, &h.OperationalState, &h.Capability,
			&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
			&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("host: list by state scan: %w", err)
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (s *Store) GetHost(ctx context.Context, id uuid.UUID) (*Host, error) {
	var h Host
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, address, worker_grpc_addr, fabric_ip, operational_state, capability, resource_physical,
		        overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at
		 FROM hosts WHERE id = $1`,
		id,
	).Scan(&h.ID, &h.Name, &h.Address, &h.WorkerGRPCAddr, &h.FabricIP, &h.OperationalState, &h.Capability,
		&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
		&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, wrapErr("host: get", err)
	}
	return &h, nil
}

func (s *Store) ListHosts(ctx context.Context) ([]Host, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, address, worker_grpc_addr, fabric_ip, operational_state, capability, resource_physical,
		        overcommit_ratios, resource_used, last_heartbeat, created_at, updated_at
		 FROM hosts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("host: list: %w", err)
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var h Host
		if err := rows.Scan(&h.ID, &h.Name, &h.Address, &h.WorkerGRPCAddr, &h.FabricIP, &h.OperationalState, &h.Capability,
			&h.ResourcePhysical, &h.OvercommitRatios, &h.ResourceUsed,
			&h.LastHeartbeat, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, fmt.Errorf("host: list scan: %w", err)
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (s *Store) DeleteHost(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM hosts WHERE id = $1 AND operational_state = 'retiring'`, id)
	if err != nil {
		return fmt.Errorf("host: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Check if host exists but is not in retiring state
		var state string
		err := s.pool.QueryRow(ctx, `SELECT operational_state FROM hosts WHERE id = $1`, id).Scan(&state)
		if err != nil {
			return fmt.Errorf("host: delete: %w", ErrNotFound)
		}
		return fmt.Errorf("host: delete: host is in %s state, must be retiring: %w", state, ErrInvalidState)
	}
	return nil
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

// validTransitions defines the allowed state transitions per host.md.
var validTransitions = map[OperationalState][]OperationalState{
	StateRegistering: {StateActive},
	StateActive:      {StateDraining, StateMaintenance, StateFaulty},
	StateDraining:    {StateActive, StateMaintenance, StateFaulty},
	StateMaintenance: {StateActive, StateRetiring},
	StateFaulty:      {StateActive, StateMaintenance},
	StateRetiring:    {}, // terminal state
}

func (s *Store) SetOperationalState(ctx context.Context, hostID uuid.UUID, state OperationalState) error {
	switch state {
	case StateActive, StateMaintenance, StateDraining, StateFaulty, StateRetiring:
		// valid target
	default:
		return fmt.Errorf("host: set operational state: %w: %s", ErrInvalidState, state)
	}

	// Fetch current state and validate transition
	var currentState OperationalState
	err := s.pool.QueryRow(ctx,
		`SELECT operational_state FROM hosts WHERE id = $1`, hostID,
	).Scan(&currentState)
	if err != nil {
		return wrapErr("host: set operational state", err)
	}

	allowed := false
	for _, target := range validTransitions[currentState] {
		if target == state {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("host: set operational state: transition %s → %s not allowed: %w", currentState, state, ErrInvalidState)
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE hosts SET operational_state = $1, updated_at = now()
		 WHERE id = $2 AND operational_state = $3`,
		state, hostID, currentState)
	if err != nil {
		return fmt.Errorf("host: set operational state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("host: set operational state: concurrent modification: %w", ErrInvalidState)
	}
	return nil
}

func (s *Store) Heartbeat(ctx context.Context, hostID string, report ResourceReport) error {
	id, err := uuid.Parse(hostID)
	if err != nil {
		return fmt.Errorf("host: heartbeat: invalid UUID %q: %w", hostID, err)
	}

	now := time.Now()
	resourceUsed, _ := json.Marshal(map[string]any{
		"vcpus":     report.UsedVcpus,
		"memory_mb": report.UsedRAMMB,
	})

	// registering状態: last_heartbeatのみ更新（activate前のheartbeat動作確認用）
	// active/draining/maintenance/faulty: last_heartbeat + resource_used更新
	// retiring: 拒否
	tag, err := s.pool.Exec(ctx,
		`UPDATE hosts SET
		   last_heartbeat = $1,
		   resource_used = CASE WHEN operational_state = 'registering' THEN resource_used ELSE $2 END,
		   updated_at = $1
		 WHERE id = $3 AND operational_state != 'retiring'`,
		now, resourceUsed, id)
	if err != nil {
		return fmt.Errorf("host: heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("host: heartbeat: %w", ErrNotFound)
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
