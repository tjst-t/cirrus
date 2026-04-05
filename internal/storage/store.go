package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store implements database operations for the storage module.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new storage Store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func wrapNotFound(msg string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", msg, ErrVolumeNotFound)
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// --- Backend ---

func (s *Store) InsertBackend(ctx context.Context, b Backend) (*Backend, error) {
	caps, _ := json.Marshal(b.Capabilities)
	dcfg, _ := json.Marshal(b.DriverConfig)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO storage_backends
		   (storage_domain_id, name, driver, endpoint, total_capacity_gb, total_iops, capabilities, driver_config, state)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active')
		 RETURNING id, storage_domain_id, name, driver, endpoint, total_capacity_gb, total_iops,
		           capabilities, driver_config, state, created_at, updated_at`,
		b.StorageDomainID, b.Name, b.Driver, b.Endpoint,
		b.TotalCapacityGB, b.TotalIOPS, caps, dcfg,
	).Scan(&b.ID, &b.StorageDomainID, &b.Name, &b.Driver, &b.Endpoint,
		&b.TotalCapacityGB, &b.TotalIOPS, &b.Capabilities, &b.DriverConfig,
		&b.State, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("storage: insert backend: %w", err)
	}
	return &b, nil
}

func (s *Store) GetBackend(ctx context.Context, id uuid.UUID) (*Backend, error) {
	var b Backend
	err := s.pool.QueryRow(ctx,
		`SELECT id, storage_domain_id, name, driver, endpoint, total_capacity_gb, total_iops,
		        capabilities, driver_config, state, created_at, updated_at
		 FROM storage_backends WHERE id = $1`, id,
	).Scan(&b.ID, &b.StorageDomainID, &b.Name, &b.Driver, &b.Endpoint,
		&b.TotalCapacityGB, &b.TotalIOPS, &b.Capabilities, &b.DriverConfig,
		&b.State, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("storage: get backend: %w", ErrBackendNotFound)
		}
		return nil, fmt.Errorf("storage: get backend: %w", err)
	}
	return &b, nil
}

func (s *Store) ListBackends(ctx context.Context) ([]Backend, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, storage_domain_id, name, driver, endpoint, total_capacity_gb, total_iops,
		        capabilities, driver_config, state, created_at, updated_at
		 FROM storage_backends ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("storage: list backends: %w", err)
	}
	defer rows.Close()
	var backends []Backend
	for rows.Next() {
		var b Backend
		if err := rows.Scan(&b.ID, &b.StorageDomainID, &b.Name, &b.Driver, &b.Endpoint,
			&b.TotalCapacityGB, &b.TotalIOPS, &b.Capabilities, &b.DriverConfig,
			&b.State, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: list backends scan: %w", err)
		}
		backends = append(backends, b)
	}
	return backends, rows.Err()
}

func (s *Store) SetBackendState(ctx context.Context, id uuid.UUID, state BackendState) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE storage_backends SET state = $1, updated_at = now() WHERE id = $2`,
		state, id)
	if err != nil {
		return fmt.Errorf("storage: set backend state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("storage: set backend state: %w", ErrBackendNotFound)
	}
	return nil
}

// ListActiveBackendsForDomain returns active backends in a storage domain.
func (s *Store) ListActiveBackendsForDomain(ctx context.Context, domainID uuid.UUID) ([]Backend, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, storage_domain_id, name, driver, endpoint, total_capacity_gb, total_iops,
		        capabilities, driver_config, state, created_at, updated_at
		 FROM storage_backends WHERE storage_domain_id = $1 AND state = 'active' ORDER BY name`,
		domainID)
	if err != nil {
		return nil, fmt.Errorf("storage: list active backends for domain: %w", err)
	}
	defer rows.Close()
	var backends []Backend
	for rows.Next() {
		var b Backend
		if err := rows.Scan(&b.ID, &b.StorageDomainID, &b.Name, &b.Driver, &b.Endpoint,
			&b.TotalCapacityGB, &b.TotalIOPS, &b.Capabilities, &b.DriverConfig,
			&b.State, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: list active backends for domain scan: %w", err)
		}
		backends = append(backends, b)
	}
	return backends, rows.Err()
}

// ListActiveBackendsForAZ returns active backends in storage_domains mapped to the given AZ.
func (s *Store) ListActiveBackendsForAZ(ctx context.Context, azID uuid.UUID) ([]Backend, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT sb.id, sb.storage_domain_id, sb.name, sb.driver, sb.endpoint,
		        sb.total_capacity_gb, sb.total_iops,
		        sb.capabilities, sb.driver_config, sb.state, sb.created_at, sb.updated_at
		 FROM storage_backends sb
		 JOIN az_storage_domains azsd ON azsd.storage_domain_id = sb.storage_domain_id
		 WHERE azsd.az_id = $1 AND sb.state = 'active'
		 ORDER BY sb.name`,
		azID)
	if err != nil {
		return nil, fmt.Errorf("storage: list active backends for az: %w", err)
	}
	defer rows.Close()
	var backends []Backend
	for rows.Next() {
		var b Backend
		if err := rows.Scan(&b.ID, &b.StorageDomainID, &b.Name, &b.Driver, &b.Endpoint,
			&b.TotalCapacityGB, &b.TotalIOPS, &b.Capabilities, &b.DriverConfig,
			&b.State, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: list active backends for az scan: %w", err)
		}
		backends = append(backends, b)
	}
	return backends, rows.Err()
}

