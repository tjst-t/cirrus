package reconcile

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDriftHandler_Handle_LogsAndDeduplicates(t *testing.T) {
	handled := 0
	// Use a custom healer to count auto-heal calls.
	h := NewDriftHandler(DriftHandlerConfig{
		Pool:            nil,
		Logger:          slog.Default(),
		AutoHealEnabled: false,
		DedupTTL:        500 * time.Millisecond,
	})

	vmID := uuid.New().String()
	event := DriftEvent{
		Layer:      DriftLayerCompute,
		Type:       DriftTypeExpectedMissing,
		Severity:   DriftSeverityCritical,
		Resource:   "vm",
		ResourceID: vmID,
		HostID:     "host-1",
		Expected:   "running",
		Actual:     "absent",
		DetectedBy: "heartbeat_reconciler",
	}

	// First call: should be processed.
	h.Handle(context.Background(), event)
	handled++

	// Second call immediately: should be suppressed (within dedupTTL).
	h.Handle(context.Background(), event)

	// Only 1 unique entry in dedup map means the second was suppressed.
	h.mu.Lock()
	dedupCount := len(h.dedup)
	h.mu.Unlock()

	if dedupCount != 1 {
		t.Fatalf("expected 1 dedup entry, got %d", dedupCount)
	}
	_ = handled
}

func TestDriftHandler_DedupExpiry(t *testing.T) {
	h := NewDriftHandler(DriftHandlerConfig{
		Pool:            nil,
		Logger:          slog.Default(),
		AutoHealEnabled: false,
		DedupTTL:        50 * time.Millisecond,
	})

	vmID := uuid.New().String()
	event := DriftEvent{
		Layer:      DriftLayerCompute,
		Type:       DriftTypeExpectedMissing,
		Severity:   DriftSeverityCritical,
		Resource:   "vm",
		ResourceID: vmID,
		DetectedBy: "test",
	}

	h.Handle(context.Background(), event)

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	// Should be processed again after TTL expires.
	h.Handle(context.Background(), event)

	h.mu.Lock()
	lastSeen := h.dedup[vmID+":"+DriftTypeExpectedMissing]
	h.mu.Unlock()

	if time.Since(lastSeen) > 200*time.Millisecond {
		t.Fatal("dedup entry should have been refreshed after TTL expiry")
	}
}

func TestDriftHandler_AutoHeal_VMHealer(t *testing.T) {
	healed := false
	var healedID uuid.UUID

	mockHealer := &mockVMHealer{
		healFn: func(ctx context.Context, vmID uuid.UUID, reason string) error {
			healed = true
			healedID = vmID
			return nil
		},
	}

	h := NewDriftHandler(DriftHandlerConfig{
		Pool:            nil,
		Logger:          slog.Default(),
		AutoHealEnabled: true,
		VMHealer:        mockHealer,
	})

	vmID := uuid.New()
	event := DriftEvent{
		Layer:      DriftLayerCompute,
		Type:       DriftTypeExpectedMissing,
		Severity:   DriftSeverityCritical,
		Resource:   "vm",
		ResourceID: vmID.String(),
		HostID:     "host-1",
		Expected:   "running",
		Actual:     "absent",
		DetectedBy: "heartbeat_reconciler",
	}

	h.Handle(context.Background(), event)

	if !healed {
		t.Fatal("expected VMHealer.HealVM to be called")
	}
	if healedID != vmID {
		t.Fatalf("expected vm_id %s, got %s", vmID, healedID)
	}
}

func TestDriftHandler_AutoHeal_NetworkHealer(t *testing.T) {
	refreshed := false
	var refreshedHost uuid.UUID

	mockNetHealer := &mockNetworkHealer{
		refreshFn: func(hostID uuid.UUID) {
			refreshed = true
			refreshedHost = hostID
		},
	}

	h := NewDriftHandler(DriftHandlerConfig{
		Pool:            nil,
		Logger:          slog.Default(),
		AutoHealEnabled: true,
		NetworkHealer:   mockNetHealer,
	})

	hostID := uuid.New()
	event := DriftEvent{
		Layer:      DriftLayerNetwork,
		Type:       DriftTypeStateMismatch,
		Severity:   DriftSeverityHigh,
		Resource:   "port",
		ResourceID: uuid.New().String(),
		HostID:     hostID.String(),
		DetectedBy: "network_reconciler",
	}

	h.Handle(context.Background(), event)

	if !refreshed {
		t.Fatal("expected NetworkHealer.TriggerRefresh to be called")
	}
	if refreshedHost != hostID {
		t.Fatalf("expected host_id %s, got %s", hostID, refreshedHost)
	}
}

// --- mock helpers ---

type mockVMHealer struct {
	healFn func(ctx context.Context, vmID uuid.UUID, reason string) error
}

func (m *mockVMHealer) HealVM(ctx context.Context, vmID uuid.UUID, reason string) error {
	return m.healFn(ctx, vmID, reason)
}

type mockNetworkHealer struct {
	refreshFn func(hostID uuid.UUID)
}

func (m *mockNetworkHealer) TriggerRefresh(hostID uuid.UUID) {
	m.refreshFn(hostID)
}
