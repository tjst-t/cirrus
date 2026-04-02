// Package sim implements the storage.Driver interface for cirrus-sim storage-sim.
// It calls the storage-sim HTTP API (REST) to manage volumes.
// HostInfo.Properties are accepted but not validated — the sim does not
// enforce iSCSI ACLs or Ceph keyrings. Protocol is always "sim".
package sim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tjst-t/cirrus/internal/storage"
)

// Driver is the cirrus-sim storage driver.
type Driver struct {
	endpoint  string // e.g. "http://localhost:9003"
	backendID string // X-Backend-Id header value
	client    *http.Client
}

// New creates a new sim Driver.
func New(endpoint, backendID string) *Driver {
	return &Driver{
		endpoint:  endpoint,
		backendID: backendID,
		client:    &http.Client{},
	}
}

func (d *Driver) Capabilities() storage.DriverCapabilities {
	return storage.DriverCapabilities{
		Snapshot: true,
		Clone:    true,
	}
}

func (d *Driver) CreateVolume(ctx context.Context, spec storage.DriverVolumeSpec) (*storage.DriverVolume, error) {
	body, _ := json.Marshal(map[string]any{
		"volume_id":        spec.VolumeID,
		"size_gb":          spec.SizeGB,
		"thin_provisioned": spec.ThinProvisioned,
	})
	resp, err := d.do(ctx, http.MethodPost, "/api/v1/volumes", body)
	if err != nil {
		return nil, fmt.Errorf("sim driver: create volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("sim driver: create volume: unexpected status %d", resp.StatusCode)
	}
	var v struct {
		VolumeID string `json:"volume_id"`
		SizeGB   int64  `json:"size_gb"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("sim driver: create volume: decode response: %w", err)
	}
	return &storage.DriverVolume{VolumeID: v.VolumeID, SizeGB: v.SizeGB}, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, volumeID string) error {
	resp, err := d.do(ctx, http.MethodDelete, "/api/v1/volumes/"+volumeID, nil)
	if err != nil {
		return fmt.Errorf("sim driver: delete volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("sim driver: delete volume: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (d *Driver) ResizeVolume(ctx context.Context, volumeID string, newSizeGB int64) error {
	body, _ := json.Marshal(map[string]any{"new_size_gb": newSizeGB})
	resp, err := d.do(ctx, http.MethodPut, "/api/v1/volumes/"+volumeID+"/extend", body)
	if err != nil {
		return fmt.Errorf("sim driver: resize volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sim driver: resize volume: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ExportVolume tells the sim to mark the volume as exported to the given host.
// HostInfo.Properties are passed through but the sim does not enforce them.
func (d *Driver) ExportVolume(ctx context.Context, volumeID string, host storage.HostInfo) (*storage.ExportInfo, error) {
	body, _ := json.Marshal(map[string]any{
		"host_id":  host.ID,
		"protocol": "sim",
	})
	resp, err := d.do(ctx, http.MethodPost, "/api/v1/volumes/"+volumeID+"/export", body)
	if err != nil {
		return nil, fmt.Errorf("sim driver: export volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sim driver: export volume: unexpected status %d", resp.StatusCode)
	}
	return &storage.ExportInfo{
		Protocol: "sim",
		Params: map[string]string{
			"volume_id": volumeID,
			"host_id":   host.ID,
			"endpoint":  d.endpoint,
			"backend_id": d.backendID,
		},
	}, nil
}

// UnexportVolume revokes the export on the sim backend.
func (d *Driver) UnexportVolume(ctx context.Context, volumeID string, _ storage.HostInfo) error {
	resp, err := d.do(ctx, http.MethodDelete, "/api/v1/volumes/"+volumeID+"/export", nil)
	if err != nil {
		return fmt.Errorf("sim driver: unexport volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sim driver: unexport volume: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (d *Driver) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.endpoint+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Backend-Id", d.backendID)
	return d.client.Do(req)
}
