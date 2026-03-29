package az

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements Service using PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new AZ store.
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

const azColumns = `id, name, COALESCE(description, ''), location_id, enabled, created_at, updated_at`

func scanAZ(row pgx.Row) (*AvailabilityZone, error) {
	var az AvailabilityZone
	err := row.Scan(&az.ID, &az.Name, &az.Description, &az.LocationID, &az.Enabled, &az.CreatedAt, &az.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &az, nil
}

func (s *Store) Create(ctx context.Context, name, description string, locationID uuid.UUID) (*AvailabilityZone, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO availability_zones (name, description, location_id)
		 VALUES ($1, $2, $3)
		 RETURNING `+azColumns,
		name, nilIfEmpty(description), locationID)
	az, err := scanAZ(row)
	if err != nil {
		return nil, wrapErr("az: create", err)
	}
	return az, nil
}

func (s *Store) Get(ctx context.Context, id uuid.UUID) (*AvailabilityZone, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+azColumns+` FROM availability_zones WHERE id = $1`, id)
	az, err := scanAZ(row)
	if err != nil {
		return nil, wrapErr("az: get", err)
	}
	return az, nil
}

func (s *Store) List(ctx context.Context) ([]AvailabilityZone, error) {
	return s.queryAZs(ctx, `SELECT `+azColumns+` FROM availability_zones ORDER BY name`)
}

func (s *Store) ListEnabled(ctx context.Context) ([]AvailabilityZone, error) {
	return s.queryAZs(ctx, `SELECT `+azColumns+` FROM availability_zones WHERE enabled = true ORDER BY name`)
}

func (s *Store) queryAZs(ctx context.Context, query string, args ...any) ([]AvailabilityZone, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("az: list: %w", err)
	}
	defer rows.Close()

	var azs []AvailabilityZone
	for rows.Next() {
		var a AvailabilityZone
		if err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.LocationID, &a.Enabled, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("az: list scan: %w", err)
		}
		azs = append(azs, a)
	}
	return azs, rows.Err()
}

func (s *Store) Update(ctx context.Context, id uuid.UUID, name *string, description *string, enabled *bool) (*AvailabilityZone, error) {
	// Build dynamic update
	setClauses := "updated_at = now()"
	args := []any{id}
	argIdx := 2
	hasUpdate := false

	if name != nil {
		setClauses += fmt.Sprintf(", name = $%d", argIdx)
		args = append(args, *name)
		argIdx++
		hasUpdate = true
	}
	if description != nil {
		setClauses += fmt.Sprintf(", description = $%d", argIdx)
		args = append(args, nilIfEmpty(*description))
		argIdx++
		hasUpdate = true
	}
	if enabled != nil {
		setClauses += fmt.Sprintf(", enabled = $%d", argIdx)
		args = append(args, *enabled)
		hasUpdate = true
	}

	if !hasUpdate {
		return s.Get(ctx, id)
	}

	row := s.pool.QueryRow(ctx,
		`UPDATE availability_zones SET `+setClauses+` WHERE id = $1 RETURNING `+azColumns,
		args...)
	az, err := scanAZ(row)
	if err != nil {
		return nil, wrapErr("az: update", err)
	}
	return az, nil
}

func (s *Store) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM availability_zones WHERE id = $1`, id)
	if err != nil {
		return wrapErr("az: delete", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("az: delete: %w", ErrNotFound)
	}
	return nil
}

// --- Storage domain associations ---

func (s *Store) AddStorageDomain(ctx context.Context, azID, storageDomainID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO az_storage_domains (az_id, storage_domain_id) VALUES ($1, $2)
		 ON CONFLICT DO NOTHING`,
		azID, storageDomainID)
	if err != nil {
		return wrapErr("az: add storage domain", err)
	}
	return nil
}

func (s *Store) RemoveStorageDomain(ctx context.Context, azID, storageDomainID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM az_storage_domains WHERE az_id = $1 AND storage_domain_id = $2`,
		azID, storageDomainID)
	if err != nil {
		return fmt.Errorf("az: remove storage domain: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("az: remove storage domain: %w", ErrNotFound)
	}
	return nil
}

func (s *Store) ListStorageDomains(ctx context.Context, azID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT storage_domain_id FROM az_storage_domains WHERE az_id = $1`, azID)
	if err != nil {
		return nil, fmt.Errorf("az: list storage domains: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("az: list storage domains scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) GetDefault(ctx context.Context) (*AvailabilityZone, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+azColumns+` FROM availability_zones WHERE enabled = true ORDER BY created_at LIMIT 1`)
	az, err := scanAZ(row)
	if err != nil {
		return nil, wrapErr("az: get default", err)
	}
	return az, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
