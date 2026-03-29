package datagen

import (
	"context"
	"testing"
)

const testYAML = `
environment:
  name: "test-env"
  sites:
    - name: "site-a"
      rack_rows: 2
      racks_per_row: 3
      hosts_per_rack: 2
      host_template: "standard"
  host_templates:
    standard:
      cpu_model: "Xeon Gold"
      cpu_sockets: 2
      cores_per_socket: 28
      threads_per_core: 2
      memory_gb: 512
      numa_nodes: 2
  storage_backends:
    - name: "ceph-ssd"
      type: "ceph"
      total_capacity_tb: 100
      total_iops: 100000
      capabilities: ["ssd", "snapshot"]
  ovn_clusters:
    - name: "ovn-1"
      covers: ["site-a"]
`

func TestGenerate(t *testing.T) {
	g := New()
	ctx := context.Background()

	result, err := g.Generate(ctx, []byte(testYAML))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if result.Name != "test-env" {
		t.Errorf("name = %q, want %q", result.Name, "test-env")
	}

	// 2 rows * 3 racks * 2 hosts = 12 hosts
	expectedHosts := 2 * 3 * 2
	if len(result.Hosts) != expectedHosts {
		t.Errorf("hosts = %d, want %d", len(result.Hosts), expectedHosts)
	}

	if len(result.Backends) != 1 {
		t.Errorf("backends = %d, want 1", len(result.Backends))
	}

	if result.Backends[0].TotalCapacityGB != 100*1024 {
		t.Errorf("capacity = %d, want %d", result.Backends[0].TotalCapacityGB, 100*1024)
	}

	// Check host details
	h := result.Hosts[0]
	if h.CPUModel != "Xeon Gold" {
		t.Errorf("cpu_model = %q", h.CPUModel)
	}
	if h.MemoryMB != 512*1024 {
		t.Errorf("memory_mb = %d, want %d", h.MemoryMB, 512*1024)
	}
	if h.LibvirtPort < 16510 {
		t.Errorf("libvirt_port = %d, expected >= 16510", h.LibvirtPort)
	}
}

func TestGenerateStatus(t *testing.T) {
	g := New()
	ctx := context.Background()

	status := g.GetStatus(ctx)
	if status.State != "idle" {
		t.Errorf("initial state = %q, want idle", status.State)
	}

	if _, err := g.Generate(ctx, []byte(testYAML)); err != nil {
		t.Fatal(err)
	}

	status = g.GetStatus(ctx)
	if status.State != "completed" {
		t.Errorf("after generate state = %q, want completed", status.State)
	}
}

func TestGenerateInvalidYAML(t *testing.T) {
	g := New()
	ctx := context.Background()

	_, err := g.Generate(ctx, []byte("invalid: [yaml"))
	if err == nil {
		t.Error("expected error for invalid YAML")
	}

	status := g.GetStatus(ctx)
	if status.State != "failed" {
		t.Errorf("state = %q, want failed", status.State)
	}
}

func TestReset(t *testing.T) {
	g := New()
	ctx := context.Background()

	if _, err := g.Generate(ctx, []byte(testYAML)); err != nil {
		t.Fatal(err)
	}

	g.Reset(ctx)

	if g.GetResult(ctx) != nil {
		t.Error("expected nil result after reset")
	}
	if g.GetStatus(ctx).State != "idle" {
		t.Error("expected idle state after reset")
	}
}
