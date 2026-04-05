//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/internal/network"
)

// TestE2EPolicyGroupAccessControl verifies that Policy/Group access control rules
// are correctly persisted and that port-group assignments are updated in the DB
// when policies are changed.
//
// Flow:
//  1. Create a Network with two Groups (allow-group, deny-group).
//  2. Create a VM attached to the Network (with allow-group).
//  3. Create a Policy allowing port 22 within allow-group.
//  4. Verify the policy is stored correctly (DB / API).
//  5. Delete the port-22 policy (simulate deny).
//  6. Verify the policy is gone from the DB.
//  7. Clean up.
func TestE2EPolicyGroupAccessControl(t *testing.T) {
	env := NewTestEnv(t)
	ctx := context.Background()

	tenantID := env.GetTenantID(t)
	hostID := env.GetHostID(t, "worker-1")

	// --- Step 1: Create Network ---
	net := env.CreateNetwork(t, tenantID, fmt.Sprintf("policy-e2e-net-%d", time.Now().UnixNano()))

	// --- Step 2: Create two Groups ---
	allowGroup := env.CreateGroup(t, net.ID, "allow-group")
	denyGroup := env.CreateGroup(t, net.ID, "deny-group")
	t.Logf("created allow-group=%s deny-group=%s", allowGroup.ID, denyGroup.ID)

	// --- Step 3: Create a Port (representing a VM's network port) in allow-group ---
	port := env.CreatePort(t, network.PortSpec{
		TenantID:  tenantID,
		NetworkID: net.ID,
		GroupID:   allowGroup.ID,
		HostID:    hostID,
		VMName:    fmt.Sprintf("policy-e2e-vm-%d", time.Now().UnixNano()),
	})
	t.Logf("created port %s ip=%s in allow-group", port.ID, port.IPAddress)

	// Verify the port is in allow-group
	if port.GroupID == nil || *port.GroupID != allowGroup.ID {
		t.Errorf("port group mismatch: expected %s got %v", allowGroup.ID, port.GroupID)
	}

	// --- Step 4: Create Policy allowing TCP port 22 within allow-group ---
	port22 := 22
	policy, err := env.NetStore.CreatePolicy(ctx, net.ID, network.PolicySpec{
		SrcGroupID: allowGroup.ID,
		DstGroupID: allowGroup.ID,
		Protocol:   "tcp",
		DstPort:    &port22,
		Action:     "allow",
	})
	if err != nil {
		t.Fatalf("create allow-ssh policy: %v", err)
	}
	t.Logf("created policy id=%s proto=tcp dstPort=22 action=allow", policy.ID)

	// --- Step 5: Verify policy is stored in the database ---
	policies, err := env.NetStore.ListPolicies(ctx, net.ID)
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}
	var foundAllow bool
	for _, p := range policies {
		if p.ID == policy.ID {
			foundAllow = true
			if p.Protocol != "tcp" {
				t.Errorf("policy protocol: expected tcp, got %s", p.Protocol)
			}
			if p.DstPort == nil || *p.DstPort != 22 {
				t.Errorf("policy dst_port: expected 22, got %v", p.DstPort)
			}
			if p.Action != "allow" {
				t.Errorf("policy action: expected allow, got %s", p.Action)
			}
			break
		}
	}
	if !foundAllow {
		t.Fatalf("allow-ssh policy %s not found in list after creation", policy.ID)
	}
	t.Logf("allow-ssh policy verified in DB: %s", policy.ID)

	// --- Step 6: Create a deny policy on denyGroup for illustration ---
	port80 := 80
	denyPolicy, err := env.NetStore.CreatePolicy(ctx, net.ID, network.PolicySpec{
		SrcGroupID: denyGroup.ID,
		DstGroupID: allowGroup.ID,
		Protocol:   "tcp",
		DstPort:    &port80,
		Action:     "deny",
	})
	if err != nil {
		t.Fatalf("create deny-http policy: %v", err)
	}
	t.Logf("created deny policy id=%s proto=tcp dstPort=80 action=deny", denyPolicy.ID)

	// Verify deny policy also in DB
	policies, err = env.NetStore.ListPolicies(ctx, net.ID)
	if err != nil {
		t.Fatalf("list policies after deny add: %v", err)
	}
	policyCount := len(policies)
	if policyCount < 2 {
		t.Errorf("expected at least 2 policies, got %d", policyCount)
	}
	t.Logf("total policies after deny add: %d", policyCount)

	// --- Step 7: Delete the allow-ssh policy (simulate port-22 rule removal) ---
	if err := env.NetStore.DeletePolicy(ctx, policy.ID); err != nil {
		t.Fatalf("delete allow-ssh policy: %v", err)
	}
	t.Logf("deleted allow-ssh policy %s", policy.ID)

	// --- Step 8: Verify allow-ssh policy is gone from DB ---
	policies, err = env.NetStore.ListPolicies(ctx, net.ID)
	if err != nil {
		t.Fatalf("list policies after allow delete: %v", err)
	}
	for _, p := range policies {
		if p.ID == policy.ID {
			t.Errorf("deleted policy %s still present in DB", policy.ID)
		}
	}
	t.Logf("allow-ssh policy correctly removed; remaining policies: %d", len(policies))

	// --- Step 9: Verify network state reflects updated policy count ---
	stateCtrl := network.NewStateController(env.DB, env.Logger)
	state, err := stateCtrl.ComputeHostNetworkState(ctx, hostID)
	if err != nil {
		t.Fatalf("compute host network state: %v", err)
	}
	t.Logf("host network state: %d ports, %d policies", len(state.Ports), len(state.Policies))

	if len(state.Ports) == 0 {
		t.Error("expected at least one port in computed network state")
	}

	// Ensure the deleted allow policy is not reflected in the computed state
	for _, sp := range state.Policies {
		if sp.PolicyId == policy.ID.String() {
			t.Errorf("deleted policy %s still present in computed host state", policy.ID)
		}
	}
	t.Logf("Policy/Group access control E2E test completed successfully")
}
