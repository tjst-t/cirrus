package hypervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// LibvirtDriver connects to cirrus-sim's libvirt simulator via HTTP API.
// In production this would use go-libvirt to connect to a real libvirt daemon.
type LibvirtDriver struct {
	uri        string // tcp://host:port
	httpClient *http.Client
	baseURL    string
}

// NewLibvirtDriver creates a new libvirt driver for the given URI.
func NewLibvirtDriver(uri string) *LibvirtDriver {
	addr := strings.TrimPrefix(uri, "tcp://")
	return &LibvirtDriver{
		uri: uri,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		baseURL: "http://" + addr,
	}
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
