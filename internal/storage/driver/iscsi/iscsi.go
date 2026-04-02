// Package iscsi implements the storage.Driver interface for a real iSCSI target
// managed by cirrus-iscsi-server (tgtd wrapper).
//
// ExportInfo params:
//
//	"portal"  – "<ip>:3260"
//	"target"  – IQN string
//	"lun"     – LUN number (always "1")
package iscsi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tjst-t/cirrus/internal/storage"
)

// Driver is the iSCSI backend driver. It communicates with cirrus-iscsi-server
// via HTTP to manage tgt targets.
type Driver struct {
	endpoint  string // e.g. "http://10.100.0.100:8080"
	backendID string
	client    *http.Client
}

// New creates a new iSCSI Driver.
// endpoint is the cirrus-iscsi-server base URL.
// config keys: none currently required.
func New(endpoint, backendID string, _ map[string]any) *Driver {
	return &Driver{
		endpoint:  endpoint,
		backendID: backendID,
		client:    &http.Client{},
	}
}

func (d *Driver) Capabilities() storage.DriverCapabilities {
	return storage.DriverCapabilities{}
}

func (d *Driver) CreateVolume(ctx context.Context, spec storage.DriverVolumeSpec) (*storage.DriverVolume, error) {
	body, _ := json.Marshal(map[string]any{
		"volume_id": spec.VolumeID,
		"size_gb":   spec.SizeGB,
	})
	resp, err := d.do(ctx, http.MethodPost, "/volumes", body)
	if err != nil {
		return nil, fmt.Errorf("iscsi driver: create volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("iscsi driver: create volume: unexpected status %d", resp.StatusCode)
	}
	return &storage.DriverVolume{VolumeID: spec.VolumeID, SizeGB: spec.SizeGB}, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, volumeID string) error {
	resp, err := d.do(ctx, http.MethodDelete, "/volumes/"+volumeID, nil)
	if err != nil {
		return fmt.Errorf("iscsi driver: delete volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("iscsi driver: delete volume: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func (d *Driver) ResizeVolume(_ context.Context, _ string, _ int64) error {
	// iSCSI target resize via tgt is not supported in this implementation.
	return fmt.Errorf("iscsi driver: resize not supported")
}

// ExportVolume binds the given host's first DataIP as an iSCSI initiator and
// returns connection info.
func (d *Driver) ExportVolume(ctx context.Context, volumeID string, host storage.HostInfo) (*storage.ExportInfo, error) {
	initiatorIP := initiatorIPFromHost(host)
	if initiatorIP == "" {
		return nil, fmt.Errorf("iscsi driver: host has no DataIPs")
	}

	body, _ := json.Marshal(map[string]any{
		"initiator_ip": initiatorIP,
	})
	resp, err := d.do(ctx, http.MethodPost, "/volumes/"+volumeID+"/export", body)
	if err != nil {
		return nil, fmt.Errorf("iscsi driver: export volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iscsi driver: export volume: unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Target string `json:"target"`
		Portal string `json:"portal"`
		LUN    int    `json:"lun"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("iscsi driver: export volume: decode response: %w", err)
	}

	return &storage.ExportInfo{
		Protocol: "iscsi",
		Params: map[string]string{
			"portal": result.Portal,
			"target": result.Target,
			"lun":    fmt.Sprintf("%d", result.LUN),
		},
	}, nil
}

// UnexportVolume revokes the host's iSCSI access.
func (d *Driver) UnexportVolume(ctx context.Context, volumeID string, host storage.HostInfo) error {
	initiatorIP := initiatorIPFromHost(host)
	if initiatorIP == "" {
		return fmt.Errorf("iscsi driver: host has no DataIPs")
	}

	body, _ := json.Marshal(map[string]any{
		"initiator_ip": initiatorIP,
	})
	resp, err := d.do(ctx, http.MethodDelete, "/volumes/"+volumeID+"/export", body)
	if err != nil {
		return fmt.Errorf("iscsi driver: unexport volume: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("iscsi driver: unexport volume: unexpected status %d", resp.StatusCode)
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

func initiatorIPFromHost(host storage.HostInfo) string {
	if len(host.DataIPs) > 0 {
		return host.DataIPs[0]
	}
	return ""
}
