//go:build integration

package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/controller/reconcile"
	"github.com/tjst-t/cirrus/internal/host"
	"github.com/tjst-t/cirrus/internal/network"
)

// Ensure reconcile package types are used (OVSFlowVerifier is implemented by mockOVSFlowVerifier).
var _ reconcile.OVSFlowVerifier = (*mockOVSFlowVerifier)(nil)

// mockOVSFlowVerifier is an in-memory OVSFlowVerifier implementation for testing.
// When failHosts is non-empty, VerifyFlows returns an error for those host IDs.
type mockOVSFlowVerifier struct {
	failHosts map[uuid.UUID]string // hostID → error message
}

func newMockOVSFlowVerifier() *mockOVSFlowVerifier {
	return &mockOVSFlowVerifier{failHosts: make(map[uuid.UUID]string)}
}

// SetFlowMissing registers hostID to return a flow-missing error.
func (m *mockOVSFlowVerifier) SetFlowMissing(hostID uuid.UUID, msg string) {
	m.failHosts[hostID] = msg
}

// ClearFlowMissing removes a previously registered failure.
func (m *mockOVSFlowVerifier) ClearFlowMissing(hostID uuid.UUID) {
	delete(m.failHosts, hostID)
}

// VerifyFlows implements OVSFlowVerifier.
func (m *mockOVSFlowVerifier) VerifyFlows(_ context.Context, hostID uuid.UUID) error {
	if msg, bad := m.failHosts[hostID]; bad {
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// --- Integration test ---

// TestReconcileNetwork_FlowMissing verifies that when OVSFlowVerifier reports a
// missing flow, the NetworkReconciler fires a drift_event of type "flow_missing"
// and it is recorded in the drift_events table.
//
// This test uses a mock OVSFlowVerifier so it works in cirrus-sim environments
// without a real OVS bridge.
//
// Prerequisites:
//
//	TEST_DB_DSN — DB connection string (default: postgres://cirrus:cirrus@localhost:5432/cirrus)
func TestReconcileNetwork_FlowMissing(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgresql://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("DB not reachable (%v); skipping network reconcile integration test", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Build a real DriftHandler (with DB persistence, no auto-heal).
	driftHandler := reconcile.NewDriftHandler(reconcile.DriftHandlerConfig{
		Pool:            pool,
		Logger:          logger,
		AutoHealEnabled: false,
		DedupTTL:        0, // use default (10m); we use unique host IDs per run
	})

	// Create the NetworkReconciler.  We need real host.Service and
	// network.StateController pointing to the same DB.
	// host.Store implements host.Service directly.
	hostSvc := host.NewStore(pool)

	stateCtrl := network.NewStateController(pool, logger)
	reconciler := reconcile.NewNetworkReconciler(stateCtrl, hostSvc, driftHandler, logger, 5*time.Minute)

	// Create a mock flow verifier with a simulated missing-flow condition.
	verifier := newMockOVSFlowVerifier()

	// Pick a host that actually exists in the DB.
	hosts, err := hostSvc.ListHostsByState(context.Background(), host.StateActive)
	if err != nil || len(hosts) == 0 {
		t.Skip("no active hosts in DB; skipping network reconcile integration test")
	}
	targetHost := hosts[0]
	t.Logf("using host %s (%s) for flow verification test", targetHost.Name, targetHost.ID)

	// Register the flow-missing condition for this host.
	verifier.SetFlowMissing(targetHost.ID, "table=0 miss-flow absent")

	// Inject verifier into reconciler.
	reconciler.WithOVSFlowVerifier(verifier)

	// Record pre-test drift_event count to detect new rows.
	var prevCount int
	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM drift_events
		 WHERE resource_id = $1 AND type = 'flow_missing'`,
		targetHost.ID.String(),
	).Scan(&prevCount)
	if err != nil {
		t.Fatalf("query drift_events: %v", err)
	}

	// Run a single reconciliation pass by calling the exported RunForTest method.
	// Since NetworkReconciler.reconcileOnce is unexported, we use Run with a
	// short-lived context that gets cancelled after the first iteration.
	runCtx, runCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer runCancel()

	// reconcileOnce is called immediately when Run starts (before the ticker).
	// We cancel the context after giving it time to complete one pass.
	done := make(chan error, 1)
	go func() {
		done <- reconciler.Run(runCtx)
	}()

	// Give it 5 seconds to complete the first pass.
	time.Sleep(5 * time.Second)
	runCancel()
	<-done

	// Assert: drift_events should now contain a flow_missing event for this host.
	var newCount int
	err = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM drift_events
		 WHERE resource_id = $1 AND type = 'flow_missing'`,
		targetHost.ID.String(),
	).Scan(&newCount)
	if err != nil {
		t.Fatalf("query drift_events after reconcile: %v", err)
	}

	if newCount <= prevCount {
		t.Fatalf("expected new flow_missing drift event for host %s; prevCount=%d newCount=%d",
			targetHost.ID, prevCount, newCount)
	}
	t.Logf("flow_missing drift event recorded for host %s (count: %d → %d)",
		targetHost.ID, prevCount, newCount)

	// Verify the content of the last event.
	var layer, evType, severity, detectedBy, actual string
	err = pool.QueryRow(context.Background(),
		`SELECT layer, type, severity, detected_by, actual
		 FROM drift_events
		 WHERE resource_id = $1 AND type = 'flow_missing'
		 ORDER BY created_at DESC LIMIT 1`,
		targetHost.ID.String(),
	).Scan(&layer, &evType, &severity, &detectedBy, &actual)
	if err != nil {
		t.Fatalf("scan drift_event: %v", err)
	}
	t.Logf("drift_event: layer=%s type=%s severity=%s detected_by=%s actual=%q",
		layer, evType, severity, detectedBy, actual)

	if layer != "network" {
		t.Errorf("expected layer=network, got %s", layer)
	}
	if evType != "flow_missing" {
		t.Errorf("expected type=flow_missing, got %s", evType)
	}
	if detectedBy != "network_reconciler" {
		t.Errorf("expected detected_by=network_reconciler, got %s", detectedBy)
	}
}
