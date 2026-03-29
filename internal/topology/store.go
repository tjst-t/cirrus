package topology

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements Service using PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new topology store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
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
			return fmt.Errorf("%s: referenced resource does not exist: %w", msg, ErrNotFound)
		}
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// --- Storage domains ---

func (s *Store) CreateStorageDomain(ctx context.Context, name string) (*StorageDomain, error) {
	var d StorageDomain
	err := s.pool.QueryRow(ctx,
		`INSERT INTO storage_domains (name) VALUES ($1)
		 RETURNING id, name, created_at, updated_at`,
		name,
	).Scan(&d.ID, &d.Name, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, wrapErr("topology: create storage domain", err)
	}
	return &d, nil
}

func (s *Store) GetStorageDomain(ctx context.Context, id uuid.UUID) (*StorageDomain, error) {
	var d StorageDomain
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, created_at, updated_at FROM storage_domains WHERE id = $1`, id,
	).Scan(&d.ID, &d.Name, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, wrapErr("topology: get storage domain", err)
	}
	return &d, nil
}

func (s *Store) ListStorageDomains(ctx context.Context) ([]StorageDomain, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, created_at, updated_at FROM storage_domains ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("topology: list storage domains: %w", err)
	}
	defer rows.Close()

	var domains []StorageDomain
	for rows.Next() {
		var d StorageDomain
		if err := rows.Scan(&d.ID, &d.Name, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("topology: list storage domains scan: %w", err)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// --- Locations ---

func (s *Store) CreateLocation(ctx context.Context, parentID *uuid.UUID, name string, locType LocationType, faultAttrs []byte) (*Location, error) {
	if !IsValidLocationType(locType) {
		return nil, fmt.Errorf("topology: create location: %w: %s", ErrInvalidType, locType)
	}

	// Validate parent exists if specified
	if parentID != nil {
		var exists bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM locations WHERE id = $1)`, *parentID).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("topology: create location: %w", err)
		}
		if !exists {
			return nil, fmt.Errorf("topology: create location: parent %s: %w", parentID, ErrInvalidParent)
		}
	}

	var loc Location
	err := s.pool.QueryRow(ctx,
		`INSERT INTO locations (parent_id, name, type, fault_attributes)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, parent_id, name, type, fault_attributes, created_at, updated_at`,
		parentID, name, locType, faultAttrs,
	).Scan(&loc.ID, &loc.ParentID, &loc.Name, &loc.Type, &loc.FaultAttributes, &loc.CreatedAt, &loc.UpdatedAt)
	if err != nil {
		return nil, wrapErr("topology: create location", err)
	}
	return &loc, nil
}

func (s *Store) GetLocation(ctx context.Context, id uuid.UUID) (*Location, error) {
	var loc Location
	err := s.pool.QueryRow(ctx,
		`SELECT id, parent_id, name, type, fault_attributes, created_at, updated_at
		 FROM locations WHERE id = $1`, id,
	).Scan(&loc.ID, &loc.ParentID, &loc.Name, &loc.Type, &loc.FaultAttributes, &loc.CreatedAt, &loc.UpdatedAt)
	if err != nil {
		return nil, wrapErr("topology: get location", err)
	}
	return &loc, nil
}

func (s *Store) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, parent_id, name, type, fault_attributes, created_at, updated_at
		 FROM locations ORDER BY type, name`)
	if err != nil {
		return nil, fmt.Errorf("topology: list locations: %w", err)
	}
	defer rows.Close()

	var locations []Location
	for rows.Next() {
		var loc Location
		if err := rows.Scan(&loc.ID, &loc.ParentID, &loc.Name, &loc.Type, &loc.FaultAttributes, &loc.CreatedAt, &loc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("topology: list locations scan: %w", err)
		}
		locations = append(locations, loc)
	}
	return locations, rows.Err()
}

func (s *Store) GetLocationPath(ctx context.Context, id uuid.UUID) ([]Location, error) {
	// Verify location exists
	if _, err := s.GetLocation(ctx, id); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`WITH RECURSIVE path AS (
			SELECT id, parent_id, name, type, fault_attributes, created_at, updated_at, 1 as depth
			FROM locations WHERE id = $1
			UNION ALL
			SELECT l.id, l.parent_id, l.name, l.type, l.fault_attributes, l.created_at, l.updated_at, p.depth + 1
			FROM locations l JOIN path p ON p.parent_id = l.id
		)
		SELECT id, parent_id, name, type, fault_attributes, created_at, updated_at
		FROM path ORDER BY depth DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("topology: get location path: %w", err)
	}
	defer rows.Close()

	var path []Location
	for rows.Next() {
		var loc Location
		if err := rows.Scan(&loc.ID, &loc.ParentID, &loc.Name, &loc.Type, &loc.FaultAttributes, &loc.CreatedAt, &loc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("topology: get location path scan: %w", err)
		}
		path = append(path, loc)
	}
	return path, rows.Err()
}

