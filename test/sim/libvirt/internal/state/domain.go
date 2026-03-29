package state

import (
	"fmt"
	"time"
)

// DomainState represents the libvirt domain state.
type DomainState int32

const (
	// DomainStateNoState represents VIR_DOMAIN_NOSTATE.
	DomainStateNoState DomainState = 0
	// DomainStateRunning represents VIR_DOMAIN_RUNNING.
	DomainStateRunning DomainState = 1
	// DomainStateBlocked represents VIR_DOMAIN_BLOCKED.
	DomainStateBlocked DomainState = 2
	// DomainStatePaused represents VIR_DOMAIN_PAUSED.
	DomainStatePaused DomainState = 3
	// DomainStateShutdown represents VIR_DOMAIN_SHUTDOWN.
	DomainStateShutdown DomainState = 4
	// DomainStateShutoff represents VIR_DOMAIN_SHUTOFF.
	DomainStateShutoff DomainState = 5
	// DomainStateCrashed represents VIR_DOMAIN_CRASHED.
	DomainStateCrashed DomainState = 6
	// DomainStatePMSuspended represents VIR_DOMAIN_PMSUSPENDED.
	DomainStatePMSuspended DomainState = 7
)

// MigrationState tracks the migration status of a domain.
type MigrationState int32

const (
	// MigrationStateNone indicates no migration is in progress.
	MigrationStateNone MigrationState = 0
	// MigrationStatePrepared indicates resources are reserved on destination.
	MigrationStatePrepared MigrationState = 1
	// MigrationStatePerforming indicates migration is in progress from source.
	MigrationStatePerforming MigrationState = 2
)

// Domain represents a simulated libvirt domain (VM).
type Domain struct {
	Name      string      `json:"name"`
	UUID      [16]byte    `json:"uuid"`
	ID        int32       `json:"id"`
	State     DomainState `json:"state"`
	VCPUs     int         `json:"vcpus"`
	MemoryKiB int64       `json:"memory_kib"`
	XML       string      `json:"xml"`
	CreatedAt time.Time   `json:"created_at"`
	StartedAt time.Time   `json:"started_at,omitempty"`

	// InterfaceIDs holds OVS interfaceid values from the domain XML.
	// These correspond to OVN Logical_Switch_Port names.
	InterfaceIDs []string `json:"interface_ids,omitempty"`

	// Migration tracking fields.
	MigrationState MigrationState `json:"migration_state,omitempty"`
	MigrationCookie string        `json:"migration_cookie,omitempty"`
}

// UUIDString returns the UUID as a formatted string.
func (d *Domain) UUIDString() string {
	u := d.UUID
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uint32(u[0])<<24|uint32(u[1])<<16|uint32(u[2])<<8|uint32(u[3]),
		uint16(u[4])<<8|uint16(u[5]),
		uint16(u[6])<<8|uint16(u[7]),
		uint16(u[8])<<8|uint16(u[9]),
		uint64(u[10])<<40|uint64(u[11])<<32|uint64(u[12])<<24|uint64(u[13])<<16|uint64(u[14])<<8|uint64(u[15]),
	)
}

// Start transitions the domain from shutoff to running.
func (d *Domain) Start() error {
	if d.State != DomainStateShutoff {
		return fmt.Errorf("cannot start domain in state %d: %w", d.State, ErrOperationInvalid)
	}
	d.State = DomainStateRunning
	d.ID = -1 // Will be assigned by store
	d.StartedAt = time.Now()
	return nil
}

// Destroy transitions the domain from running/paused to shutoff.
func (d *Domain) Destroy() error {
	if d.State != DomainStateRunning && d.State != DomainStatePaused {
		return fmt.Errorf("cannot destroy domain in state %d: %w", d.State, ErrOperationInvalid)
	}
	d.State = DomainStateShutoff
	d.ID = -1
	return nil
}

// Shutdown performs a graceful shutdown (same effect as destroy in simulator).
func (d *Domain) Shutdown() error {
	if d.State != DomainStateRunning {
		return fmt.Errorf("cannot shutdown domain in state %d: %w", d.State, ErrOperationInvalid)
	}
	d.State = DomainStateShutoff
	d.ID = -1
	return nil
}

// Suspend transitions the domain from running to paused.
func (d *Domain) Suspend() error {
	if d.State != DomainStateRunning {
		return fmt.Errorf("cannot suspend domain in state %d: %w", d.State, ErrOperationInvalid)
	}
	d.State = DomainStatePaused
	return nil
}

// Resume transitions the domain from paused to running.
func (d *Domain) Resume() error {
	if d.State != DomainStatePaused {
		return fmt.Errorf("cannot resume domain in state %d: %w", d.State, ErrOperationInvalid)
	}
	d.State = DomainStateRunning
	return nil
}

// Reboot transitions through shutdown and back to running.
func (d *Domain) Reboot() error {
	if d.State != DomainStateRunning {
		return fmt.Errorf("cannot reboot domain in state %d: %w", d.State, ErrOperationInvalid)
	}
	// In the simulator, reboot is instantaneous: stays running
	return nil
}
