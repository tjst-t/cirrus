package compute

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tjst-t/cirrus/internal/host"
)

const vmCols = `id, tenant_id, name, flavor_id, az_id, network_id, host_id, status, error_message, created_at, updated_at`

// ErrNotFound is returned when a VM is not found.
var ErrNotFound = errors.New("compute: vm not found")

func (o *Orchestrator) insertVM(ctx context.Context, vm *VM) error {
	_, err := o.pool.Exec(ctx,
		`INSERT INTO vms (id, tenant_id, name, flavor_id, az_id, network_id, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		vm.ID, vm.TenantID, vm.Name, vm.FlavorID, vm.AZID, vm.NetworkID, vm.Status, vm.CreatedAt, vm.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("compute: insert vm: %w", err)
	}
	return nil
}

func (o *Orchestrator) getVM(ctx context.Context, tenantID, vmID uuid.UUID) (*VM, error) {
	var vm VM
	err := o.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, flavor_id, az_id, network_id, host_id, status, error_message, created_at, updated_at
		 FROM vms WHERE id = $1 AND tenant_id = $2`,
		vmID, tenantID,
	).Scan(&vm.ID, &vm.TenantID, &vm.Name, &vm.FlavorID, &vm.AZID, &vm.NetworkID, &vm.HostID,
		&vm.Status, &vm.ErrorMessage, &vm.CreatedAt, &vm.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("compute: get vm: %w", err)
	}
	return &vm, nil
}

func (o *Orchestrator) listVMs(ctx context.Context, tenantID uuid.UUID) ([]VM, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT id, tenant_id, name, flavor_id, az_id, network_id, host_id, status, error_message, created_at, updated_at
		 FROM vms WHERE tenant_id = $1 ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("compute: list vms: %w", err)
	}
	defer rows.Close()

	var vms []VM
	for rows.Next() {
		var vm VM
		if err := rows.Scan(&vm.ID, &vm.TenantID, &vm.Name, &vm.FlavorID, &vm.AZID, &vm.NetworkID, &vm.HostID,
			&vm.Status, &vm.ErrorMessage, &vm.CreatedAt, &vm.UpdatedAt); err != nil {
			return nil, fmt.Errorf("compute: list vms scan: %w", err)
		}
		vms = append(vms, vm)
	}
	return vms, rows.Err()
}

// ListVMsPage implements Service.ListVMsPage.
func (o *Orchestrator) ListVMsPage(ctx context.Context, tenantID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]VM, error) {
	scanVMs := func(query string, args ...any) ([]VM, error) {
		rows, err := o.pool.Query(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("compute: list vms page: %w", err)
		}
		defer rows.Close()
		var vms []VM
		for rows.Next() {
			var vm VM
			if err := rows.Scan(&vm.ID, &vm.TenantID, &vm.Name, &vm.FlavorID, &vm.AZID, &vm.NetworkID, &vm.HostID,
				&vm.Status, &vm.ErrorMessage, &vm.CreatedAt, &vm.UpdatedAt); err != nil {
				return nil, fmt.Errorf("compute: list vms page scan: %w", err)
			}
			vms = append(vms, vm)
		}
		return vms, rows.Err()
	}

	if afterCreatedAt.IsZero() {
		return scanVMs(`SELECT `+vmCols+` FROM vms WHERE tenant_id = $1 ORDER BY created_at, id LIMIT $2`, tenantID, limit)
	}
	return scanVMs(`SELECT `+vmCols+` FROM vms WHERE tenant_id = $1 AND (created_at > $2 OR (created_at = $2 AND id > $3)) ORDER BY created_at, id LIMIT $4`,
		tenantID, afterCreatedAt, afterID, limit)
}

func (o *Orchestrator) setVMStatus(ctx context.Context, vmID uuid.UUID, status VMStatus, errMsg string) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE vms SET status = $1, error_message = $2, updated_at = $3 WHERE id = $4`,
		status, errMsg, time.Now(), vmID,
	)
	return err
}

// HealVM transitions a VM that has drifted from expected state to error.
// It uses an optimistic check to avoid overwriting transitional statuses
// (pending, building, deleting) or a status that was already corrected.
// Satisfies reconcile.VMHealer.
func (o *Orchestrator) HealVM(ctx context.Context, vmID uuid.UUID, reason string) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE vms SET status = $1, error_message = $2, updated_at = $3
		 WHERE id = $4 AND status NOT IN ('pending', 'building', 'deleting', 'migrating', 'error')`,
		VMStatusError, reason, time.Now(), vmID,
	)
	if err != nil {
		return fmt.Errorf("compute: heal vm %s: %w", vmID, err)
	}
	return nil
}

// RecoverVM transitions a VM from error state back to the given status.
// Called by the DriftHandler when a previously error-marked VM is re-observed
// as running or shutoff in a heartbeat (e.g. after a worker restart).
// Satisfies reconcile.VMRecoverer.
func (o *Orchestrator) RecoverVM(ctx context.Context, vmID uuid.UUID, newStatus string) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE vms SET status = $1, error_message = '', updated_at = $2
		 WHERE id = $3 AND status = 'error'`,
		newStatus, time.Now(), vmID,
	)
	if err != nil {
		return fmt.Errorf("compute: recover vm %s: %w", vmID, err)
	}
	return nil
}

func (o *Orchestrator) setVMHost(ctx context.Context, vmID, hostID uuid.UUID) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE vms SET host_id = $1, updated_at = $2 WHERE id = $3`,
		hostID, time.Now(), vmID,
	)
	return err
}

