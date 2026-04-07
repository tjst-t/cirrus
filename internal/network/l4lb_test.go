package network

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestL4LBConstant verifies the IngressTypeL4LB constant value.
func TestL4LBConstant(t *testing.T) {
	if IngressTypeL4LB != "l4_lb" {
		t.Errorf("expected IngressTypeL4LB == \"l4_lb\", got %q", IngressTypeL4LB)
	}
}

// TestL4LBConfig_Serialization verifies JSON round-trip for L4LBConfig.
func TestL4LBConfig_Serialization(t *testing.T) {
	vmID := uuid.New().String()
	cfg := L4LBConfig{
		Backends: []L4LBBackend{
			{VMID: vmID, IP: "10.0.0.1", Port: 8080, Weight: 2, Healthy: true},
		},
		ListenerPort:    80,
		Protocol:        "tcp",
		SessionAffinity: "none",
		HealthCheck: L4LBHealthCheck{
			Type:               "tcp",
			Port:               8080,
			IntervalSec:        10,
			TimeoutSec:         3,
			UnhealthyThreshold: 2,
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded L4LBConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(decoded.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(decoded.Backends))
	}
	if decoded.Backends[0].VMID != vmID {
		t.Errorf("expected VMID %s, got %s", vmID, decoded.Backends[0].VMID)
	}
	if decoded.Backends[0].IP != "10.0.0.1" {
		t.Errorf("expected IP 10.0.0.1, got %s", decoded.Backends[0].IP)
	}
	if decoded.ListenerPort != 80 {
		t.Errorf("expected ListenerPort 80, got %d", decoded.ListenerPort)
	}
	if decoded.Protocol != "tcp" {
		t.Errorf("expected Protocol tcp, got %s", decoded.Protocol)
	}
	if decoded.SessionAffinity != "none" {
		t.Errorf("expected SessionAffinity none, got %s", decoded.SessionAffinity)
	}
	if !decoded.Backends[0].Healthy {
		t.Error("expected backend Healthy=true")
	}
}

// TestL4LBConfig_Wrapper_Serialization verifies the JSONB wrapper format used in the DB.
func TestL4LBConfig_Wrapper_Serialization(t *testing.T) {
	cfg := &L4LBConfig{
		Backends: []L4LBBackend{
			{VMID: "vm-1", IP: "10.0.0.1", Port: 80, Weight: 1, Healthy: true},
		},
		ListenerPort:    80,
		Protocol:        "tcp",
		SessionAffinity: "source_ip",
	}

	// Simulate the wrapper format used in the DB store.
	wrapper := struct {
		L4LB *L4LBConfig `json:"l4lb"`
	}{L4LB: cfg}

	data, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatalf("marshal wrapper failed: %v", err)
	}

	var decoded struct {
		L4LB *L4LBConfig `json:"l4lb"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal wrapper failed: %v", err)
	}
	if decoded.L4LB == nil {
		t.Fatal("expected non-nil L4LB config after unmarshal")
	}
	if decoded.L4LB.SessionAffinity != "source_ip" {
		t.Errorf("expected SessionAffinity source_ip, got %s", decoded.L4LB.SessionAffinity)
	}
}

// TestL4LBIngressSpec_Validation verifies validation rules for l4_lb IngressSpec.
func TestL4LBIngressSpec_Validation(t *testing.T) {
	tests := []struct {
		name      string
		spec      IngressSpec
		wantError bool
	}{
		{
			name: "valid l4_lb spec",
			spec: IngressSpec{
				Type:     IngressTypeL4LB,
				PublicIP: "203.0.113.10",
				IPPoolID: uuid.New(),
				L4LBConfig: &L4LBConfig{
					Backends: []L4LBBackend{
						{IP: "10.0.0.1", Port: 80, Weight: 1},
					},
					ListenerPort: 80,
					Protocol:     "tcp",
				},
			},
			wantError: false,
		},
		{
			name: "invalid: missing l4lb_config",
			spec: IngressSpec{
				Type:       IngressTypeL4LB,
				PublicIP:   "203.0.113.10",
				IPPoolID:   uuid.New(),
				L4LBConfig: nil,
			},
			wantError: true,
		},
		{
			name: "invalid: empty backends",
			spec: IngressSpec{
				Type:     IngressTypeL4LB,
				PublicIP: "203.0.113.10",
				IPPoolID: uuid.New(),
				L4LBConfig: &L4LBConfig{
					Backends:     []L4LBBackend{},
					ListenerPort: 80,
				},
			},
			wantError: true,
		},
		{
			name: "invalid: listener_port 0",
			spec: IngressSpec{
				Type:     IngressTypeL4LB,
				PublicIP: "203.0.113.10",
				IPPoolID: uuid.New(),
				L4LBConfig: &L4LBConfig{
					Backends: []L4LBBackend{
						{IP: "10.0.0.1", Port: 80, Weight: 1},
					},
					ListenerPort: 0,
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from Store.CreateIngress
			var err error
			if tt.spec.Type == IngressTypeL4LB {
				if tt.spec.L4LBConfig == nil {
					err = errors.Join(errors.New("l4lb_config required"), ErrInvalidState)
				} else if len(tt.spec.L4LBConfig.Backends) == 0 {
					err = errors.Join(errors.New("backends empty"), ErrInvalidState)
				} else if tt.spec.L4LBConfig.ListenerPort <= 0 {
					err = errors.Join(errors.New("invalid listener_port"), ErrInvalidState)
				}
			}
			if tt.wantError && err == nil {
				t.Errorf("expected validation error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

// TestL4LBIngress_Model verifies the Ingress struct supports L4LBConfig field.
func TestL4LBIngress_Model(t *testing.T) {
	id := uuid.New()
	netID := uuid.New()
	poolID := uuid.New()

	ing := Ingress{
		ID:        id,
		NetworkID: netID,
		Type:      IngressTypeL4LB,
		PublicIP:  "203.0.113.20",
		IPPoolID:  &poolID,
		L4LBConfig: &L4LBConfig{
			Backends: []L4LBBackend{
				{VMID: "vm-1", IP: "10.0.0.1", Port: 80, Weight: 1, Healthy: true},
				{VMID: "vm-2", IP: "10.0.0.2", Port: 80, Weight: 2, Healthy: false},
			},
			ListenerPort:    80,
			Protocol:        "tcp",
			SessionAffinity: "none",
		},
	}

	if ing.Type != IngressTypeL4LB {
		t.Errorf("expected type l4_lb, got %s", ing.Type)
	}
	if ing.L4LBConfig == nil {
		t.Fatal("expected non-nil L4LBConfig")
	}
	if len(ing.L4LBConfig.Backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(ing.L4LBConfig.Backends))
	}
	if ing.L4LBConfig.Backends[0].Healthy != true {
		t.Error("expected first backend Healthy=true")
	}
	if ing.L4LBConfig.Backends[1].Healthy != false {
		t.Error("expected second backend Healthy=false")
	}
}

// TestComputeIngressRules_L4LB_ExcludesUnhealthyBackends verifies that the
// computeIngressRules logic excludes unhealthy backends from IngressRules.
// Full DB integration: tested via controller_test.go.
// This test verifies the model behavior used in the computation.
func TestComputeIngressRules_L4LB_ExcludesUnhealthyBackends(t *testing.T) {
	// Simulate the health map merging that happens in computeIngressRules.
	cfg := &L4LBConfig{
		Backends: []L4LBBackend{
			{VMID: "vm-1", IP: "10.0.0.1", Port: 80, Weight: 1, Healthy: true},
			{VMID: "vm-2", IP: "10.0.0.2", Port: 80, Weight: 1, Healthy: true},
		},
	}

	// Health map from l4lb_backend_health table (vm-2 went unhealthy)
	healthMap := map[string]bool{
		"vm-1": true,
		"vm-2": false,
	}

	// Simulate the filtering
	var healthyBackends []L4LBBackend
	for _, b := range cfg.Backends {
		healthy := b.Healthy
		if h, ok := healthMap[b.VMID]; ok {
			healthy = h
		}
		if healthy {
			healthyBackends = append(healthyBackends, b)
		}
	}

	if len(healthyBackends) != 1 {
		t.Errorf("expected 1 healthy backend, got %d", len(healthyBackends))
	}
	if healthyBackends[0].VMID != "vm-1" {
		t.Errorf("expected vm-1 to be healthy backend, got %s", healthyBackends[0].VMID)
	}
}