func (s *Store) GetLocationTree(ctx context.Context, id uuid.UUID) (*Location, error) {
	// Fetch root
	root, err := s.GetLocation(ctx, id)
	if err != nil {
		return nil, err
	}

	// Fetch all descendants
	rows, err := s.pool.Query(ctx,
		`WITH RECURSIVE tree AS (
			SELECT id, parent_id, name, type, fault_attributes, created_at, updated_at
			FROM locations WHERE id = $1
			UNION ALL
			SELECT l.id, l.parent_id, l.name, l.type, l.fault_attributes, l.created_at, l.updated_at
			FROM locations l JOIN tree t ON l.parent_id = t.id
		)
		SELECT id, parent_id, name, type, fault_attributes, created_at, updated_at
		FROM tree`, id)
	if err != nil {
		return nil, fmt.Errorf("topology: get location tree: %w", err)
	}
	defer rows.Close()

	all := map[uuid.UUID]*Location{}
	for rows.Next() {
		var loc Location
		if err := rows.Scan(&loc.ID, &loc.ParentID, &loc.Name, &loc.Type, &loc.FaultAttributes, &loc.CreatedAt, &loc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("topology: get location tree scan: %w", err)
		}
		all[loc.ID] = &loc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("topology: get location tree: %w", err)
	}

	// Build tree structure — use pointers so nested children propagate correctly
	for _, loc := range all {
		if loc.ParentID != nil {
			if parent, ok := all[*loc.ParentID]; ok {
				parent.Children = append(parent.Children, loc)
			}
		}
	}

	// Sort children by name for deterministic output
	for _, loc := range all {
		sort.Slice(loc.Children, func(i, j int) bool {
			return loc.Children[i].Name < loc.Children[j].Name
		})
	}

	// Return updated root from map (with children populated)
	if built, ok := all[root.ID]; ok {
		return built, nil
	}
	return root, nil
}

// --- Host-domain associations ---

func (s *Store) AssociateHostStorageDomain(ctx context.Context, hostID, storageDomainID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO host_storage_domains (host_id, storage_domain_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		hostID, storageDomainID)
	if err != nil {
		return wrapErr("topology: associate host storage domain", err)
	}
	return nil
}

func (s *Store) DissociateHostStorageDomain(ctx context.Context, hostID, storageDomainID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM host_storage_domains WHERE host_id = $1 AND storage_domain_id = $2`,
		hostID, storageDomainID)
	if err != nil {
		return fmt.Errorf("topology: dissociate host storage domain: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("topology: dissociate host storage domain: %w", ErrNotFound)
	}
	return nil
}

func (s *Store) SetHostLocation(ctx context.Context, hostID, locationID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE hosts SET location_id = $1, updated_at = now() WHERE id = $2`,
		locationID, hostID)
	if err != nil {
		return wrapErr("topology: set host location", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("topology: set host location: %w", ErrNotFound)
	}
	return nil
}

// --- Compute pool derivation ---

