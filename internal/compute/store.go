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

func (o *Orchestrator) setVMStatus(ctx context.Context, vmID uuid.UUID, status VMStatus, errMsg string) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE vms SET status = $1, error_message = $2, updated_at = $3 WHERE id = $4`,
		status, errMsg, time.Now(), vmID,
	)
	return err
}

func (o *Orchestrator) setVMHost(ctx context.Context, vmID, hostID uuid.UUID) error {
	_, err := o.pool.Exec(ctx,
		`UPDATE vms SET host_id = $1, updated_at = $2 WHERE id = $3`,
		hostID, time.Now(), vmID,
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

// listVMVolumeIDs returns the volume UUIDs attached to a VM.
func (o *Orchestrator) listVMVolumeIDs(ctx context.Context, vmID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT volume_id FROM vm_volumes WHERE vm_id = $1 ORDER BY device`, vmID)
	if err != nil {
		return nil, fmt.Errorf("compute: list vm volumes: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("compute: list vm volumes scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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
