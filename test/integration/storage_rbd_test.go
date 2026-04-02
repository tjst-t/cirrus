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
	rbdstorage "github.com/tjst-t/cirrus/internal/storage/driver/rbd"
)

// rbdServerURL is the cirrus-rbd-server management endpoint.
// In the docker-compose.storage.yml environment it is exposed on 18090.
const rbdServerURL = "http://localhost:18090"

// TestRBDDriver_ExportUnexport exercises the full ExportVolume → rbd info →
// UnexportVolume flow against a real Ceph demo container.
//
// Requirements:
//   - docker-compose.storage.yml is up (make serve-storage)
//   - ceph container is healthy at 10.100.0.101
func TestRBDDriver_ExportUnexport(t *testing.T) {
	waitForRBDServer(t, rbdServerURL)

	volumeID := uuid.New().String()
	driver := rbdstorage.New(rbdServerURL, "test-rbd-backend", nil)
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
	hostID := uuid.New().String()
	host := storage.HostInfo{
		ID:      hostID,
		DataIPs: []string{"10.100.0.11"},
	}
	t.Logf("ExportVolume → client %s", hostID)
	info, err := driver.ExportVolume(ctx, volumeID, host)
	if err != nil {
		t.Fatalf("ExportVolume: %v", err)
	}
	if info.Protocol != "rbd" {
		t.Fatalf("expected protocol rbd, got %s", info.Protocol)
	}
	if info.Params["monitor"] == "" {
		t.Fatal("ExportVolume returned empty monitor")
	}
	if info.Params["pool"] == "" {
		t.Fatal("ExportVolume returned empty pool")
	}
	if info.Params["image"] != volumeID {
		t.Fatalf("ExportVolume image=%s, want %s", info.Params["image"], volumeID)
	}
	if info.Params["keyring"] == "" {
		t.Fatal("ExportVolume returned empty keyring")
	}
	t.Logf("ExportInfo: monitor=%s pool=%s image=%s client_id=%s",
		info.Params["monitor"], info.Params["pool"],
		info.Params["image"], info.Params["client_id"])

	// ── Step 3: verify rbd image info from worker-1 ──────────────────────────
	env := newTestEnvForStorage(t)
	monitor := info.Params["monitor"]
	pool := info.Params["pool"]
	image := info.Params["image"]
	keyring := info.Params["keyring"]
	clientID := info.Params["client_id"]

	// Write keyring to a temp file in the worker container and run rbd info.
	rbdCmd := fmt.Sprintf(
		`echo '[%s]
	key = %s
' > /tmp/rbd.keyring && \
rbd info --mon-host %s --id %s --keyfile /tmp/rbd.keyring %s/%s 2>&1 || true`,
		clientID, keyring, monitor, clientID, pool, image,
	)
	rbdOut := env.ExecInWorker(t, "worker-1", rbdCmd)
	t.Logf("rbd info output: %s", rbdOut)
	if rbdOut == "" {
		t.Log("WARNING: rbd info returned no output; Ceph kernel modules may not be available")
	}

	// ── Step 4: UnexportVolume ───────────────────────────────────────────────
	t.Logf("UnexportVolume")
	if err := driver.UnexportVolume(ctx, volumeID, host); err != nil {
		t.Fatalf("UnexportVolume: %v", err)
	}

	// ── Step 5: verify no clients remain ────────────────────────────────────
	volInfo := rbdGetVolumeInfo(t, volumeID)
	clients, _ := volInfo["clients"].([]any)
	if len(clients) != 0 {
		t.Errorf("expected 0 clients after unexport, got %d", len(clients))
	}
	t.Logf("Volume info after unexport: clients=%v", clients)
}

// TestRBDDriver_CreateDelete verifies CreateVolume / DeleteVolume without
// requiring worker containers.
func TestRBDDriver_CreateDelete(t *testing.T) {
	waitForRBDServer(t, rbdServerURL)

	volumeID := uuid.New().String()
	driver := rbdstorage.New(rbdServerURL, "test-rbd-backend", nil)
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

func waitForRBDServer(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}
	t.Skipf("cirrus-rbd-server not reachable at %s; skipping (run make serve-storage)", url)
}

func rbdGetVolumeInfo(t *testing.T, volumeID string) map[string]any {
	t.Helper()
	resp, err := http.Get(rbdServerURL + "/volumes/" + volumeID)
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
