package state

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type Project struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	QuotaVCPUs int       `json:"quota_vcpus"`
	QuotaRamMB int       `json:"quota_ram_mb"`
	QuotaVMs   int       `json:"quota_vms"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type APIKey struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	KeyHash   string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type APIKeyWithRaw struct {
	APIKey
	RawKey string `json:"key"`
}

type Worker struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Address       string    `json:"address"`
	TotalVCPUs    int       `json:"total_vcpus"`
	TotalRamMB    int       `json:"total_ram_mb"`
	TotalDiskGB   int       `json:"total_disk_gb"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Image struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	ProjectID *string `json:"project_id,omitempty"`
	Format    string  `json:"format"`
	SizeBytes int64   `json:"size_bytes"`
	Path      string  `json:"-"`
	Status    string  `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Network struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	CIDR      string    `json:"cidr"`
	Gateway   string    `json:"gateway"`
	VNI       int       `json:"vni"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type VM struct {
	ID          string           `json:"id"`
	ProjectID   string           `json:"project_id"`
	Name        string           `json:"name"`
	WorkerID    *string          `json:"worker_id,omitempty"`
	ImageID     string           `json:"image_id"`
	VCPUs       int              `json:"vcpus"`
	RamMB       int              `json:"ram_mb"`
	DiskGB      int              `json:"disk_gb"`
	Status      string           `json:"status"`
	ErrorMsg    *string          `json:"error_msg,omitempty"`
	StorageData json.RawMessage  `json:"storage_data,omitempty"`
	ComputeData json.RawMessage  `json:"compute_data,omitempty"`
	Ports       []Port           `json:"ports,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

type Port struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id,omitempty"`
	NetworkID   string          `json:"network_id"`
	VMID        *string         `json:"vm_id,omitempty"`
	MACAddress  string          `json:"mac_address"`
	IPAddress   string          `json:"ip_address"`
	Status      string          `json:"status"`
	NetworkData json.RawMessage `json:"network_data,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// NextAvailableIP finds the next available IP in a CIDR, skipping the network address,
// gateway, and broadcast address.
func NextAvailableIP(cidr string, gateway string, usedIPs []string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}

	used := make(map[string]bool)
	used[gateway] = true
	for _, ip := range usedIPs {
		used[ip] = true
	}

	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)

	// Skip network address
	inc(ip)
	// Skip gateway (typically .1)
	if ip.String() == gateway {
		inc(ip)
	}

	for ipNet.Contains(ip) {
		if !isBroadcast(ip, ipNet) && !used[ip.String()] {
			return ip.String(), nil
		}
		inc(ip)
	}

	return "", fmt.Errorf("no available IPs in %s", cidr)
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isBroadcast(ip net.IP, ipNet *net.IPNet) bool {
	for i := range ip {
		if ip[i] != ipNet.IP[i]|^ipNet.Mask[i] {
			return false
		}
	}
	return true
}