func (s *Store) GetComputePool(ctx context.Context, storageDomainID uuid.UUID) (*ComputePool, error) {
	sd, err := s.GetStorageDomain(ctx, storageDomainID)
	if err != nil {
		return nil, fmt.Errorf("topology: get compute pool: storage domain: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT h.id FROM hosts h
		 JOIN host_storage_domains hsd ON h.id = hsd.host_id
		 WHERE hsd.storage_domain_id = $1
		 ORDER BY h.name`,
		storageDomainID)
	if err != nil {
		return nil, fmt.Errorf("topology: get compute pool: %w", err)
	}
	defer rows.Close()

	var hostIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("topology: get compute pool scan: %w", err)
		}
		hostIDs = append(hostIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("topology: get compute pool: %w", err)
	}

	if hostIDs == nil {
		hostIDs = []uuid.UUID{}
	}

	return &ComputePool{
		StorageDomainID:   sd.ID,
		StorageDomainName: sd.Name,
		HostIDs:           hostIDs,
		Count:             len(hostIDs),
	}, nil
}

// --- Fault domain derivation ---

func (s *Store) GetFaultDomains(ctx context.Context, level LocationType) ([]FaultDomain, error) {
	if !IsValidLocationType(level) {
		return nil, fmt.Errorf("topology: get fault domains: %w: %s", ErrInvalidType, level)
	}

	// For each location at the given level, find all hosts whose location_id
	// is either that location itself or any descendant of it.
	rows, err := s.pool.Query(ctx,
		`WITH fd_locations AS (
			SELECT id, name FROM locations WHERE type = $1
		),
		descendant_hosts AS (
			SELECT fl.id AS fd_id, fl.name AS fd_name, h.id AS host_id
			FROM fd_locations fl
			JOIN LATERAL (
				WITH RECURSIVE subtree AS (
					SELECT id FROM locations WHERE id = fl.id
					UNION ALL
					SELECT l.id FROM locations l JOIN subtree st ON l.parent_id = st.id
				)
				SELECT id FROM hosts WHERE location_id IN (SELECT id FROM subtree)
			) h ON true
		)
		SELECT fd_id, fd_name, array_agg(host_id ORDER BY host_id) AS host_ids
		FROM descendant_hosts
		GROUP BY fd_id, fd_name
		ORDER BY fd_name`, level)
	if err != nil {
		return nil, fmt.Errorf("topology: get fault domains: %w", err)
	}
	defer rows.Close()

	var fds []FaultDomain
	for rows.Next() {
		var fd FaultDomain
		fd.Level = level
		if err := rows.Scan(&fd.LocationID, &fd.LocationName, &fd.HostIDs); err != nil {
			return nil, fmt.Errorf("topology: get fault domains scan: %w", err)
		}
		fd.Count = len(fd.HostIDs)
		fds = append(fds, fd)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("topology: get fault domains: %w", err)
	}

	// Include locations at the level that have no hosts (empty fault domains)
	locRows, err := s.pool.Query(ctx,
		`SELECT id, name FROM locations WHERE type = $1 ORDER BY name`, level)
	if err != nil {
		return nil, fmt.Errorf("topology: get fault domains: %w", err)
	}
	defer locRows.Close()

	existing := make(map[uuid.UUID]bool, len(fds))
	for _, fd := range fds {
		existing[fd.LocationID] = true
	}
	for locRows.Next() {
		var id uuid.UUID
		var name string
		if err := locRows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("topology: get fault domains scan: %w", err)
		}
		if !existing[id] {
			fds = append(fds, FaultDomain{
				LocationID:   id,
				LocationName: name,
				Level:        level,
				HostIDs:      []uuid.UUID{},
				Count:        0,
			})
		}
	}

	// Sort by name for deterministic output
	sort.Slice(fds, func(i, j int) bool {
		return fds[i].LocationName < fds[j].LocationName
	})

	return fds, locRows.Err()
}

// --- Reachability queries ---

func (s *Store) ListReachableHosts(ctx context.Context, storageDomainID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT host_id FROM host_storage_domains WHERE storage_domain_id = $1`,
		storageDomainID)
	if err != nil {
		return nil, fmt.Errorf("topology: list reachable hosts: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("topology: list reachable hosts scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) ListReachableBackends(ctx context.Context, hostID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT storage_domain_id FROM host_storage_domains WHERE host_id = $1`,
		hostID)
	if err != nil {
		return nil, fmt.Errorf("topology: list reachable backends: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("topology: list reachable backends scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
