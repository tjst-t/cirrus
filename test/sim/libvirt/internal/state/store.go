package state

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Sentinel errors for domain operations.
var (
	// ErrNoDomain indicates the requested domain was not found.
	ErrNoDomain = errors.New("domain not found")
	// ErrOperationInvalid indicates the operation is not valid for the current state.
	ErrOperationInvalid = errors.New("operation invalid for current state")
	// ErrOperationDenied indicates the operation was denied (e.g., insufficient resources).
	ErrOperationDenied = errors.New("operation denied")
	// ErrHostNotFound indicates the requested host was not found.
	ErrHostNotFound = errors.New("host not found")
	// ErrHostExists indicates a host with the same ID already exists.
	ErrHostExists = errors.New("host already exists")
	// ErrPortInUse indicates the port is already used by another host.
	ErrPortInUse = errors.New("port already in use")
	// ErrMigrationInProgress indicates a migration is already in progress for this domain.
	ErrMigrationInProgress = errors.New("migration already in progress")
)

// MigrationConfig controls simulated migration timing.
type MigrationConfig struct {
	PrepareDurationMs      int64 `json:"prepare_duration_ms"`
	BaseTransferDurationMs int64 `json:"base_transfer_duration_ms"`
	PerGBMemoryMs          int64 `json:"per_gb_memory_ms"`
	FinishDurationMs       int64 `json:"finish_duration_ms"`
}

// DefaultMigrationConfig returns the default migration config.
func DefaultMigrationConfig() MigrationConfig {
	return MigrationConfig{
		PrepareDurationMs:      500,
		BaseTransferDurationMs: 2000,
		PerGBMemoryMs:          500,
		FinishDurationMs:       200,
	}
}

// Store is the top-level in-memory store for all hosts and their domains.
type Store struct {
	mu    sync.RWMutex
	hosts map[string]*Host // key: host_id
	ports map[int]string   // key: port, value: host_id

	nextDomainID atomic.Int32

	migrationConfigMu sync.RWMutex
	migrationConfig   MigrationConfig

	migrationSpeedMu sync.RWMutex
	migrationSpeed   uint64 // max bandwidth in MiB/s, per-domain default
}

// NewStore creates a new empty Store.
func NewStore() *Store {
	s := &Store{
		hosts:           make(map[string]*Host),
		ports:           make(map[int]string),
		migrationConfig: DefaultMigrationConfig(),
		migrationSpeed:  9223372036, // effectively unlimited
	}
	s.nextDomainID.Store(1)
	return s
}

// GetMigrationConfig returns the current migration config.
func (s *Store) GetMigrationConfig() MigrationConfig {
	s.migrationConfigMu.RLock()
	defer s.migrationConfigMu.RUnlock()
	return s.migrationConfig
}

// SetMigrationConfig updates the migration config.
func (s *Store) SetMigrationConfig(cfg MigrationConfig) {
	s.migrationConfigMu.Lock()
	defer s.migrationConfigMu.Unlock()
	s.migrationConfig = cfg
}

// GetMigrationSpeed returns the max migration bandwidth.
func (s *Store) GetMigrationSpeed() uint64 {
	s.migrationSpeedMu.RLock()
	defer s.migrationSpeedMu.RUnlock()
	return s.migrationSpeed
}

// SetMigrationSpeed sets the max migration bandwidth.
func (s *Store) SetMigrationSpeed(speed uint64) {
	s.migrationSpeedMu.Lock()
	defer s.migrationSpeedMu.Unlock()
	s.migrationSpeed = speed
}

