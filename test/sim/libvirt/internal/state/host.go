// Package state provides in-memory state management for libvirt-sim hosts and domains.
package state

import (
	"fmt"
	"sync"
)

// HostState represents the operational state of a simulated host.
type HostState string

const (
	// HostStateOnline indicates the host is operational.
	HostStateOnline HostState = "online"
	// HostStateOffline indicates the host is not operational.
	HostStateOffline HostState = "offline"
	// HostStateMaintenance indicates the host is in maintenance mode.
	HostStateMaintenance HostState = "maintenance"
)

// GPU represents a GPU device on a host.
type GPU struct {
	Address  string `json:"address"`
	Model    string `json:"model"`
	VendorID string `json:"vendor_id"`
	DeviceID string `json:"device_id"`
}

// NUMANode represents a NUMA topology node.
type NUMANode struct {
	ID       int   `json:"id"`
	CPUs     []int `json:"cpus"`
	MemoryMB int64 `json:"memory_mb"`
}

// Host represents a simulated libvirt host.
type Host struct {
	mu sync.RWMutex

	HostID             string     `json:"host_id"`
	LibvirtPort        int        `json:"libvirt_port"`
	CPUModel           string     `json:"cpu_model"`
	CPUSockets         int        `json:"cpu_sockets"`
	CoresPerSocket     int        `json:"cores_per_socket"`
	ThreadsPerCore     int        `json:"threads_per_core"`
	MemoryMB           int64      `json:"memory_mb"`
	CPUOvercommitRatio float64    `json:"cpu_overcommit_ratio"`
	MemOvercommitRatio float64    `json:"memory_overcommit_ratio"`
	NUMATopology       []NUMANode `json:"numa_topology,omitempty"`
	GPUs               []GPU      `json:"gpus,omitempty"`
	State              HostState  `json:"state"`

	// Domains maps domain UUID (string) to Domain.
	Domains map[string]*Domain `json:"-"`
}

// TotalVCPUs returns the total number of logical CPUs on the host.
func (h *Host) TotalVCPUs() int {
	return h.CPUSockets * h.CoresPerSocket * h.ThreadsPerCore
}

// AvailableVCPUs returns the maximum vCPUs considering overcommit ratio.
func (h *Host) AvailableVCPUs() int {
	return int(float64(h.TotalVCPUs()) * h.CPUOvercommitRatio)
}

// AvailableMemoryMB returns the maximum memory considering overcommit ratio.
func (h *Host) AvailableMemoryMB() int64 {
	return int64(float64(h.MemoryMB) * h.MemOvercommitRatio)
}

// UsedVCPUs returns the total vCPUs used by running/paused domains.
func (h *Host) UsedVCPUs() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, d := range h.Domains {
		if d.State == DomainStateRunning || d.State == DomainStatePaused {
			total += d.VCPUs
		}
	}
	return total
}

// UsedMemoryMB returns the total memory used by running/paused domains.
func (h *Host) UsedMemoryMB() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var total int64
	for _, d := range h.Domains {
		if d.State == DomainStateRunning || d.State == DomainStatePaused {
			total += d.MemoryKiB / 1024
		}
	}
	return total
}

// CanAllocate checks if the host has enough resources to start a domain.
func (h *Host) CanAllocate(vcpus int, memoryKiB int64) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	usedVCPUs := 0
	var usedMemMB int64
	for _, d := range h.Domains {
		if d.State == DomainStateRunning || d.State == DomainStatePaused {
			usedVCPUs += d.VCPUs
			usedMemMB += d.MemoryKiB / 1024
		}
	}

	if usedVCPUs+vcpus > h.AvailableVCPUs() {
		return fmt.Errorf("insufficient vCPUs: need %d, available %d (used %d, total %d)",
			vcpus, h.AvailableVCPUs()-usedVCPUs, usedVCPUs, h.AvailableVCPUs())
	}

	memMB := memoryKiB / 1024
	if usedMemMB+memMB > h.AvailableMemoryMB() {
		return fmt.Errorf("insufficient memory: need %d MB, available %d MB (used %d, total %d)",
			memMB, h.AvailableMemoryMB()-usedMemMB, usedMemMB, h.AvailableMemoryMB())
	}

	return nil
}

// HostInfo represents host information returned by the management API.
type HostInfo struct {
	HostID             string     `json:"host_id"`
	LibvirtPort        int        `json:"libvirt_port"`
	CPUModel           string     `json:"cpu_model"`
	CPUSockets         int        `json:"cpu_sockets"`
	CoresPerSocket     int        `json:"cores_per_socket"`
	ThreadsPerCore     int        `json:"threads_per_core"`
	MemoryMB           int64      `json:"memory_mb"`
	CPUOvercommitRatio float64    `json:"cpu_overcommit_ratio"`
	MemOvercommitRatio float64    `json:"memory_overcommit_ratio"`
	NUMATopology       []NUMANode `json:"numa_topology,omitempty"`
	GPUs               []GPU      `json:"gpus,omitempty"`
	State              HostState  `json:"state"`
	UsedVCPUs          int        `json:"used_vcpus"`
	UsedMemoryMB       int64      `json:"used_memory_mb"`
	DomainCount        int        `json:"domain_count"`
}

// Info returns a HostInfo snapshot.
func (h *Host) Info() HostInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	usedVCPUs := 0
	var usedMemMB int64
	for _, d := range h.Domains {
		if d.State == DomainStateRunning || d.State == DomainStatePaused {
			usedVCPUs += d.VCPUs
			usedMemMB += d.MemoryKiB / 1024
		}
	}

	return HostInfo{
		HostID:             h.HostID,
		LibvirtPort:        h.LibvirtPort,
		CPUModel:           h.CPUModel,
		CPUSockets:         h.CPUSockets,
		CoresPerSocket:     h.CoresPerSocket,
		ThreadsPerCore:     h.ThreadsPerCore,
		MemoryMB:           h.MemoryMB,
		CPUOvercommitRatio: h.CPUOvercommitRatio,
		MemOvercommitRatio: h.MemOvercommitRatio,
		NUMATopology:       h.NUMATopology,
		GPUs:               h.GPUs,
		State:              h.State,
		UsedVCPUs:          usedVCPUs,
		UsedMemoryMB:       usedMemMB,
		DomainCount:        len(h.Domains),
	}
}
