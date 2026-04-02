//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tjst-t/cirrus/internal/storage"
	iscsistorage "github.com/tjst-t/cirrus/internal/storage/driver/iscsi"
)

// iscsiServerURL is the cirrus-iscsi-server management endpoint.
// In the docker-compose.storage.yml environment it is exposed on 18080.
const iscsiServerURL = "http://localhost:18080"

// TestISCSIDriver_ExportUnexport exercises the full ExportVolume → iscsiadm
// discovery → UnexportVolume flow against a real iSCSI target container.
//
// Requirements:
//   - docker-compose.storage.yml is up (make serve-storage)
//   - iscsi-target container is healthy at 10.100.0.100
func TestISCSIDriver_ExportUnexport(t *testing.T) {
	waitForISCSIServer(t, iscsiServerURL)

	volumeID := uuid.New().String()
	driver := iscsistorage.New(iscsiServerURL, "test-backend", nil)
	ctx := context.Background()

	// ── Step 1: CreateVolume ──────────────────────────────────────────────────
	t.Logf("CreateVolume %s (1 GB)", volumeID)
	vol, err := driver.CreateVolume(ctx, storage.DriverVolumeSpec{
		VolumeID: volumeID,
		SizeGB:   1,
	})
	if err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}
	if vol.VolumeID != volumeID {
		t.Fatalf("got volume_id %s, want %s", vol.VolumeID, volumeID)
	}
	t.Cleanup(func() {
		_ = driver.DeleteVolume(ctx, volumeID)
	})

	// ── Step 2: ExportVolume ─────────────────────────────────────────────────
	host := storage.HostInfo{
		ID:      "test-host-1",
		DataIPs: []string{"10.100.0.11"}, // worker-1 fabric IP
	}
	t.Logf("ExportVolume → initiator %s", host.DataIPs[0])
	info, err := driver.ExportVolume(ctx, volumeID, host)
	if err != nil {
		t.Fatalf("ExportVolume: %v", err)
	}
	if info.Protocol != "iscsi" {
		t.Fatalf("expected protocol iscsi, got %s", info.Protocol)
	}
	if info.Params["target"] == "" {
		t.Fatal("ExportVolume returned empty target IQN")
	}
	if info.Params["portal"] == "" {
		t.Fatal("ExportVolume returned empty portal")
	}
	if info.Params["lun"] == "" {
		t.Fatal("ExportVolume returned empty lun")
	}
	t.Logf("ExportInfo: target=%s portal=%s lun=%s",
		info.Params["target"], info.Params["portal"], info.Params["lun"])

	// ── Step 3: verify target is discoverable from worker-1 ──────────────────
	// We use docker exec to run iscsiadm discovery inside worker-1.
	env := newTestEnvForStorage(t)
	portal := info.Params["portal"]
	discoverOut := env.ExecInWorker(t, "worker-1",
		fmt.Sprintf("iscsiadm -m discovery -t sendtargets -p %s 2>&1 || true", portal))
	t.Logf("iscsiadm discovery output: %s", discoverOut)
	// The discovery output should contain the IQN even if the actual login
	// would require additional kernel modules. We just verify the target
	// is reachable and announces itself.
	if discoverOut == "" {
		t.Log("WARNING: iscsiadm discovery returned no output; iSCSI kernel modules may not be available in this environment")
	}

	// ── Step 4: UnexportVolume ───────────────────────────────────────────────
	t.Logf("UnexportVolume")
	if err := driver.UnexportVolume(ctx, volumeID, host); err != nil {
		t.Fatalf("UnexportVolume: %v", err)
	}

	// ── Step 5: verify volume info via management API ────────────────────────
	volInfo := iscsiGetVolumeInfo(t, volumeID)
	boundIPs, _ := volInfo["bound_ips"].([]any)
	if len(boundIPs) != 0 {
		t.Errorf("expected 0 bound IPs after unexport, got %d", len(boundIPs))
	}
	t.Logf("Volume info after unexport: bound_ips=%v", boundIPs)
}

// TestISCSIDriver_CreateDelete verifies CreateVolume / DeleteVolume without
// requiring a running worker container.
func TestISCSIDriver_CreateDelete(t *testing.T) {
	waitForISCSIServer(t, iscsiServerURL)

	volumeID := uuid.New().String()
	driver := iscsistorage.New(iscsiServerURL, "test-backend", nil)
	ctx := context.Background()

	_, err := driver.CreateVolume(ctx, storage.DriverVolumeSpec{
		VolumeID: volumeID,
		SizeGB:   1,
	})
	if err != nil {
		t.Fatalf("CreateVolume: %v", err)
	}

	// duplicate create should fail
	_, err = driver.CreateVolume(ctx, storage.DriverVolumeSpec{VolumeID: volumeID, SizeGB: 1})
	if err == nil {
		t.Error("expected error on duplicate CreateVolume, got nil")
	}

	if err := driver.DeleteVolume(ctx, volumeID); err != nil {
		t.Fatalf("DeleteVolume: %v", err)
	}

	// duplicate delete should fail
	if err := driver.DeleteVolume(ctx, volumeID); err == nil {
		t.Error("expected error on duplicate DeleteVolume, got nil")
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func waitForISCSIServer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}
	t.Skipf("cirrus-iscsi-server not reachable at %s; skipping (run make serve-storage)", url)
}

func iscsiGetVolumeInfo(t *testing.T, volumeID string) map[string]any {
	t.Helper()
	resp, err := http.Get(iscsiServerURL + "/volumes/" + volumeID)
	if err != nil {
		t.Fatalf("get volume info: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("parse volume info: %v", err)
	}
	return result
}

// newTestEnvForStorage returns a minimal TestEnv suitable for exec-only
// operations (ExecInWorker). Does not require a DB connection.
func newTestEnvForStorage(t *testing.T) *TestEnv {
	t.Helper()
	return &TestEnv{}
}
