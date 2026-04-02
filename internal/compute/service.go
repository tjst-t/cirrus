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
	DeleteVM(ctx context.Context, tenantID, vmID uuid.UUID) error
}
