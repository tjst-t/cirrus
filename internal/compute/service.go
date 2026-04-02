// Package compute manages VM lifecycle: create, list, get, delete.
package compute

import (
	"context"

	"github.com/google/uuid"
)

// Service defines compute (VM) management operations.
type Service interface {
	// CreateVM enqueues an async VM creation job and returns the VM record in "pending" status.
	CreateVM(ctx context.Context, spec CreateVMSpec) (*VM, error)

	// GetVM returns a VM by ID within the given tenant.
	GetVM(ctx context.Context, tenantID, vmID uuid.UUID) (*VM, error)

	// ListVMs returns all VMs belonging to the given tenant.
	ListVMs(ctx context.Context, tenantID uuid.UUID) ([]VM, error)

	// DeleteVM enqueues an async VM deletion job.
	// Allowed only when status is stopped or error.
	DeleteVM(ctx context.Context, tenantID, vmID uuid.UUID) error

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
}
