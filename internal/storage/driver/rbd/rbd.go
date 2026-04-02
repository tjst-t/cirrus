// Package rbd implements the storage.Driver interface for a Ceph RBD backend
// managed by cirrus-rbd-server (rbd/ceph CLI wrapper).
//
// ExportInfo params:
//
//	"monitor"   – "<ip>:6789"
//	"pool"      – Ceph pool name (default: "cirrus")
//	"image"     – RBD image name (volume ID)
//	"keyring"   – Ceph auth key string
//	"client_id" – Ceph client entity (e.g. "client.cirrus.<host_id>")
package rbd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tjst-t/cirrus/internal/storage"
)

// Driver is the Ceph RBD backend driver. It communicates with cirrus-rbd-server
// via HTTP to manage RBD images and client keyrings.
type Driver struct {
	endpoint  string // e.g. "http://10.100.0.101:8080"
	backendID string
	client    *http.Client
}

// New creates a new RBD Driver.
// endpoint is the cirrus-rbd-server base URL.
// config keys: none currently required.
func New(endpoint, backendID string, _ map[string]any) *Driver {
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
		"volume_id": spec.VolumeID,
		"size_gb":   spec.SizeGB,
	})
	resp, err := d.do(ctx, http.MethodPost, "/volumes", body)
	if err != nil {
		return nil, fmt.Errorf("rbd driver: create volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("rbd driver: create volume: unexpected status %d", resp.StatusCode)
	}
	return &storage.DriverVolume{VolumeID: spec.VolumeID, SizeGB: spec.SizeGB}, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, volumeID string) error {
	resp, err := d.do(ctx, http.MethodDelete, "/volumes/"+volumeID, nil)
	if err != nil {
		return fmt.Errorf("rbd driver: delete volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("rbd driver: delete volume: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (d *Driver) ResizeVolume(_ context.Context, _ string, _ int64) error {
	// RBD resize via rbd resize is possible but not implemented here.
	return fmt.Errorf("rbd driver: resize not supported")
}

// ExportVolume creates a Ceph client keyring for the host and returns
// the monitor address, pool, image, and keyring needed to map the volume.
func (d *Driver) ExportVolume(ctx context.Context, volumeID string, host storage.HostInfo) (*storage.ExportInfo, error) {
	body, _ := json.Marshal(map[string]any{
		"client_id": host.ID,
	})
	resp, err := d.do(ctx, http.MethodPost, "/volumes/"+volumeID+"/export", body)
	if err != nil {
		return nil, fmt.Errorf("rbd driver: export volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rbd driver: export volume: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Monitor  string `json:"monitor"`
		Pool     string `json:"pool"`
		Image    string `json:"image"`
		Keyring  string `json:"keyring"`
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("rbd driver: export volume: decode response: %w", err)
	}

	return &storage.ExportInfo{
		Protocol: "rbd",
		Params: map[string]string{
			"monitor":   result.Monitor,
			"pool":      result.Pool,
			"image":     result.Image,
			"keyring":   result.Keyring,
			"client_id": result.ClientID,
		},
	}, nil
}

// UnexportVolume removes the host's Ceph client keyring.
func (d *Driver) UnexportVolume(ctx context.Context, volumeID string, host storage.HostInfo) error {
	body, _ := json.Marshal(map[string]any{
		"client_id": host.ID,
	})
	resp, err := d.do(ctx, http.MethodDelete, "/volumes/"+volumeID+"/export", body)
	if err != nil {
		return fmt.Errorf("rbd driver: unexport volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("rbd driver: unexport volume: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func (d *Driver) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var buf *bytes.Reader
	if body != nil {
		buf = bytes.NewReader(body)
	} else {
		buf = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.endpoint+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return d.client.Do(req)
}
