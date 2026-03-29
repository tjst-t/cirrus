// Package datagen generates large-scale environment data from YAML definitions.
package datagen

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// EnvironmentDef is the top-level environment definition.
type EnvironmentDef struct {
	Environment Environment `yaml:"environment"`
}

// Environment defines the simulated environment.
type Environment struct {
	Name             string                    `yaml:"name"`
	Sites            []SiteDef                 `yaml:"sites"`
	HostTemplates    map[string]HostTemplate   `yaml:"host_templates"`
	StorageBackends  []StorageBackendDef       `yaml:"storage_backends"`
	OVNClusters      []OVNClusterDef           `yaml:"ovn_clusters"`
	Preload          *PreloadDef               `yaml:"preload,omitempty"`
}

// SiteDef defines a site with its topology.
type SiteDef struct {
	Name         string `yaml:"name"`
	RackRows     int    `yaml:"rack_rows"`
	RacksPerRow  int    `yaml:"racks_per_row"`
	HostsPerRack int    `yaml:"hosts_per_rack"`
	HostTemplate string `yaml:"host_template"`
}

// HostTemplate defines a host hardware profile.
type HostTemplate struct {
	CPUModel       string    `yaml:"cpu_model"`
	CPUSockets     int       `yaml:"cpu_sockets"`
	CoresPerSocket int       `yaml:"cores_per_socket"`
	ThreadsPerCore int       `yaml:"threads_per_core"`
	MemoryGB       int       `yaml:"memory_gb"`
	NUMANodes      int       `yaml:"numa_nodes"`
	NICs           []NICDef  `yaml:"nics,omitempty"`
	LocalDisks     []DiskDef `yaml:"local_disks,omitempty"`
	GPUs           []GPUDef  `yaml:"gpus,omitempty"`
	Inherits       string    `yaml:"inherits,omitempty"`
}

// NICDef defines a network interface.
type NICDef struct {
	Name      string `yaml:"name"`
	SpeedGbps int    `yaml:"speed_gbps"`
	SRIOV     bool   `yaml:"sriov"`
}

// DiskDef defines a local disk.
type DiskDef struct {
	Type   string `yaml:"type"`
	SizeGB int    `yaml:"size_gb"`
}

// GPUDef defines a GPU.
type GPUDef struct {
	Model string `yaml:"model"`
	Count int    `yaml:"count"`
}

// StorageBackendDef defines a storage backend.
type StorageBackendDef struct {
	Name            string   `yaml:"name"`
	Type            string   `yaml:"type"`
	TotalCapacityTB int      `yaml:"total_capacity_tb"`
	TotalIOPS       int      `yaml:"total_iops"`
	Capabilities    []string `yaml:"capabilities"`
	AccessibleFrom  []string `yaml:"accessible_from"`
}

// OVNClusterDef defines an OVN cluster.
type OVNClusterDef struct {
	Name   string   `yaml:"name"`
	Covers []string `yaml:"covers"`
}

// PreloadDef defines preload settings for the environment.
type PreloadDef struct {
	VMsPerHost        Range `yaml:"vms_per_host"`
	VolumesPerVM      Range `yaml:"volumes_per_vm"`
	NetworksPerTenant int   `yaml:"networks_per_tenant"`
	Tenants           int   `yaml:"tenants"`
}

// Range defines a min-max range.
type Range struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

// GeneratedHost represents a generated host.
type GeneratedHost struct {
	HostID         string `json:"host_id"`
	SiteName       string `json:"site_name"`
	RackName       string `json:"rack_name"`
	RowName        string `json:"row_name"`
	CPUModel       string `json:"cpu_model"`
	CPUSockets     int    `json:"cpu_sockets"`
	CoresPerSocket int    `json:"cores_per_socket"`
	ThreadsPerCore int    `json:"threads_per_core"`
	MemoryMB       int    `json:"memory_mb"`
	LibvirtPort    int    `json:"libvirt_port"`
}

// GeneratedBackend represents a generated storage backend.
type GeneratedBackend struct {
	BackendID       string   `json:"backend_id"`
	TotalCapacityGB int64    `json:"total_capacity_gb"`
	TotalIOPS       int64    `json:"total_iops"`
	Capabilities    []string `json:"capabilities"`
}

// GenerateResult holds the result of environment generation.
type GenerateResult struct {
	Name     string             `json:"name"`
	Hosts    []GeneratedHost    `json:"hosts"`
	Backends []GeneratedBackend `json:"backends"`
}

