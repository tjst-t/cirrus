package ovs

import (
	"errors"
	"testing"
)

func TestAddAndGetFlows(t *testing.T) {
	m := New("br-int")

	if err := m.AddFlow(0, 100, "in_port=1", "output:2"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddFlow(0, 50, "in_port=2", "output:1"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddFlow(1, 100, "dl_dst=ff:ff:ff:ff:ff:ff", "flood"); err != nil {
		t.Fatal(err)
	}

	flows, err := m.GetFlows(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 2 {
		t.Fatalf("expected 2 flows in table 0, got %d", len(flows))
	}

	flows, err = m.GetFlows(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow in table 1, got %d", len(flows))
	}

	flows, err = m.GetFlows(99)
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 0 {
		t.Fatalf("expected 0 flows in table 99, got %d", len(flows))
	}
}

func TestDeleteFlow(t *testing.T) {
	m := New("br-int")

	m.AddFlow(0, 100, "in_port=1", "output:2")
	m.AddFlow(0, 50, "in_port=2", "output:1")

	if err := m.DeleteFlow(0, "in_port=1"); err != nil {
		t.Fatal(err)
	}

	flows, _ := m.GetFlows(0)
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow after delete, got %d", len(flows))
	}
	if flows[0].Match != "in_port=2" {
		t.Fatalf("wrong flow remaining: %s", flows[0].Match)
	}
}

func TestAddAndDeletePort(t *testing.T) {
	m := New("br-int")

	if err := m.AddPort("br-int", "veth0"); err != nil {
		t.Fatal(err)
	}
	if err := m.AddPort("br-int", "veth1"); err != nil {
		t.Fatal(err)
	}

	// Duplicate port should error
	if err := m.AddPort("br-int", "veth0"); err == nil {
		t.Fatal("expected error adding duplicate port")
	}

	ports, err := m.GetPorts("br-int")
	if err != nil {
		t.Fatal(err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(ports))
	}

	if err := m.DeletePort("br-int", "veth0"); err != nil {
		t.Fatal(err)
	}

	ports, _ = m.GetPorts("br-int")
	if len(ports) != 1 {
		t.Fatalf("expected 1 port after delete, got %d", len(ports))
	}

	// Delete non-existent port should error
	if err := m.DeletePort("br-int", "veth99"); err == nil {
		t.Fatal("expected error deleting non-existent port")
	}
}

func TestSetInterfaceExternalIDs(t *testing.T) {
	m := New("br-int")

	err := m.SetInterfaceExternalIDs("veth0", map[string]string{
		"iface-id": "port-123",
		"attached-mac": "fa:16:3e:00:00:01",
	})
	if err != nil {
		t.Fatal(err)
	}

	ids := m.GetInterfaceExternalIDs("veth0")
	if ids["iface-id"] != "port-123" {
		t.Fatalf("expected iface-id=port-123, got %s", ids["iface-id"])
	}

	// Update existing
	m.SetInterfaceExternalIDs("veth0", map[string]string{"iface-id": "port-456"})
	ids = m.GetInterfaceExternalIDs("veth0")
	if ids["iface-id"] != "port-456" {
		t.Fatalf("expected updated iface-id=port-456, got %s", ids["iface-id"])
	}
}

func TestRecordedCommands(t *testing.T) {
	m := New("br-int")

	m.AddFlow(0, 100, "in_port=1", "output:2")
	m.AddPort("br-int", "veth0")
	m.DeleteFlow(0, "in_port=1")
	m.DeletePort("br-int", "veth0")

	cmds := m.GetRecordedCommands()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 recorded commands, got %d", len(cmds))
	}

	expected := []string{"add-flow", "add-port", "delete-flow", "delete-port"}
	for i, op := range expected {
		if cmds[i].Op != op {
			t.Fatalf("command %d: expected op %s, got %s", i, op, cmds[i].Op)
		}
	}
}

func TestReset(t *testing.T) {
	m := New("br-int")

	m.AddFlow(0, 100, "in_port=1", "output:2")
	m.AddPort("br-int", "veth0")

	m.Reset()

	flows, _ := m.GetFlows(0)
	if len(flows) != 0 {
		t.Fatalf("expected 0 flows after reset, got %d", len(flows))
	}
	ports, _ := m.GetPorts("br-int")
	if len(ports) != 0 {
		t.Fatalf("expected 0 ports after reset, got %d", len(ports))
	}
	cmds := m.GetRecordedCommands()
	if len(cmds) != 0 {
		t.Fatalf("expected 0 commands after reset, got %d", len(cmds))
	}
}

func TestInjectError(t *testing.T) {
	m := New("br-int")
	injected := errors.New("simulated OVS failure")

	m.InjectError("add-flow", injected)
	if err := m.AddFlow(0, 100, "in_port=1", "output:2"); !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got %v", err)
	}

	// Flows should not have been added
	flows, _ := m.GetFlows(0)
	if len(flows) != 0 {
		t.Fatal("flow should not be added when error is injected")
	}

	// Clear error
	m.InjectError("add-flow", nil)
	if err := m.AddFlow(0, 100, "in_port=1", "output:2"); err != nil {
		t.Fatalf("expected no error after clearing injection, got %v", err)
	}
}

func TestInterfaceCompliance(t *testing.T) {
	// Ensure MockClient implements Client
	var _ Client = (*MockClient)(nil)
}