// resolveAZForHost returns the AZ associated with the given host via
// host_storage_domains → az_storage_domains → availability_zones.
// Returns uuid.Nil when no AZ is linked.
func (o *Orchestrator) resolveAZForHost(ctx context.Context, hostID uuid.UUID) (uuid.UUID, error) {
	var azID uuid.UUID
	err := o.pool.QueryRow(ctx,
		`SELECT az.id
		 FROM availability_zones az
		 JOIN az_storage_domains azsd ON azsd.az_id = az.id
		 JOIN host_storage_domains hsd ON hsd.storage_domain_id = azsd.storage_domain_id
		 WHERE hsd.host_id = $1
		 LIMIT 1`,
		hostID,
	).Scan(&azID)
	if err != nil {
		return uuid.Nil, nil // no AZ found — not an error
	}
	return azID, nil
}

func (o *Orchestrator) setVMAZ(ctx context.Context, vmID, azID uuid.UUID) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE vms SET az_id = $1, updated_at = $2 WHERE id = $3`,
		azID, time.Now(), vmID,
	)
	return err
}

func (o *Orchestrator) insertVMVolume(ctx context.Context, vmID, volumeID uuid.UUID, device string) error {
	_, err := o.pool.Exec(ctx,
		`INSERT INTO vm_volumes (id, vm_id, volume_id, device) VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		uuid.New(), vmID, volumeID, device,
	)
	return err
}

func (o *Orchestrator) deleteVMRecord(ctx context.Context, vmID uuid.UUID) error {
	_, err := o.pool.Exec(ctx, `DELETE FROM vms WHERE id = $1`, vmID)
	return err
}

// vmVolumeEntry holds the volume ID and device name for a VM attachment.
type vmVolumeEntry struct {
	volumeID uuid.UUID
	device   string
}

// listVMVolumeIDs returns the volume UUIDs attached to a VM.
func (o *Orchestrator) listVMVolumeIDs(ctx context.Context, vmID uuid.UUID) ([]uuid.UUID, error) {
	entries, err := o.listVMVolumeEntries(ctx, vmID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, len(entries))
	for i, e := range entries {
		ids[i] = e.volumeID
	}
	return ids, nil
}

// listVMVolumeEntries returns the volume IDs and device names for all volumes attached to a VM.
func (o *Orchestrator) listVMVolumeEntries(ctx context.Context, vmID uuid.UUID) ([]vmVolumeEntry, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT volume_id, device FROM vm_volumes WHERE vm_id = $1 ORDER BY device`, vmID)
	if err != nil {
		return nil, fmt.Errorf("compute: list vm volumes: %w", err)
	}
	defer rows.Close()

	var entries []vmVolumeEntry
	for rows.Next() {
		var e vmVolumeEntry
		if err := rows.Scan(&e.volumeID, &e.device); err != nil {
			return nil, fmt.Errorf("compute: list vm volumes scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// insertFallbackRoute creates a migration_fallback_routes record and returns its ID.
// Called by MigrateVM before StartMigration to instruct the source host to forward
// traffic for the migrating VM to the destination host via Geneve tunnel.
func (o *Orchestrator) insertFallbackRoute(ctx context.Context, portID, srcHostID, destHostID uuid.UUID) (uuid.UUID, error) {
	id := uuid.New()
	_, err := o.pool.Exec(ctx,
		`INSERT INTO migration_fallback_routes (id, port_id, src_host_id, dest_host_id)
		 VALUES ($1, $2, $3, $4)`,
		id, portID, srcHostID, destHostID,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("compute: insert fallback route: %w", err)
	}
	return id, nil
}

// deleteFallbackRoute removes a migration_fallback_routes record by ID.
// Called by MigrateVM after migration completes (or on error) to remove the
// fallback forwarding flow from the source host.
func (o *Orchestrator) deleteFallbackRoute(ctx context.Context, id uuid.UUID) error {
	_, err := o.pool.Exec(ctx,
		`DELETE FROM migration_fallback_routes WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("compute: delete fallback route: %w", err)
	}
	return nil
}

// getVMByID looks up a VM by ID without tenant scoping (admin/internal use).
func (o *Orchestrator) getVMByID(ctx context.Context, vmID uuid.UUID) (*VM, error) {
	var vm VM
	err := o.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, flavor_id, az_id, network_id, host_id, status, error_message, created_at, updated_at
		 FROM vms WHERE id = $1`, vmID,
	).Scan(&vm.ID, &vm.TenantID, &vm.Name, &vm.FlavorID, &vm.AZID, &vm.NetworkID, &vm.HostID,
		&vm.Status, &vm.ErrorMessage, &vm.CreatedAt, &vm.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("compute: get vm by id: %w", err)
	}
	return &vm, nil
}

// getHostByID looks up a host by its UUID.
func (o *Orchestrator) getHostByID(ctx context.Context, hostID uuid.UUID) (*host.Host, error) {
	var h host.Host
	err := o.pool.QueryRow(ctx,
		`SELECT id, name, address, worker_grpc_addr, fabric_ip, operational_state
		 FROM hosts WHERE id = $1`, hostID,
	).Scan(&h.ID, &h.Name, &h.Address, &h.WorkerGRPCAddr, &h.FabricIP, &h.OperationalState)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("compute: host %s not found", hostID)
		}
		return nil, fmt.Errorf("compute: get host: %w", err)
	}
	return &h, nil
}