// GenerateStatus tracks generation progress.
type GenerateStatus struct {
	State   string `json:"state"` // "idle", "running", "completed", "failed"
	Message string `json:"message,omitempty"`
}

// Generator generates environment data from YAML definitions.
type Generator struct {
	mu     sync.RWMutex
	result *GenerateResult
	status GenerateStatus
}

// New creates a new Generator.
func New() *Generator {
	return &Generator{
		status: GenerateStatus{State: "idle"},
	}
}

// GenerateOptions configures port allocation for generation.
type GenerateOptions struct {
	// LibvirtBasePort is the starting port for libvirt host listeners.
	// Defaults to 16510 if zero.
	LibvirtBasePort int
}

// Generate parses YAML and generates environment data.
// Deprecated: Use GenerateWithOptions for configurable port allocation.
func (g *Generator) Generate(ctx context.Context, yamlData []byte) (*GenerateResult, error) {
	return g.GenerateWithOptions(ctx, yamlData, GenerateOptions{})
}

// GenerateWithOptions parses YAML and generates environment data with configurable options.
func (g *Generator) GenerateWithOptions(_ context.Context, yamlData []byte, opts GenerateOptions) (*GenerateResult, error) {
	g.mu.Lock()
	g.status = GenerateStatus{State: "running"}
	g.mu.Unlock()

	var envDef EnvironmentDef
	if err := yaml.Unmarshal(yamlData, &envDef); err != nil {
		g.mu.Lock()
		g.status = GenerateStatus{State: "failed", Message: err.Error()}
		g.mu.Unlock()
		return nil, fmt.Errorf("parse environment YAML: %w", err)
	}

	env := envDef.Environment
	result := &GenerateResult{Name: env.Name}

	basePort := opts.LibvirtBasePort
	if basePort == 0 {
		basePort = 16510
	}
	hostIdx := 0

	for _, site := range env.Sites {
		tmpl, ok := env.HostTemplates[site.HostTemplate]
		if !ok {
			g.mu.Lock()
			g.status = GenerateStatus{State: "failed", Message: fmt.Sprintf("template %q not found", site.HostTemplate)}
			g.mu.Unlock()
			return nil, fmt.Errorf("generate: host template %q not found", site.HostTemplate)
		}

		for row := 1; row <= site.RackRows; row++ {
			rowName := fmt.Sprintf("row-%s-%d", site.Name, row)
			for rack := 1; rack <= site.RacksPerRow; rack++ {
				rackName := fmt.Sprintf("rack-%s-%d-%d", site.Name, row, rack)
				for h := 1; h <= site.HostsPerRack; h++ {
					hostIdx++
					host := GeneratedHost{
						HostID:         fmt.Sprintf("host-%03d", hostIdx),
						SiteName:       site.Name,
						RackName:       rackName,
						RowName:        rowName,
						CPUModel:       tmpl.CPUModel,
						CPUSockets:     tmpl.CPUSockets,
						CoresPerSocket: tmpl.CoresPerSocket,
						ThreadsPerCore: tmpl.ThreadsPerCore,
						MemoryMB:       tmpl.MemoryGB * 1024,
						LibvirtPort:    basePort + hostIdx - 1,
					}
					result.Hosts = append(result.Hosts, host)
				}
			}
		}
	}

	for _, sb := range env.StorageBackends {
		backend := GeneratedBackend{
			BackendID:       sb.Name,
			TotalCapacityGB: int64(sb.TotalCapacityTB) * 1024,
			TotalIOPS:       int64(sb.TotalIOPS),
			Capabilities:    sb.Capabilities,
		}
		result.Backends = append(result.Backends, backend)
	}

	g.mu.Lock()
	g.result = result
	g.status = GenerateStatus{State: "completed"}
	g.mu.Unlock()

	return result, nil
}

// GetStatus returns the current generation status.
func (g *Generator) GetStatus(_ context.Context) GenerateStatus {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.status
}

// GetResult returns the last generation result.
func (g *Generator) GetResult(_ context.Context) *GenerateResult {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.result
}

// Reset clears all generated data.
func (g *Generator) Reset(_ context.Context) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.result = nil
	g.status = GenerateStatus{State: "idle"}
}

// GetGeneratedAt returns when the data was generated (current time for simplicity).
func (g *Generator) GetGeneratedAt() time.Time {
	return time.Now().UTC()
}