// AddHost registers a new host. Returns error if host ID or port already in use.
func (s *Store) AddHost(h *Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.hosts[h.HostID]; exists {
		return fmt.Errorf("host %s: %w", h.HostID, ErrHostExists)
	}
	if existingHost, exists := s.ports[h.LibvirtPort]; exists {
		return fmt.Errorf("port %d already used by host %s: %w", h.LibvirtPort, existingHost, ErrPortInUse)
	}

	if h.Domains == nil {
		h.Domains = make(map[string]*Domain)
	}
	if h.State == "" {
		h.State = HostStateOnline
	}
	if h.CPUOvercommitRatio == 0 {
		h.CPUOvercommitRatio = 1.0
	}
	if h.MemOvercommitRatio == 0 {
		h.MemOvercommitRatio = 1.0
	}

	s.hosts[h.HostID] = h
	s.ports[h.LibvirtPort] = h.HostID
	return nil
}

// GetHost returns the host by ID.
func (s *Store) GetHost(hostID string) (*Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h, ok := s.hosts[hostID]
	if !ok {
		return nil, fmt.Errorf("host %s: %w", hostID, ErrHostNotFound)
	}
	return h, nil
}

// GetHostByPort returns the host listening on the given port.
func (s *Store) GetHostByPort(port int) (*Host, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hostID, ok := s.ports[port]
	if !ok {
		return nil, fmt.Errorf("no host on port %d: %w", port, ErrHostNotFound)
	}
	return s.hosts[hostID], nil
}

// ListHosts returns all registered hosts.
func (s *Store) ListHosts() []*Host {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hosts := make([]*Host, 0, len(s.hosts))
	for _, h := range s.hosts {
		hosts = append(hosts, h)
	}
	return hosts
}

// RemoveHost removes a host and all its domains.
func (s *Store) RemoveHost(hostID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	h, ok := s.hosts[hostID]
	if !ok {
		return fmt.Errorf("host %s: %w", hostID, ErrHostNotFound)
	}

	delete(s.ports, h.LibvirtPort)
	delete(s.hosts, hostID)
	return nil
}

// Reset clears all state.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hosts = make(map[string]*Host)
	s.ports = make(map[int]string)
	s.nextDomainID.Store(1)

	s.migrationConfigMu.Lock()
	s.migrationConfig = DefaultMigrationConfig()
	s.migrationConfigMu.Unlock()

	s.migrationSpeedMu.Lock()
	s.migrationSpeed = 9223372036
	s.migrationSpeedMu.Unlock()
}

