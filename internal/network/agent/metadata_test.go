package agent

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

func setupMetadataTest() *MetadataServer {
	cache := NewStateCache()
	cache.ApplyFull(&pb.HostNetworkStateUpdate{
		Full:    true,
		Version: 1,
		State: &pb.HostNetworkState{
			Ports: []*pb.PortState{
				makePort("p1", "vm1", "web-1", "net1", "prod", "g1", "web", "aa:bb:cc:dd:ee:01", "100.64.0.1", "100.64.0.2", 100),
			},
		},
	})

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return NewMetadataServer(cache, logger)
}

func TestMetadata_Root(t *testing.T) {
	srv := setupMetadataTest()

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "100.64.0.1:12345"
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("root: expected 200, got %d", w.Code)
	}
}

func TestMetadata_KnownVM(t *testing.T) {
	srv := setupMetadataTest()

	req := httptest.NewRequest("GET", "/meta-data/", nil)
	req.RemoteAddr = "100.64.0.1:12345"
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp MetadataResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.VMID != "vm1" {
		t.Errorf("vm_id = %q, want vm1", resp.VMID)
	}
	if resp.VMName != "web-1" {
		t.Errorf("vm_name = %q, want web-1", resp.VMName)
	}
	if resp.Hostname != "web-1" {
		t.Errorf("hostname = %q, want web-1", resp.Hostname)
	}
	if resp.NetworkName != "prod" {
		t.Errorf("network_name = %q, want prod", resp.NetworkName)
	}
	if resp.GroupName != "web" {
		t.Errorf("group_name = %q, want web", resp.GroupName)
	}
	if len(resp.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(resp.Interfaces))
	}
	iface := resp.Interfaces[0]
	if iface.IP != "100.64.0.1" {
		t.Errorf("ip = %q, want 100.64.0.1", iface.IP)
	}
	if iface.Gateway != "100.64.0.2" {
		t.Errorf("gateway = %q, want 100.64.0.2", iface.Gateway)
	}
	if iface.Netmask != "255.255.255.252" {
		t.Errorf("netmask = %q, want 255.255.255.252", iface.Netmask)
	}
}

func TestMetadata_UnknownVM(t *testing.T) {
	srv := setupMetadataTest()

	req := httptest.NewRequest("GET", "/meta-data/", nil)
	req.RemoteAddr = "10.0.0.99:12345" // unknown IP
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown VM, got %d", w.Code)
	}
}