// ListBackendsReachableFromHost returns active backends reachable from a host
// (via host_storage_domains → storage_backends).
func (s *Store) ListBackendsReachableFromHost(ctx context.Context, hostID uuid.UUID) ([]Backend, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT sb.id, sb.storage_domain_id, sb.name, sb.driver, sb.endpoint,
		        sb.total_capacity_gb, sb.total_iops,
		        sb.capabilities, sb.driver_config, sb.state, sb.created_at, sb.updated_at
		 FROM storage_backends sb
		 JOIN host_storage_domains hsd ON hsd.storage_domain_id = sb.storage_domain_id
		 WHERE hsd.host_id = $1 AND sb.state = 'active'
		 ORDER BY sb.name`,
		hostID)
	if err != nil {
		return nil, fmt.Errorf("storage: list backends reachable from host: %w", err)
	}
	defer rows.Close()
	var backends []Backend
	for rows.Next() {
		var b Backend
		if err := rows.Scan(&b.ID, &b.StorageDomainID, &b.Name, &b.Driver, &b.Endpoint,
			&b.TotalCapacityGB, &b.TotalIOPS, &b.Capabilities, &b.DriverConfig,
			&b.State, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: list backends reachable from host scan: %w", err)
		}
		backends = append(backends, b)
	}
	return backends, rows.Err()
}

// --- VolumeType ---

func (s *Store) InsertVolumeType(ctx context.Context, vt VolumeType) (*VolumeType, error) {
	caps, _ := json.Marshal(vt.RequiredCapabilities)
	qos, _ := json.Marshal(vt.QoSPolicy)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO volume_types (name, description, required_capabilities, qos_policy, is_public)
		 VALUES ($1,$2,$3,$4,$5)
		 RETURNING id, name, description, required_capabilities, qos_policy, is_public, created_at, updated_at`,
		vt.Name, vt.Description, caps, qos, vt.IsPublic,
	).Scan(&vt.ID, &vt.Name, &vt.Description, &vt.RequiredCapabilities, &vt.QoSPolicy,
		&vt.IsPublic, &vt.CreatedAt, &vt.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("storage: insert volume type: %w", err)
	}
	return &vt, nil
}

func (s *Store) GetVolumeType(ctx context.Context, id uuid.UUID) (*VolumeType, error) {
	var vt VolumeType
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, description, required_capabilities, qos_policy, is_public, created_at, updated_at
		 FROM volume_types WHERE id = $1`, id,
	).Scan(&vt.ID, &vt.Name, &vt.Description, &vt.RequiredCapabilities, &vt.QoSPolicy,
		&vt.IsPublic, &vt.CreatedAt, &vt.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("storage: get volume type: %w", ErrVolumeTypeNotFound)
		}
		return nil, fmt.Errorf("storage: get volume type: %w", err)
	}
	return &vt, nil
}

func (s *Store) ListVolumeTypes(ctx context.Context) ([]VolumeType, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, description, required_capabilities, qos_policy, is_public, created_at, updated_at
		 FROM volume_types ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("storage: list volume types: %w", err)
	}
	defer rows.Close()
	var vts []VolumeType
	for rows.Next() {
		var vt VolumeType
		if err := rows.Scan(&vt.ID, &vt.Name, &vt.Description, &vt.RequiredCapabilities, &vt.QoSPolicy,
			&vt.IsPublic, &vt.CreatedAt, &vt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("storage: list volume types scan: %w", err)
		}
		vts = append(vts, vt)
	}
	return vts, rows.Err()
}

// --- Volume ---

const volumeColumns = `id, tenant_id, name, volume_type_id, backend_id, size_gb, state,
                        exported_host_id, export_info, az_id, created_at, updated_at`

func scanVolume(row pgx.Row) (*Volume, error) {
	var v Volume
	if err := row.Scan(&v.ID, &v.TenantID, &v.Name, &v.VolumeTypeID, &v.BackendID,
		&v.SizeGB, &v.State, &v.ExportedHostID, &v.ExportInfo, &v.AZID,
		&v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) InsertVolume(ctx context.Context, v Volume) (*Volume, error) {
	if v.ID == (uuid.UUID{}) {
		v.ID = uuid.New()
	}
	result, err := scanVolume(s.pool.QueryRow(ctx,
		`INSERT INTO volumes (id, tenant_id, name, volume_type_id, backend_id, size_gb, state, az_id)
		 VALUES ($1,$2,$3,$4,$5,$6,'creating',$7)
		 RETURNING `+volumeColumns,
		v.ID, v.TenantID, v.Name, v.VolumeTypeID, v.BackendID, v.SizeGB, v.AZID))
	if err != nil {
		return nil, fmt.Errorf("storage: insert volume: %w", err)
	}
	return result, nil
}

func (s *Store) GetVolume(ctx context.Context, id uuid.UUID) (*Volume, error) {
	v, err := scanVolume(s.pool.QueryRow(ctx,
		`SELECT `+volumeColumns+` FROM volumes WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("storage: get volume: %w", ErrVolumeNotFound)
		}
		return nil, fmt.Errorf("storage: get volume: %w", err)
	}
	return v, nil
}