// DefineDomain adds a domain to a host in shutoff state.
func (s *Store) DefineDomain(hostID string, d *Domain) error {
	h, err := s.GetHost(hostID)
	if err != nil {
		return fmt.Errorf("define domain: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d.State = DomainStateShutoff
	d.ID = -1
	d.CreatedAt = time.Now()
	h.Domains[d.UUIDString()] = d
	return nil
}

// GetDomain looks up a domain by UUID on a host.
func (s *Store) GetDomain(hostID string, uuid string) (*Domain, error) {
	h, err := s.GetHost(hostID)
	if err != nil {
		return nil, fmt.Errorf("get domain: %w", err)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return nil, fmt.Errorf("domain %s on host %s: %w", uuid, hostID, ErrNoDomain)
	}
	return d, nil
}

// GetDomainByName looks up a domain by name on a host.
func (s *Store) GetDomainByName(hostID string, name string) (*Domain, error) {
	h, err := s.GetHost(hostID)
	if err != nil {
		return nil, fmt.Errorf("get domain by name: %w", err)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, d := range h.Domains {
		if d.Name == name {
			return d, nil
		}
	}
	return nil, fmt.Errorf("domain name %s on host %s: %w", name, hostID, ErrNoDomain)
}

// ListDomains returns all domains on a host.
func (s *Store) ListDomains(hostID string) ([]*Domain, error) {
	h, err := s.GetHost(hostID)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	domains := make([]*Domain, 0, len(h.Domains))
	for _, d := range h.Domains {
		domains = append(domains, d)
	}
	return domains, nil
}

// StartDomain transitions a domain to running state, checking resources.
func (s *Store) StartDomain(hostID string, uuid string) error {
	h, err := s.GetHost(hostID)
	if err != nil {
		return fmt.Errorf("start domain: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	if d.State != DomainStateShutoff {
		return fmt.Errorf("cannot start domain %s in state %d: %w", uuid, d.State, ErrOperationInvalid)
	}

	// Check resources (calculate without lock since we already hold it)
	usedVCPUs := 0
	var usedMemMB int64
	for _, dom := range h.Domains {
		if dom.State == DomainStateRunning || dom.State == DomainStatePaused {
			usedVCPUs += dom.VCPUs
			usedMemMB += dom.MemoryKiB / 1024
		}
	}

	if usedVCPUs+d.VCPUs > h.AvailableVCPUs() {
		return fmt.Errorf("insufficient vCPUs to start domain %s: %w", uuid, ErrOperationDenied)
	}

	memMB := d.MemoryKiB / 1024
	if usedMemMB+memMB > h.AvailableMemoryMB() {
		return fmt.Errorf("insufficient memory to start domain %s: %w", uuid, ErrOperationDenied)
	}

	d.State = DomainStateRunning
	d.ID = s.nextDomainID.Add(1) - 1
	d.StartedAt = time.Now()
	return nil
}

// DestroyDomain force-stops a domain.
func (s *Store) DestroyDomain(hostID string, uuid string) error {
	h, err := s.GetHost(hostID)
	if err != nil {
		return fmt.Errorf("destroy domain: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	return d.Destroy()
}

// ShutdownDomain gracefully stops a domain.
func (s *Store) ShutdownDomain(hostID string, uuid string) error {
	h, err := s.GetHost(hostID)
	if err != nil {
		return fmt.Errorf("shutdown domain: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	return d.Shutdown()
}

// SuspendDomain pauses a running domain.
func (s *Store) SuspendDomain(hostID string, uuid string) error {
	h, err := s.GetHost(hostID)
	if err != nil {
		return fmt.Errorf("suspend domain: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	return d.Suspend()
}

// ResumeDomain resumes a paused domain.
func (s *Store) ResumeDomain(hostID string, uuid string) error {
	h, err := s.GetHost(hostID)
	if err != nil {
		return fmt.Errorf("resume domain: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	return d.Resume()
}

// UndefineDomain removes a domain from a host (must be shutoff).
func (s *Store) UndefineDomain(hostID string, uuid string) error {
	h, err := s.GetHost(hostID)
	if err != nil {
		return fmt.Errorf("undefine domain: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	if d.State != DomainStateShutoff {
		return fmt.Errorf("cannot undefine running domain %s: %w", uuid, ErrOperationInvalid)
	}

	delete(h.Domains, uuid)
	return nil
}

// MigratePrepare reserves resources on the destination host for incoming migration.
// It creates a placeholder domain in "paused" state with MigrationStatePrepared.
func (s *Store) MigratePrepare(destHostID string, dom *Domain) error {
	h, err := s.GetHost(destHostID)
	if err != nil {
		return fmt.Errorf("migrate prepare: %w", err)
	}

	if h.State != HostStateOnline {
		return fmt.Errorf("destination host %s not online: %w", destHostID, ErrOperationDenied)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Check if domain already exists on destination
	if _, exists := h.Domains[dom.UUIDString()]; exists {
		return fmt.Errorf("domain %s already exists on destination host %s: %w",
			dom.UUIDString(), destHostID, ErrOperationInvalid)
	}

	// Check resources
	usedVCPUs := 0
	var usedMemMB int64
	for _, d := range h.Domains {
		if d.State == DomainStateRunning || d.State == DomainStatePaused {
			usedVCPUs += d.VCPUs
			usedMemMB += d.MemoryKiB / 1024
		}
	}

	if usedVCPUs+dom.VCPUs > h.AvailableVCPUs() {
		return fmt.Errorf("insufficient vCPUs on destination host %s: %w", destHostID, ErrOperationDenied)
	}

	memMB := dom.MemoryKiB / 1024
	if usedMemMB+memMB > h.AvailableMemoryMB() {
		return fmt.Errorf("insufficient memory on destination host %s: %w", destHostID, ErrOperationDenied)
	}

	// Create placeholder domain
	placeholder := &Domain{
		Name:            dom.Name,
		UUID:            dom.UUID,
		ID:              -1,
		State:           DomainStatePaused,
		VCPUs:           dom.VCPUs,
		MemoryKiB:       dom.MemoryKiB,
		XML:             dom.XML,
		InterfaceIDs:    dom.InterfaceIDs,
		CreatedAt:       dom.CreatedAt,
		MigrationState:  MigrationStatePrepared,
		MigrationCookie: dom.UUIDString(),
	}
	h.Domains[dom.UUIDString()] = placeholder
	return nil
}

// MigratePerform marks a domain on the source host as migrating.
func (s *Store) MigratePerform(srcHostID string, uuid string) error {
	h, err := s.GetHost(srcHostID)
	if err != nil {
		return fmt.Errorf("migrate perform: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	if d.State != DomainStateRunning {
		return fmt.Errorf("cannot migrate domain %s in state %d: %w", uuid, d.State, ErrOperationInvalid)
	}

	if d.MigrationState != MigrationStateNone {
		return fmt.Errorf("domain %s: %w", uuid, ErrMigrationInProgress)
	}

	d.MigrationState = MigrationStatePerforming
	d.MigrationCookie = uuid
	return nil
}

// MigrateFinish activates the domain on the destination host.
func (s *Store) MigrateFinish(destHostID string, uuid string) (*Domain, error) {
	h, err := s.GetHost(destHostID)
	if err != nil {
		return nil, fmt.Errorf("migrate finish: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return nil, fmt.Errorf("domain %s on destination: %w", uuid, ErrNoDomain)
	}

	if d.MigrationState != MigrationStatePrepared {
		return nil, fmt.Errorf("domain %s not in prepared state: %w", uuid, ErrOperationInvalid)
	}

	d.State = DomainStateRunning
	d.ID = s.nextDomainID.Add(1) - 1
	d.StartedAt = time.Now()
	d.MigrationState = MigrationStateNone
	d.MigrationCookie = ""
	return d, nil
}

// MigrateConfirm removes the domain from the source host after successful migration.
func (s *Store) MigrateConfirm(srcHostID string, uuid string) error {
	h, err := s.GetHost(srcHostID)
	if err != nil {
		return fmt.Errorf("migrate confirm: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	d, ok := h.Domains[uuid]
	if !ok {
		return fmt.Errorf("domain %s: %w", uuid, ErrNoDomain)
	}

	if d.MigrationState != MigrationStatePerforming {
		return fmt.Errorf("domain %s not in migrating state: %w", uuid, ErrOperationInvalid)
	}

	delete(h.Domains, uuid)
	return nil
}

// Stats returns overall statistics.
type Stats struct {
	TotalHosts      int `json:"total_hosts"`
	OnlineHosts     int `json:"online_hosts"`
	TotalDomains    int `json:"total_domains"`
	RunningDomains  int `json:"running_domains"`
	TotalVCPUsUsed  int `json:"total_vcpus_used"`
	TotalMemoryUsed int64 `json:"total_memory_used_mb"`
}

// GetStats returns aggregate statistics.
func (s *Store) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats Stats
	stats.TotalHosts = len(s.hosts)
	for _, h := range s.hosts {
		if h.State == HostStateOnline {
			stats.OnlineHosts++
		}
		h.mu.RLock()
		for _, d := range h.Domains {
			stats.TotalDomains++
			if d.State == DomainStateRunning {
				stats.RunningDomains++
			}
			if d.State == DomainStateRunning || d.State == DomainStatePaused {
				stats.TotalVCPUsUsed += d.VCPUs
				stats.TotalMemoryUsed += d.MemoryKiB / 1024
			}
		}
		h.mu.RUnlock()
	}
	return stats
}
