package hypervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// simDomainInfo matches the DomainInfo JSON from the sim management API.
type simDomainInfo struct {
	Name         string   `json:"name"`
	UUID         string   `json:"uuid"`
	State        int32    `json:"state"`
	VCPUs        int      `json:"vcpus"`
	MemoryKiB    int64    `json:"memory_kib"`
	HostID       string   `json:"host_id"`
	InterfaceIDs []string `json:"interface_ids,omitempty"`
}

// LibvirtDriver connects to cirrus-sim's libvirt simulator via HTTP API.
// In production this would use go-libvirt to connect to a real libvirt daemon.
type LibvirtDriver struct {
	uri        string // tcp://host:port  (libvirt protocol port)
	hostID     string // worker host ID (used in management API paths)
	httpClient *http.Client
	baseURL    string // HTTP management API base URL (e.g. http://localhost:8100)
}

// NewLibvirtDriver creates a new libvirt driver.
// uri is the libvirt protocol URI (tcp://host:port).
// simMgmtAddr is the HTTP management API base URL for libvirtd-sim (e.g. http://localhost:8100).
// If simMgmtAddr is empty, it falls back to the URI address (legacy behaviour).
func NewLibvirtDriver(uri, simMgmtAddr string) *LibvirtDriver {
	baseURL := simMgmtAddr
	if baseURL == "" {
		addr := strings.TrimPrefix(uri, "tcp://")
		baseURL = "http://" + addr
	}
	return &LibvirtDriver{
		uri: uri,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// SetHostID sets the worker host ID used in management API paths.
func (d *LibvirtDriver) SetHostID(hostID string) {
	d.hostID = hostID
}

func (d *LibvirtDriver) Connect(ctx context.Context) error {
	addr := strings.TrimPrefix(d.uri, "tcp://")
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("libvirt: connect %s: %w", addr, err)
	}
	conn.Close()
	return nil
}

func (d *LibvirtDriver) Close() error {
	return nil
}

func (d *LibvirtDriver) GetHostInfo(ctx context.Context) (*HostInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", d.baseURL+"/sim/host-info", nil)
	if err != nil {
		return nil, fmt.Errorf("libvirt: get host info: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("libvirt: get host info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("libvirt: get host info: HTTP %d: %s", resp.StatusCode, body)
	}

	var info HostInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("libvirt: decode host info: %w", err)
	}
	return &info, nil
}

func (d *LibvirtDriver) ListVMs(ctx context.Context) ([]VMInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", d.baseURL+"/sim/domains", nil)
	if err != nil {
		return nil, fmt.Errorf("libvirt: list vms: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("libvirt: list vms: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("libvirt: list vms: HTTP %d: %s", resp.StatusCode, body)
	}

	var vms []VMInfo
	if err := json.NewDecoder(resp.Body).Decode(&vms); err != nil {
		return nil, fmt.Errorf("libvirt: decode vms: %w", err)
	}
	return vms, nil
}

// DefineVM creates and immediately starts a VM on the sim host.
// If spec.CloudInit is set and spec.CloudInitISOPath is empty, an ISO is built
// under /tmp/cirrus-cloudinit and the path is embedded in the domain XML.
func (d *LibvirtDriver) DefineVM(ctx context.Context, spec VMSpec) (*VMInfo, error) {
	if d.hostID == "" {
		return nil, fmt.Errorf("libvirt: DefineVM: hostID not set")
	}

	// Build cloud-init ISO if requested and not already supplied.
	if spec.CloudInit != nil && spec.CloudInitISOPath == "" {
		dir := filepath.Join(os.TempDir(), "cirrus-cloudinit")
		isoPath, err := BuildCloudInitISO(*spec.CloudInit, dir)
		if err != nil {
			return nil, fmt.Errorf("libvirt: DefineVM: cloud-init: %w", err)
		}
		spec.CloudInitISOPath = isoPath
	}

	domXML, err := BuildDomainXML(spec)
	if err != nil {
		return nil, fmt.Errorf("libvirt: DefineVM: build xml: %w", err)
	}

	body, err := json.Marshal(map[string]string{"xml": domXML})
	if err != nil {
		return nil, fmt.Errorf("libvirt: DefineVM: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/sim/hosts/%s/domains", d.baseURL, d.hostID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("libvirt: DefineVM: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("libvirt: DefineVM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("libvirt: DefineVM: HTTP %d: %s", resp.StatusCode, b)
	}

	var info simDomainInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("libvirt: DefineVM: decode: %w", err)
	}

	return &VMInfo{
		ID:    info.UUID,
		Name:  info.Name,
		State: domainStateToVMState(info.State),
		Vcpus: int32(info.VCPUs),
		RAMMb: info.MemoryKiB / 1024,
	}, nil
}

// StartVM starts a shutoff VM identified by name.
func (d *LibvirtDriver) StartVM(ctx context.Context, name string) error {
	if d.hostID == "" {
		return fmt.Errorf("libvirt: StartVM: hostID not set")
	}
	domUUID, err := d.lookupDomainUUID(ctx, name)
	if err != nil {
		return fmt.Errorf("libvirt: StartVM: %w", err)
	}

	url := fmt.Sprintf("%s/sim/hosts/%s/domains/%s/start", d.baseURL, d.hostID, domUUID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("libvirt: StartVM: %w", err)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("libvirt: StartVM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("libvirt: StartVM: HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}

// StopVM requests a graceful shutdown of a running VM.
func (d *LibvirtDriver) StopVM(ctx context.Context, name string) error {
	if d.hostID == "" {
		return fmt.Errorf("libvirt: StopVM: hostID not set")
	}
	domUUID, err := d.lookupDomainUUID(ctx, name)
	if err != nil {
		return fmt.Errorf("libvirt: StopVM: %w", err)
	}

	url := fmt.Sprintf("%s/sim/hosts/%s/domains/%s/stop", d.baseURL, d.hostID, domUUID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("libvirt: StopVM: %w", err)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("libvirt: StopVM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("libvirt: StopVM: HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}

// RebootVM gracefully stops then starts a VM, simulating a reboot.
func (d *LibvirtDriver) RebootVM(ctx context.Context, name string) error {
	if err := d.StopVM(ctx, name); err != nil {
		return fmt.Errorf("libvirt: RebootVM: stop: %w", err)
	}
	if err := d.StartVM(ctx, name); err != nil {
		return fmt.Errorf("libvirt: RebootVM: start: %w", err)
	}
	return nil
}

// DestroyVM forcefully powers off a VM.
func (d *LibvirtDriver) DestroyVM(ctx context.Context, name string) error {
	if d.hostID == "" {
		return fmt.Errorf("libvirt: DestroyVM: hostID not set")
	}
	domUUID, err := d.lookupDomainUUID(ctx, name)
	if err != nil {
		return fmt.Errorf("libvirt: DestroyVM: %w", err)
	}

	url := fmt.Sprintf("%s/sim/hosts/%s/domains/%s/destroy", d.baseURL, d.hostID, domUUID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("libvirt: DestroyVM: %w", err)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("libvirt: DestroyVM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("libvirt: DestroyVM: HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}

// UndefineVM removes a VM definition. The VM must already be shutoff.
func (d *LibvirtDriver) UndefineVM(ctx context.Context, name string) error {
	if d.hostID == "" {
		return fmt.Errorf("libvirt: UndefineVM: hostID not set")
	}
	domUUID, err := d.lookupDomainUUID(ctx, name)
	if err != nil {
		return fmt.Errorf("libvirt: UndefineVM: %w", err)
	}

	url := fmt.Sprintf("%s/sim/hosts/%s/domains/%s", d.baseURL, d.hostID, domUUID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("libvirt: UndefineVM: %w", err)
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("libvirt: UndefineVM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("libvirt: UndefineVM: HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}

// lookupDomainUUID fetches the domain list for this host and returns the UUID
// matching the given name. Returns an error if not found.
func (d *LibvirtDriver) lookupDomainUUID(ctx context.Context, name string) (string, error) {
	url := fmt.Sprintf("%s/sim/hosts/%s/domains", d.baseURL, d.hostID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("list domains: HTTP %d: %s", resp.StatusCode, b)
	}
	var doms []simDomainInfo
	if err := json.NewDecoder(resp.Body).Decode(&doms); err != nil {
		return "", fmt.Errorf("decode domains: %w", err)
	}
	for _, dom := range doms {
		if dom.Name == name {
			return dom.UUID, nil
		}
	}
	return "", fmt.Errorf("domain %q not found", name)
}

// domainStateToVMState converts a libvirt integer state to VMState.
func domainStateToVMState(s int32) VMState {
	switch s {
	case 1:
		return StateRunning
	case 3:
		return StatePaused
	case 4:
		return StateShutdown
	case 5:
		return StateShutoff
	case 6:
		return StateCrashed
	default:
		return StateShutoff
	}
}