func (s *Store) ListVolumesByTenant(ctx context.Context, tenantID uuid.UUID) ([]Volume, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+volumeColumns+` FROM volumes WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("storage: list volumes: %w", err)
	}
	defer rows.Close()
	var vs []Volume
	for rows.Next() {
		v, err := scanVolume(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: list volumes scan: %w", err)
		}
		vs = append(vs, *v)
	}
	return vs, rows.Err()
}

func (s *Store) ListVolumesByBackend(ctx context.Context, backendID uuid.UUID) ([]Volume, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+volumeColumns+` FROM volumes WHERE backend_id = $1 ORDER BY name`, backendID)
	if err != nil {
		return nil, fmt.Errorf("storage: list volumes by backend: %w", err)
	}
	defer rows.Close()
	var vs []Volume
	for rows.Next() {
		v, err := scanVolume(rows)
		if err != nil {
			return nil, fmt.Errorf("storage: list volumes by backend scan: %w", err)
		}
		vs = append(vs, *v)
	}
	return vs, rows.Err()
}

func (s *Store) SetVolumeState(ctx context.Context, id uuid.UUID, state VolumeState) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE volumes SET state = $1, updated_at = now() WHERE id = $2`, state, id)
	if err != nil {
		return fmt.Errorf("storage: set volume state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("storage: set volume state: %w", ErrVolumeNotFound)
	}
	return nil
}

func (s *Store) SetVolumeExport(ctx context.Context, id, hostID uuid.UUID, info json.RawMessage) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE volumes SET state = 'in_use', exported_host_id = $1, export_info = $2, updated_at = now()
		 WHERE id = $3`,
		hostID, info, id)
	if err != nil {
		return fmt.Errorf("storage: set volume export: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("storage: set volume export: %w", ErrVolumeNotFound)
	}
	return nil
}

func (s *Store) ClearVolumeExport(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE volumes SET state = 'available', exported_host_id = NULL, export_info = NULL, updated_at = now()
		 WHERE id = $1`,
		id)
	if err != nil {
		return fmt.Errorf("storage: clear volume export: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("storage: clear volume export: %w", ErrVolumeNotFound)
	}
	return nil
}

func (s *Store) ResizeVolume(ctx context.Context, id uuid.UUID, newSizeGB int64) (*Volume, error) {
	v, err := scanVolume(s.pool.QueryRow(ctx,
		`UPDATE volumes SET size_gb = $1, updated_at = now()
		 WHERE id = $2
		 RETURNING `+volumeColumns,
		newSizeGB, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("storage: resize volume: %w", ErrVolumeNotFound)
		}
		return nil, fmt.Errorf("storage: resize volume: %w", err)
	}
	return v, nil
}

func (s *Store) DeleteVolume(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM volumes WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("storage: delete volume: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("storage: delete volume: %w", ErrVolumeNotFound)
	}
	return nil
}

// GetHostStorageProperties returns hosts.storage_properties for a given host.
func (s *Store) GetHostStorageProperties(ctx context.Context, hostID uuid.UUID) (map[string]string, error) {
	var raw json.RawMessage
	err := s.pool.QueryRow(ctx,
		`SELECT storage_properties FROM hosts WHERE id = $1`, hostID,
	).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("storage: get host properties: host not found")
		}
		return nil, fmt.Errorf("storage: get host properties: %w", err)
	}
	var props map[string]string
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, fmt.Errorf("storage: get host properties: unmarshal: %w", err)
	}
	return props, nil
}

// GetHostDataIPs returns the management/data IPs for a host (address field).
func (s *Store) GetHostDataIPs(ctx context.Context, hostID uuid.UUID) ([]string, error) {
	var address string
	err := s.pool.QueryRow(ctx,
		`SELECT address FROM hosts WHERE id = $1`, hostID,
	).Scan(&address)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("storage: get host ips: host not found")
		}
		return nil, fmt.Errorf("storage: get host ips: %w", err)
	}
	return []string{address}, nil
}
