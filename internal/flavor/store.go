package flavor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a flavor is not found.
var ErrNotFound = errors.New("flavor not found")

type serviceImpl struct {
	db *pgxpool.Pool
}

// NewService creates a new flavor service backed by PostgreSQL.
func NewService(db *pgxpool.Pool) Service {
	return &serviceImpl{db: db}
}

func (s *serviceImpl) Create(ctx context.Context, spec CreateFlavorSpec) (*Flavor, error) {
	f := &Flavor{}
	err := s.db.QueryRow(ctx, `
		INSERT INTO flavors (name, vcpus, ram_mb, disk_gb, is_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, vcpus, ram_mb, disk_gb, is_public, created_at, updated_at`,
		spec.Name, spec.VCPUs, spec.RAMMB, spec.DiskGB, spec.IsPublic,
	).Scan(&f.ID, &f.Name, &f.VCPUs, &f.RAMMB, &f.DiskGB, &f.IsPublic, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("flavor: create: %w", err)
	}
	return f, nil
}

func (s *serviceImpl) Get(ctx context.Context, id uuid.UUID) (*Flavor, error) {
	f := &Flavor{}
	err := s.db.QueryRow(ctx, `
		SELECT id, name, vcpus, ram_mb, disk_gb, is_public, created_at, updated_at
		FROM flavors WHERE id = $1`, id,
	).Scan(&f.ID, &f.Name, &f.VCPUs, &f.RAMMB, &f.DiskGB, &f.IsPublic, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("flavor: get: %w", err)
	}
	return f, nil
}

func (s *serviceImpl) List(ctx context.Context) ([]Flavor, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, vcpus, ram_mb, disk_gb, is_public, created_at, updated_at
		FROM flavors ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("flavor: list: %w", err)
	}
	defer rows.Close()

	var flavors []Flavor
	for rows.Next() {
		var f Flavor
		if err := rows.Scan(&f.ID, &f.Name, &f.VCPUs, &f.RAMMB, &f.DiskGB, &f.IsPublic, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("flavor: list scan: %w", err)
		}
		flavors = append(flavors, f)
	}
	if flavors == nil {
		flavors = []Flavor{}
	}
	return flavors, nil
}

func (s *serviceImpl) ListPage(ctx context.Context, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]Flavor, error) {
	scanFlavors := func(query string, args ...any) ([]Flavor, error) {
		rows, err := s.db.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("flavor: list page: %w", err)
		}
		defer rows.Close()
		var flavors []Flavor
		for rows.Next() {
			var f Flavor
			if err := rows.Scan(&f.ID, &f.Name, &f.VCPUs, &f.RAMMB, &f.DiskGB, &f.IsPublic, &f.CreatedAt, &f.UpdatedAt); err != nil {
				return nil, fmt.Errorf("flavor: list page scan: %w", err)
			}
			flavors = append(flavors, f)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("flavor: list page rows: %w", err)
		}
		if flavors == nil {
			flavors = []Flavor{}
		}
		return flavors, nil
	}

	if afterCreatedAt.IsZero() {
		return scanFlavors(`
			SELECT id, name, vcpus, ram_mb, disk_gb, is_public, created_at, updated_at
			FROM flavors ORDER BY created_at, id LIMIT $1`, limit)
	}
	return scanFlavors(`
		SELECT id, name, vcpus, ram_mb, disk_gb, is_public, created_at, updated_at
		FROM flavors
		WHERE (created_at, id) > ($1, $2)
		ORDER BY created_at, id LIMIT $3`, afterCreatedAt, afterID, limit)
}

func (s *serviceImpl) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM flavors WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("flavor: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
