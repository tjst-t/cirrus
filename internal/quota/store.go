package quota

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

// NewStore creates a new quota store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Check verifies that adding delta to current usage (including in-flight reserves)
// would not exceed tenant or org limits. Read-only.
func (s *Store) Check(ctx context.Context, tenantID uuid.UUID, delta ResourceDelta) error {
	return s.checkLimits(ctx, tenantID, delta)
}

// Reserve atomically inserts a reservation and verifies limits. Rolls back if exceeded.
func (s *Store) Reserve(ctx context.Context, tenantID uuid.UUID, resourceType ResourceType, resourceID uuid.UUID, delta ResourceDelta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("quota: reserve: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Insert reservation.
	_, err = tx.Exec(ctx,
		`INSERT INTO quota_reserves
		 (tenant_id, resource_type, resource_id, vcpus, ram_mb, volume_gb, vms, volumes, snapshots, networks, egresses, ingresses)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		tenantID, string(resourceType), resourceID,
		delta.Vcpus, delta.RAMMB, delta.VolumeGB,
		delta.VMs, delta.Volumes, delta.Snapshots, delta.Networks,
		delta.Egresses, delta.Ingresses,
	)
	if err != nil {
		return fmt.Errorf("quota: reserve: insert: %w", err)
	}

	// Verify limits within the same transaction.
	if err := s.checkLimitsTx(ctx, tx, tenantID, ResourceDelta{}); err != nil {
		return err // includes ErrQuotaExceeded; tx will be rolled back by defer
	}

	return tx.Commit(ctx)
}

// Commit moves a reservation into committed usage.
func (s *Store) Commit(ctx context.Context, resourceType ResourceType, resourceID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("quota: commit: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var r struct {
		tenantID  uuid.UUID
		vcpus     int
		ramMB     int
		volumeGB  int
		vms       int
		volumes   int
		snapshots int
		networks  int
		egresses  int
		ingresses int
	}
	err = tx.QueryRow(ctx,
		`DELETE FROM quota_reserves
		 WHERE resource_type = $1 AND resource_id = $2
		 RETURNING tenant_id, vcpus, ram_mb, volume_gb, vms, volumes, snapshots, networks, egresses, ingresses`,
		string(resourceType), resourceID,
	).Scan(&r.tenantID, &r.vcpus, &r.ramMB, &r.volumeGB, &r.vms, &r.volumes, &r.snapshots, &r.networks, &r.egresses, &r.ingresses)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrReservationNotFound
		}
		return fmt.Errorf("quota: commit: delete reserve: %w", err)
	}

	// Upsert usage.
	_, err = tx.Exec(ctx,
		`INSERT INTO quota_usage
		 (tenant_id, vcpus_used, ram_mb_used, volume_gb_used, vms_count, volumes_count, snapshots_count, networks_count, egresses_count, ingresses_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   vcpus_used      = quota_usage.vcpus_used      + EXCLUDED.vcpus_used,
		   ram_mb_used     = quota_usage.ram_mb_used     + EXCLUDED.ram_mb_used,
		   volume_gb_used  = quota_usage.volume_gb_used  + EXCLUDED.volume_gb_used,
		   vms_count       = quota_usage.vms_count       + EXCLUDED.vms_count,
		   volumes_count   = quota_usage.volumes_count   + EXCLUDED.volumes_count,
		   snapshots_count = quota_usage.snapshots_count + EXCLUDED.snapshots_count,
		   networks_count  = quota_usage.networks_count  + EXCLUDED.networks_count,
		   egresses_count  = quota_usage.egresses_count  + EXCLUDED.egresses_count,
		   ingresses_count = quota_usage.ingresses_count + EXCLUDED.ingresses_count,
		   updated_at      = now()`,
		r.tenantID, r.vcpus, r.ramMB, r.volumeGB, r.vms, r.volumes, r.snapshots, r.networks, r.egresses, r.ingresses,
	)
	if err != nil {
		return fmt.Errorf("quota: commit: upsert usage: %w", err)
	}

	return tx.Commit(ctx)
}

// Release deletes a reservation without updating usage.
func (s *Store) Release(ctx context.Context, resourceType ResourceType, resourceID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM quota_reserves WHERE resource_type = $1 AND resource_id = $2`,
		string(resourceType), resourceID,
	)
	if err != nil {
		return fmt.Errorf("quota: release: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrReservationNotFound
	}
	return nil
}

// Decommit decrements committed usage when a resource is destroyed.
// Floors each counter at 0 to prevent negative values from stale state.
func (s *Store) Decommit(ctx context.Context, tenantID uuid.UUID, delta ResourceDelta) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO quota_usage
		 (tenant_id, vcpus_used, ram_mb_used, volume_gb_used, vms_count, volumes_count, snapshots_count, networks_count, egresses_count, ingresses_count)
		 VALUES ($1, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		 ON CONFLICT (tenant_id) DO UPDATE SET
		   vcpus_used      = GREATEST(0, quota_usage.vcpus_used      - $2),
		   ram_mb_used     = GREATEST(0, quota_usage.ram_mb_used     - $3),
		   volume_gb_used  = GREATEST(0, quota_usage.volume_gb_used  - $4),
		   vms_count       = GREATEST(0, quota_usage.vms_count       - $5),
		   volumes_count   = GREATEST(0, quota_usage.volumes_count   - $6),
		   snapshots_count = GREATEST(0, quota_usage.snapshots_count - $7),
		   networks_count  = GREATEST(0, quota_usage.networks_count  - $8),
		   egresses_count  = GREATEST(0, quota_usage.egresses_count  - $9),
		   ingresses_count = GREATEST(0, quota_usage.ingresses_count - $10),
		   updated_at      = now()`,
		tenantID,
		delta.Vcpus, delta.RAMMB, delta.VolumeGB,
		delta.VMs, delta.Volumes, delta.Snapshots, delta.Networks,
		delta.Egresses, delta.Ingresses,
	)
	if err != nil {
		return fmt.Errorf("quota: decommit: %w", err)
	}
	return nil
}

// GetUsage returns committed usage for a tenant.
func (s *Store) GetUsage(ctx context.Context, tenantID uuid.UUID) (*Usage, error) {
	u := &Usage{TenantID: tenantID}
	err := s.pool.QueryRow(ctx,
		`SELECT vcpus_used, ram_mb_used, volume_gb_used, vms_count, volumes_count, snapshots_count, networks_count, egresses_count, ingresses_count
		 FROM quota_usage WHERE tenant_id = $1`,
		tenantID,
	).Scan(&u.VcpusUsed, &u.RAMMBUsed, &u.VolumeGBUsed, &u.VMsCount, &u.VolumesCount, &u.SnapshotsCount, &u.NetworksCount, &u.EgressesCount, &u.IngressesCount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return u, nil // no usage row means zero usage
		}
		return nil, fmt.Errorf("quota: get usage: %w", err)
	}
	return u, nil
}

// SetTenantLimits updates quota limits on the tenants table.
func (s *Store) SetTenantLimits(ctx context.Context, tenantID uuid.UUID, limits Limits) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE tenants SET
		   quota_vcpus     = $2,
		   quota_ram_mb    = $3,
		   quota_volume_gb = $4,
		   quota_vms       = $5,
		   quota_volumes   = $6,
		   quota_snapshots = $7,
		   quota_networks  = $8,
		   quota_egresses  = $9,
		   quota_ingresses = $10,
		   updated_at      = now()
		 WHERE id = $1`,
		tenantID, limits.Vcpus, limits.RAMMB, limits.VolumeGB,
		limits.VMs, limits.Volumes, limits.Snapshots, limits.Networks,
		limits.Egresses, limits.Ingresses,
	)
	if err != nil {
		return fmt.Errorf("quota: set tenant limits: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("quota: set tenant limits: tenant not found")
	}
	return nil
}

// GetTenantLimits reads quota limits from the tenants table.
func (s *Store) GetTenantLimits(ctx context.Context, tenantID uuid.UUID) (*Limits, error) {
	l := &Limits{}
	err := s.pool.QueryRow(ctx,
		`SELECT quota_vcpus, quota_ram_mb, quota_volume_gb, quota_vms, quota_volumes, quota_snapshots, quota_networks, quota_egresses, quota_ingresses
		 FROM tenants WHERE id = $1`,
		tenantID,
	).Scan(&l.Vcpus, &l.RAMMB, &l.VolumeGB, &l.VMs, &l.Volumes, &l.Snapshots, &l.Networks, &l.Egresses, &l.Ingresses)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("quota: get tenant limits: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("quota: get tenant limits: %w", err)
	}
	return l, nil
}

// SetOrgLimits updates quota limits on the organizations table.
// Org limits only cover vcpus, ram_mb, volume_gb.
func (s *Store) SetOrgLimits(ctx context.Context, orgID uuid.UUID, limits Limits) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE organizations SET
		   quota_vcpus     = $2,
		   quota_ram_mb    = $3,
		   quota_volume_gb = $4,
		   updated_at      = now()
		 WHERE id = $1`,
		orgID, limits.Vcpus, limits.RAMMB, limits.VolumeGB,
	)
	if err != nil {
		return fmt.Errorf("quota: set org limits: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("quota: set org limits: organization not found")
	}
	return nil
}

// GetOrgLimits reads quota limits from the organizations table.
func (s *Store) GetOrgLimits(ctx context.Context, orgID uuid.UUID) (*Limits, error) {
	l := &Limits{}
	err := s.pool.QueryRow(ctx,
		`SELECT quota_vcpus, quota_ram_mb, quota_volume_gb
		 FROM organizations WHERE id = $1`,
		orgID,
	).Scan(&l.Vcpus, &l.RAMMB, &l.VolumeGB)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("quota: get org limits: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("quota: get org limits: %w", err)
	}
	return l, nil
}

// --- internal helpers ---

// checkLimits performs the limit check using a regular pool connection.
func (s *Store) checkLimits(ctx context.Context, tenantID uuid.UUID, delta ResourceDelta) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("quota: check: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return s.checkLimitsTx(ctx, tx, tenantID, delta)
}

// checkLimitsTx checks tenant and org limits within an existing transaction.
// delta is added on top of usage+reserves (used in Check; zero in Reserve where the
// reserve row is already inserted before this is called).
func (s *Store) checkLimitsTx(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, delta ResourceDelta) error {
	// Query 1: aggregate tenant usage+reserves+delta alongside tenant limits and orgID.
	var tl Limits
	var orgID uuid.UUID
	var e struct {
		vcpus, ramMB, volumeGB, vms, volumes, snapshots, networks, egresses, ingresses int
	}
	err := tx.QueryRow(ctx,
		`SELECT
		   t.organization_id,
		   t.quota_vcpus, t.quota_ram_mb, t.quota_volume_gb,
		   t.quota_vms, t.quota_volumes, t.quota_snapshots, t.quota_networks,
		   t.quota_egresses, t.quota_ingresses,
		   COALESCE(u.vcpus_used,      0) + COALESCE(SUM(r.vcpus),     0) + $2,
		   COALESCE(u.ram_mb_used,     0) + COALESCE(SUM(r.ram_mb),    0) + $3,
		   COALESCE(u.volume_gb_used,  0) + COALESCE(SUM(r.volume_gb), 0) + $4,
		   COALESCE(u.vms_count,       0) + COALESCE(SUM(r.vms),       0) + $5,
		   COALESCE(u.volumes_count,   0) + COALESCE(SUM(r.volumes),   0) + $6,
		   COALESCE(u.snapshots_count, 0) + COALESCE(SUM(r.snapshots), 0) + $7,
		   COALESCE(u.networks_count,  0) + COALESCE(SUM(r.networks),  0) + $8,
		   COALESCE(u.egresses_count,  0) + COALESCE(SUM(r.egresses),  0) + $9,
		   COALESCE(u.ingresses_count, 0) + COALESCE(SUM(r.ingresses), 0) + $10
		 FROM tenants t
		 LEFT JOIN quota_usage u ON u.tenant_id = t.id
		 LEFT JOIN quota_reserves r ON r.tenant_id = t.id
		 WHERE t.id = $1
		 GROUP BY t.id, t.organization_id,
		          t.quota_vcpus, t.quota_ram_mb, t.quota_volume_gb,
		          t.quota_vms, t.quota_volumes, t.quota_snapshots, t.quota_networks,
		          t.quota_egresses, t.quota_ingresses,
		          u.vcpus_used, u.ram_mb_used, u.volume_gb_used,
		          u.vms_count, u.volumes_count, u.snapshots_count, u.networks_count,
		          u.egresses_count, u.ingresses_count`,
		tenantID,
		delta.Vcpus, delta.RAMMB, delta.VolumeGB,
		delta.VMs, delta.Volumes, delta.Snapshots, delta.Networks,
		delta.Egresses, delta.Ingresses,
	).Scan(
		&orgID,
		&tl.Vcpus, &tl.RAMMB, &tl.VolumeGB, &tl.VMs, &tl.Volumes, &tl.Snapshots, &tl.Networks,
		&tl.Egresses, &tl.Ingresses,
		&e.vcpus, &e.ramMB, &e.volumeGB, &e.vms, &e.volumes, &e.snapshots, &e.networks,
		&e.egresses, &e.ingresses,
	)
	if err != nil {
		return fmt.Errorf("quota: check: tenant: %w", err)
	}

	if err := checkAgainst(e.vcpus, e.ramMB, e.volumeGB, e.vms, e.volumes, e.snapshots, e.networks, e.egresses, e.ingresses, tl); err != nil {
		return err
	}

	// Query 2: aggregate org-wide usage+reserves alongside org limits.
	// When called from Reserve, delta is all-zero because the delta is already captured
	// in the newly inserted reserve row (which the SUM picks up). When called from Check,
	// no reserve row exists yet, so the non-zero delta is added here explicitly.
	var ol Limits
	var orgEff struct{ vcpus, ramMB, volumeGB int }
	err = tx.QueryRow(ctx,
		`SELECT
		   o.quota_vcpus, o.quota_ram_mb, o.quota_volume_gb,
		   COALESCE(SUM(u.vcpus_used),     0) + COALESCE(SUM(r.vcpus),     0) + $2,
		   COALESCE(SUM(u.ram_mb_used),    0) + COALESCE(SUM(r.ram_mb),    0) + $3,
		   COALESCE(SUM(u.volume_gb_used), 0) + COALESCE(SUM(r.volume_gb), 0) + $4
		 FROM organizations o
		 LEFT JOIN tenants t ON t.organization_id = o.id
		 LEFT JOIN quota_usage u ON u.tenant_id = t.id
		 LEFT JOIN quota_reserves r ON r.tenant_id = t.id
		 WHERE o.id = $1
		 GROUP BY o.quota_vcpus, o.quota_ram_mb, o.quota_volume_gb`,
		orgID, delta.Vcpus, delta.RAMMB, delta.VolumeGB,
	).Scan(&ol.Vcpus, &ol.RAMMB, &ol.VolumeGB, &orgEff.vcpus, &orgEff.ramMB, &orgEff.volumeGB)
	if err != nil {
		return fmt.Errorf("quota: check: org: %w", err)
	}

	if ol.Vcpus > 0 && orgEff.vcpus > ol.Vcpus {
		return fmt.Errorf("%w: org vcpus limit %d, effective %d", ErrQuotaExceeded, ol.Vcpus, orgEff.vcpus)
	}
	if ol.RAMMB > 0 && orgEff.ramMB > ol.RAMMB {
		return fmt.Errorf("%w: org ram_mb limit %d, effective %d", ErrQuotaExceeded, ol.RAMMB, orgEff.ramMB)
	}
	if ol.VolumeGB > 0 && orgEff.volumeGB > ol.VolumeGB {
		return fmt.Errorf("%w: org volume_gb limit %d, effective %d", ErrQuotaExceeded, ol.VolumeGB, orgEff.volumeGB)
	}

	return nil
}

// checkAgainst compares effective usage against tenant limits.
// A limit of 0 means unlimited.
func checkAgainst(vcpus, ramMB, volumeGB, vms, volumes, snapshots, networks, egresses, ingresses int, l Limits) error {
	if l.Vcpus > 0 && vcpus > l.Vcpus {
		return fmt.Errorf("%w: vcpus limit %d, effective %d", ErrQuotaExceeded, l.Vcpus, vcpus)
	}
	if l.RAMMB > 0 && ramMB > l.RAMMB {
		return fmt.Errorf("%w: ram_mb limit %d, effective %d", ErrQuotaExceeded, l.RAMMB, ramMB)
	}
	if l.VolumeGB > 0 && volumeGB > l.VolumeGB {
		return fmt.Errorf("%w: volume_gb limit %d, effective %d", ErrQuotaExceeded, l.VolumeGB, volumeGB)
	}
	if l.VMs > 0 && vms > l.VMs {
		return fmt.Errorf("%w: vms limit %d, effective %d", ErrQuotaExceeded, l.VMs, vms)
	}
	if l.Volumes > 0 && volumes > l.Volumes {
		return fmt.Errorf("%w: volumes limit %d, effective %d", ErrQuotaExceeded, l.Volumes, volumes)
	}
	if l.Snapshots > 0 && snapshots > l.Snapshots {
		return fmt.Errorf("%w: snapshots limit %d, effective %d", ErrQuotaExceeded, l.Snapshots, snapshots)
	}
	if l.Networks > 0 && networks > l.Networks {
		return fmt.Errorf("%w: networks limit %d, effective %d", ErrQuotaExceeded, l.Networks, networks)
	}
	if l.Egresses > 0 && egresses > l.Egresses {
		return fmt.Errorf("%w: egresses limit %d, effective %d", ErrQuotaExceeded, l.Egresses, egresses)
	}
	if l.Ingresses > 0 && ingresses > l.Ingresses {
		return fmt.Errorf("%w: ingresses limit %d, effective %d", ErrQuotaExceeded, l.Ingresses, ingresses)
	}
	return nil
}
