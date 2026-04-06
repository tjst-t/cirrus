//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"

	"github.com/tjst-t/cirrus/internal/network"
)

// TestEnv provides helpers for integration tests.
type TestEnv struct {
	DB       *pgxpool.Pool
	NetStore *network.Store
	Logger   *slog.Logger
}

// NewTestEnv creates a test environment connected to the integration database.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgresql://cirrus:cirrus@localhost:5432/cirrus?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test DB: %v", err)
	}

	t.Cleanup(func() { pool.Close() })

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	return &TestEnv{
		DB:       pool,
		NetStore: network.NewStore(pool, logger, nil),
		Logger:   logger,
	}
}

// GetHostID returns the host ID for a named worker (e.g. "worker-1").
func (e *TestEnv) GetHostID(t *testing.T, hostname string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := e.DB.QueryRow(context.Background(),
		`SELECT id FROM hosts WHERE name = $1`, hostname,
	).Scan(&id)
	if err != nil {
		t.Fatalf("get host ID for %s: %v", hostname, err)
	}
	return id
}

// GetTenantID returns the first tenant ID in the database, creating one if needed.
func (e *TestEnv) GetTenantID(t *testing.T) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := e.DB.QueryRow(context.Background(),
		`SELECT id FROM tenants LIMIT 1`,
	).Scan(&id)
	if err != nil {
		// Create a tenant if none exists
		err = e.DB.QueryRow(context.Background(),
			`INSERT INTO tenants (name) VALUES ('test-tenant') RETURNING id`,
		).Scan(&id)
		if err != nil {
			t.Fatalf("create test tenant: %v", err)
		}
	}
	return id
}

// CreateNetwork creates a test network.
func (e *TestEnv) CreateNetwork(t *testing.T, tenantID uuid.UUID, name string) *network.Network {
	t.Helper()
	net, err := e.NetStore.CreateNetwork(context.Background(), tenantID, network.NetworkSpec{
		Name: name,
	})
	if err != nil {
		t.Fatalf("create network %s: %v", name, err)
	}
	t.Logf("created network %s: id=%s cidr=%s vni=%d", name, net.ID, net.CIDR, net.VNI)
	return net
}

// CreateGroup creates a test group.
func (e *TestEnv) CreateGroup(t *testing.T, networkID uuid.UUID, name string) *network.Group {
	t.Helper()
	g, err := e.NetStore.CreateGroup(context.Background(), networkID, network.GroupSpec{
		Name: name,
	})
	if err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return g
}

// CreatePolicy creates a test policy.
func (e *TestEnv) CreatePolicy(t *testing.T, networkID uuid.UUID, spec network.PolicySpec) *network.Policy {
	t.Helper()
	p, err := e.NetStore.CreatePolicy(context.Background(), networkID, spec)
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	return p
}

// CreatePort creates a test port.
func (e *TestEnv) CreatePort(t *testing.T, spec network.PortSpec) *network.Port {
	t.Helper()
	p, err := e.NetStore.CreatePort(context.Background(), spec)
	if err != nil {
		t.Fatalf("create port: %v", err)
	}
	t.Logf("created port: id=%s ip=%s mac=%s host=%s", p.ID, p.IPAddress, p.MACAddress, spec.HostID)
	return p
}

// ExecInWorker runs a command inside a worker container.
// Supports both Docker Compose v1 naming ({project}_{service}_1) and
// v2 naming ({project}-{service}-1), trying v1 first.
func (e *TestEnv) ExecInWorker(t *testing.T, workerName, command string) string {
	t.Helper()
	project := os.Getenv("COMPOSE_PROJECT_NAME")
	if project == "" {
		project = "integration"
	}
	// Try Docker Compose v1 name first (underscores), then v2 (hyphens).
	candidates := []string{
		fmt.Sprintf("%s_%s_1", project, workerName),
		fmt.Sprintf("%s-%s-1", project, workerName),
	}
	for _, containerName := range candidates {
		out, err := exec.Command("docker", "exec", containerName, "sh", "-c", command).CombinedOutput()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
		// Only try next candidate if the container was not found.
		if strings.Contains(string(out), "No such container") {
			continue
		}
		// Container found but command failed — return output and log.
		t.Logf("exec in %s (%s) failed: %v\noutput: %s", workerName, containerName, err, string(out))
		return strings.TrimSpace(string(out))
	}
	t.Logf("exec in %s: container not found (tried %v)", workerName, candidates)
	return ""
}

// WaitForFlows polls a worker's OVS bridge until flows appear in the given table,
// or until timeout.
func (e *TestEnv) WaitForFlows(t *testing.T, workerName string, tableID int, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	cmd := fmt.Sprintf("ovs-ofctl dump-flows br-int table=%d", tableID)

	for time.Now().Before(deadline) {
		out := e.ExecInWorker(t, workerName, cmd)
		// Count non-header lines (actual flows)
		lines := strings.Split(out, "\n")
		flowCount := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "NXST_FLOW") && !strings.HasPrefix(line, "OFPST_FLOW") {
				flowCount++
			}
		}
		if flowCount > 0 {
			return out
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for flows in table %d on %s", tableID, workerName)
	return ""
}

// decodeJSON reads a JSON HTTP response body into dest.
func decodeJSON(resp *http.Response, dest any) error {
	return json.NewDecoder(resp.Body).Decode(dest)
}
