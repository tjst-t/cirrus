package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements Service using PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new identity store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// wrapErr converts pgx.ErrNoRows to ErrNotFound, wraps others.
func wrapErr(msg string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", msg, ErrNotFound)
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func (s *Store) CreateOrganization(ctx context.Context, name string) (*Organization, error) {
	var org Organization
	err := s.pool.QueryRow(ctx,
		`INSERT INTO organizations (name) VALUES ($1)
		 RETURNING id, name, quota_vcpus, quota_ram_mb, quota_volume_gb, created_at, updated_at`,
		name,
	).Scan(&org.ID, &org.Name, &org.QuotaVcpus, &org.QuotaRAMMB, &org.QuotaVolumeGB, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, wrapErr("identity: create organization", err)
	}
	return &org, nil
}

func (s *Store) GetOrganization(ctx context.Context, id uuid.UUID) (*Organization, error) {
	var org Organization
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, quota_vcpus, quota_ram_mb, quota_volume_gb, created_at, updated_at
		 FROM organizations WHERE id = $1`,
		id,
	).Scan(&org.ID, &org.Name, &org.QuotaVcpus, &org.QuotaRAMMB, &org.QuotaVolumeGB, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, wrapErr("identity: get organization", err)
	}
	return &org, nil
}

func (s *Store) ListOrganizations(ctx context.Context) ([]Organization, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, quota_vcpus, quota_ram_mb, quota_volume_gb, created_at, updated_at
		 FROM organizations ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("identity: list organizations: %w", err)
	}
	defer rows.Close()

	var orgs []Organization
	for rows.Next() {
		var org Organization
		if err := rows.Scan(&org.ID, &org.Name, &org.QuotaVcpus, &org.QuotaRAMMB, &org.QuotaVolumeGB, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, fmt.Errorf("identity: list organizations scan: %w", err)
		}
		orgs = append(orgs, org)
	}
	return orgs, rows.Err()
}

func (s *Store) CreateTenant(ctx context.Context, orgID uuid.UUID, name string) (*Tenant, error) {
	var t Tenant
	err := s.pool.QueryRow(ctx,
		`INSERT INTO tenants (organization_id, name) VALUES ($1, $2)
		 RETURNING id, organization_id, name, quota_vcpus, quota_ram_mb, quota_volume_gb,
		           quota_vms, quota_volumes, quota_snapshots, quota_networks, quota_floating_ips,
		           created_at, updated_at`,
		orgID, name,
	).Scan(&t.ID, &t.OrganizationID, &t.Name, &t.QuotaVcpus, &t.QuotaRAMMB, &t.QuotaVolumeGB,
		&t.QuotaVMs, &t.QuotaVolumes, &t.QuotaSnapshots, &t.QuotaNetworks, &t.QuotaFloatingIPs,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, wrapErr("identity: create tenant", err)
	}
	return &t, nil
}

func (s *Store) GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	var t Tenant
	err := s.pool.QueryRow(ctx,
		`SELECT id, organization_id, name, quota_vcpus, quota_ram_mb, quota_volume_gb,
		        quota_vms, quota_volumes, quota_snapshots, quota_networks, quota_floating_ips,
		        created_at, updated_at
		 FROM tenants WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.OrganizationID, &t.Name, &t.QuotaVcpus, &t.QuotaRAMMB, &t.QuotaVolumeGB,
		&t.QuotaVMs, &t.QuotaVolumes, &t.QuotaSnapshots, &t.QuotaNetworks, &t.QuotaFloatingIPs,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, wrapErr("identity: get tenant", err)
	}
	return &t, nil
}

func (s *Store) ListTenants(ctx context.Context, orgID uuid.UUID) ([]Tenant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, organization_id, name, quota_vcpus, quota_ram_mb, quota_volume_gb,
		        quota_vms, quota_volumes, quota_snapshots, quota_networks, quota_floating_ips,
		        created_at, updated_at
		 FROM tenants WHERE organization_id = $1 ORDER BY created_at`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("identity: list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.OrganizationID, &t.Name, &t.QuotaVcpus, &t.QuotaRAMMB, &t.QuotaVolumeGB,
			&t.QuotaVMs, &t.QuotaVolumes, &t.QuotaSnapshots, &t.QuotaNetworks, &t.QuotaFloatingIPs,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("identity: list tenants scan: %w", err)
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, externalID, name, email string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (external_id, name, email) VALUES ($1, $2, $3)
		 RETURNING id, external_id, name, email, created_at, updated_at`,
		externalID, name, email,
	).Scan(&u.ID, &u.ExternalID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, wrapErr("identity: create user", err)
	}
	return &u, nil
}

func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, external_id, name, email, created_at, updated_at
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.ExternalID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, wrapErr("identity: get user", err)
	}
	return &u, nil
}

func (s *Store) GetUserByExternalID(ctx context.Context, externalID string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, external_id, name, email, created_at, updated_at
		 FROM users WHERE external_id = $1`,
		externalID,
	).Scan(&u.ID, &u.ExternalID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, wrapErr("identity: get user by external_id", err)
	}
	return &u, nil
}

func (s *Store) AssignRole(ctx context.Context, userID uuid.UUID, scopeType ScopeType, scopeID *uuid.UUID, role Role) (*RoleAssignment, error) {
	var ra RoleAssignment
	err := s.pool.QueryRow(ctx,
		`INSERT INTO role_assignments (user_id, scope_type, scope_id, role) VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, scope_type, scope_id, role, created_at`,
		userID, scopeType, scopeID, role,
	).Scan(&ra.ID, &ra.UserID, &ra.ScopeType, &ra.ScopeID, &ra.Role, &ra.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("identity: assign role: %w", err)
	}
	return &ra, nil
}

func (s *Store) ListRoleAssignments(ctx context.Context, userID uuid.UUID) ([]RoleAssignment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, scope_type, scope_id, role, created_at
		 FROM role_assignments WHERE user_id = $1 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("identity: list role assignments: %w", err)
	}
	defer rows.Close()
	return scanRoleAssignments(rows)
}

func (s *Store) ListRoleAssignmentsByScope(ctx context.Context, scopeType ScopeType, scopeID uuid.UUID) ([]RoleAssignment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, scope_type, scope_id, role, created_at
		 FROM role_assignments WHERE scope_type = $1 AND scope_id = $2 ORDER BY created_at`,
		scopeType, scopeID,
	)
	if err != nil {
		return nil, fmt.Errorf("identity: list role assignments by scope: %w", err)
	}
	defer rows.Close()
	return scanRoleAssignments(rows)
}

func (s *Store) DeleteRoleAssignment(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM role_assignments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("identity: delete role assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("identity: delete role assignment: %w", ErrNotFound)
	}
	return nil
}

func scanRoleAssignments(rows pgx.Rows) ([]RoleAssignment, error) {
	var assignments []RoleAssignment
	for rows.Next() {
		var ra RoleAssignment
		if err := rows.Scan(&ra.ID, &ra.UserID, &ra.ScopeType, &ra.ScopeID, &ra.Role, &ra.CreatedAt); err != nil {
			return nil, fmt.Errorf("identity: scan role assignment: %w", err)
		}
		assignments = append(assignments, ra)
	}
	return assignments, rows.Err()
}
