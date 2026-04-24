// Package compute manages VM lifecycle: create, list, get, delete.
package compute

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/tjst-t/cirrus/internal/jobqueue"
)

// CreateVMResponse holds the result of a successful VM creation request.
type CreateVMResponse struct {
	VM    *VM
	JobID uuid.UUID
}

// DeleteVMResponse holds the result of a successful VM deletion request.
type DeleteVMResponse struct {
	JobID uuid.UUID
}

// Service defines compute (VM) management operations.
type Service interface {
	// RegisterHandlers registers the compute job handlers with the given dispatcher.
	// Must be called before the dispatcher is started.
	RegisterHandlers(d *jobqueue.Dispatcher)

	// CreateVM enqueues an async VM creation job and returns the VM record in "pending" status
	// along with the job ID for polling.
	CreateVM(ctx context.Context, spec CreateVMSpec) (*CreateVMResponse, error)

	// GetVM returns a VM by ID within the given tenant.
	GetVM(ctx context.Context, tenantID, vmID uuid.UUID) (*VM, error)

	// ListVMsPage returns a page of VMs ordered by (created_at, id).
	ListVMsPage(ctx context.Context, tenantID uuid.UUID, afterCreatedAt time.Time, afterID uuid.UUID, limit int) ([]VM, error)

	// DeleteVM enqueues an async VM deletion job.
	// Allowed only when status is stopped or error.
	DeleteVM(ctx context.Context, tenantID, vmID uuid.UUID) (*DeleteVMResponse, error)

	// StartVM starts a stopped VM.
	StartVM(ctx context.Context, tenantID, vmID uuid.UUID) error

	// StopVM gracefully shuts down a running VM.
	StopVM(ctx context.Context, tenantID, vmID uuid.UUID) error

	// ForceStopVM forcefully powers off a running VM.
	ForceStopVM(ctx context.Context, tenantID, vmID uuid.UUID) error

	// RebootVM reboots a running VM.
	RebootVM(ctx context.Context, tenantID, vmID uuid.UUID) error

	// RepairVM transitions a VM from error to stopped (admin only).
	RepairVM(ctx context.Context, vmID uuid.UUID) error

	// MigrateVM live-migrates a running VM to a new host.
	// If targetHostID is nil, the scheduler selects the destination.
	MigrateVM(ctx context.Context, tenantID, vmID uuid.UUID, targetHostID *uuid.UUID) error

	// FailoverVM cold-restarts an error-state VM on a new host after the original
	// host has been fenced. The VM must be in 'error' status.
	// Flow: Reschedule → ExportVolume → UpdatePortHost → Worker.CreateVM → update host_id → status=running
	FailoverVM(ctx context.Context, vmID uuid.UUID) error
}
