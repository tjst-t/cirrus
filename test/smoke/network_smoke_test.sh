#!/bin/bash
# Network Smoke Test — Sprint 5N-c
#
# Prerequisites: make serve (environment must be running)
#
# Usage:
#   ./test/smoke/network_smoke_test.sh
#
# This test verifies:
#   1. fabric_ip registration on all workers
#   2. Network/Group/Policy creation via CLI
#   3. Port creation via internal tool
#   4. OVS flow installation on workers
#   5. Geneve tunnel port creation for cross-host ports
#   6. Conntrack flows have correct ip match
#   7. Policy flows match expected rules
#   8. Network Reconciler state computation
set -euo pipefail

# ── Config ──
PORTMAN_ENV="${PORTMAN_ENV:-/tmp/cirrus-dev/portman.env}"
if [ ! -f "$PORTMAN_ENV" ]; then
  echo "FAIL: $PORTMAN_ENV not found. Run 'make serve' first."
  exit 1
fi
set -a; source "$PORTMAN_ENV"; set +a

CIRRUSCTL="./bin/cirrusctl"
CLI="$CIRRUSCTL --endpoint http://localhost:$API_PORT --token dev-token"
DSN="postgresql://cirrus:cirrus@localhost:$SIM_POSTGRES_PORT/cirrus?sslmode=disable"

PASS=0
FAIL=0
SKIP=0

pass() { PASS=$((PASS + 1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  FAIL: $1"; }
skip() { SKIP=$((SKIP + 1)); echo "  SKIP: $1"; }

docker_exec() {
  sudo docker exec "cirrus_${1}_1" sh -c "$2" 2>&1
}

# ── Pre-flight ──
echo "=== Network Smoke Test ==="
echo ""

# Check binaries
for bin in $CIRRUSCTL; do
  if [ ! -x "$bin" ]; then
    echo "FAIL: $bin not found. Run 'make build' first."
    exit 1
  fi
done

# Check docker
if ! sudo docker ps >/dev/null 2>&1; then
  echo "FAIL: docker not accessible"
  exit 1
fi

# ── Test 1: fabric_ip registration ──
echo "[Test 1] fabric_ip registration"
HOSTS_JSON=$(curl -sf -H "Authorization: Bearer dev-token" "http://localhost:$API_PORT/api/v1/hosts")

for worker in worker-1 worker-2 worker-3; do
  expected_ip=""
  case $worker in
    worker-1) expected_ip="10.100.0.11" ;;
    worker-2) expected_ip="10.100.0.12" ;;
    worker-3) expected_ip="10.100.0.13" ;;
  esac
  actual_ip=$(echo "$HOSTS_JSON" | python3 -c "
import sys, json
hosts = json.load(sys.stdin)
for h in hosts:
    if h['name'] == '$worker':
        print(h.get('fabric_ip', ''))
        break
")
  if [ "$actual_ip" = "$expected_ip" ]; then
    pass "$worker fabric_ip=$actual_ip"
  else
    fail "$worker fabric_ip expected=$expected_ip actual=$actual_ip"
  fi
done

# Get IDs for later use
HOST1_ID=$(echo "$HOSTS_JSON" | python3 -c "import sys,json; [print(h['id']) for h in json.load(sys.stdin) if h['name']=='worker-1']")
HOST2_ID=$(echo "$HOSTS_JSON" | python3 -c "import sys,json; [print(h['id']) for h in json.load(sys.stdin) if h['name']=='worker-2']")

# ── Test 2: Network/Group/Policy via CLI ──
echo ""
echo "[Test 2] Network/Group/Policy creation via CLI"

# Create org/tenant if needed
$CLI org create smoke-org >/dev/null 2>&1 || true
$CLI tenant create smoke-org smoke-tenant >/dev/null 2>&1 || true
TF="--org smoke-org --tenant smoke-tenant"

# Create network
NET_OUT=$($CLI network create $TF smoke-net 2>&1) || { fail "network create: $NET_OUT"; exit 1; }
NET_ID=$(echo "$NET_OUT" | tail -1 | awk '{print $1}')
if [ -n "$NET_ID" ] && [ "$NET_ID" != "Error:" ]; then
  pass "network create id=$NET_ID"
else
  fail "network create: $NET_OUT"
  exit 1
fi

# Create groups
GRP_OUT=$($CLI group create $TF --network smoke-net web 2>&1) || { fail "group create web"; exit 1; }
GROUP_ID=$(echo "$GRP_OUT" | tail -1 | awk '{print $1}')
pass "group create web id=$GROUP_ID"

# Create policy
POL_OUT=$($CLI policy create $TF --network smoke-net \
  --src-group web --dst-group web \
  --protocol tcp --dst-port 443 --action allow 2>&1) || { fail "policy create"; exit 1; }
pass "policy create tcp/443 allow"

# Get tenant ID
TENANT_ID=$(curl -sf -H "Authorization: Bearer dev-token" \
  "http://localhost:$API_PORT/api/v1/organizations" | python3 -c "
import sys, json
orgs = json.load(sys.stdin)
org_id = [o['id'] for o in orgs if o['name']=='smoke-org'][0]
print(org_id)
" 2>/dev/null)
TENANT_ID=$(curl -sf -H "Authorization: Bearer dev-token" \
  "http://localhost:$API_PORT/api/v1/organizations/$TENANT_ID/tenants" | python3 -c "
import sys, json
tenants = json.load(sys.stdin)
print([t['id'] for t in tenants if t['name']=='smoke-tenant'][0])
" 2>/dev/null)

# ── Test 3: Port creation ──
echo ""
echo "[Test 3] Port creation"

PORT1_OUT=$(go run ./cmd/internal/createport "$DSN" "$NET_ID" "$GROUP_ID" "$TENANT_ID" "$HOST1_ID" "smoke-vm-1" 2>&1)
if echo "$PORT1_OUT" | grep -q "port_id="; then
  pass "port on worker-1: $PORT1_OUT"
else
  fail "port on worker-1: $PORT1_OUT"
fi

PORT2_OUT=$(go run ./cmd/internal/createport "$DSN" "$NET_ID" "$GROUP_ID" "$TENANT_ID" "$HOST2_ID" "smoke-vm-2" 2>&1)
if echo "$PORT2_OUT" | grep -q "port_id="; then
  pass "port on worker-2: $PORT2_OUT"
else
  fail "port on worker-2: $PORT2_OUT"
fi

# ── Test 4: OVS flow installation ──
echo ""
echo "[Test 4] OVS flow installation (waiting up to 30s)"

FLOW_OK=false
for i in $(seq 1 15); do
  FLOWS=$(docker_exec worker-1 "ovs-ofctl dump-flows br-int table=0" 2>/dev/null || true)
  # Count non-header flow lines
  FLOW_COUNT=$(echo "$FLOWS" | grep -c "priority=" || true)
  if [ "$FLOW_COUNT" -ge 2 ]; then
    FLOW_OK=true
    break
  fi
  sleep 2
done

if $FLOW_OK; then
  pass "flows in table 0 ($FLOW_COUNT rules)"
else
  fail "no flows appeared in table 0 after 30s"
fi

# Check each table has flows
for t in 1 2 3 4; do
  COUNT=$(docker_exec worker-1 "ovs-ofctl dump-flows br-int table=$t" | grep -c "priority=" || true)
  if [ "$COUNT" -gt 0 ]; then
    pass "table $t has $COUNT flows"
  else
    fail "table $t has no flows"
  fi
done

# ── Test 5: Geneve tunnel port ──
echo ""
echo "[Test 5] Geneve tunnel ports"

W1_PORTS=$(docker_exec worker-1 "ovs-vsctl list-ports br-int")
if echo "$W1_PORTS" | grep -q "gn_"; then
  pass "worker-1 has tunnel port: $(echo "$W1_PORTS" | grep gn_)"
else
  fail "worker-1 has no tunnel port"
fi

W2_PORTS=$(docker_exec worker-2 "ovs-vsctl list-ports br-int")
if echo "$W2_PORTS" | grep -q "gn_"; then
  pass "worker-2 has tunnel port: $(echo "$W2_PORTS" | grep gn_)"
else
  fail "worker-2 has no tunnel port"
fi

# ── Test 6: Conntrack flows have ip match ──
echo ""
echo "[Test 6] Conntrack flows (ip match)"

CT_FLOWS=$(docker_exec worker-1 "ovs-ofctl dump-flows br-int table=1")

# All ct_state flows should have ",ip" in the match
CT_STATE_LINES=$(echo "$CT_FLOWS" | grep "ct_state=" || true)
BAD_CT=0
while IFS= read -r line; do
  if [ -n "$line" ] && ! echo "$line" | grep -q ",ip"; then
    BAD_CT=$((BAD_CT + 1))
    fail "ct_state flow missing ip match: $line"
  fi
done <<< "$CT_STATE_LINES"

if [ "$BAD_CT" -eq 0 ]; then
  pass "all ct_state flows have ip match"
fi

# Check untracked rule (priority=50,ip → ct)
if echo "$CT_FLOWS" | grep -q "priority=50,ip actions=ct"; then
  pass "untracked → ct(table=1) rule present"
else
  fail "untracked → ct rule not found"
fi

# ── Test 7: Policy flow ──
echo ""
echo "[Test 7] Policy flows (waiting up to 30s for streaming)"

POL_OK=false
for i in $(seq 1 15); do
  POL_FLOWS=$(docker_exec worker-1 "ovs-ofctl dump-flows br-int table=3")
  if echo "$POL_FLOWS" | grep -q "tcp.*tp_dst=443.*ct(commit)"; then
    POL_OK=true
    break
  fi
  sleep 2
done

if $POL_OK; then
  pass "TCP/443 allow policy flow present"
else
  fail "TCP/443 allow policy flow not found in: $POL_FLOWS"
fi

# ── Test 8: Tunnel dst uses fabric_ip ──
echo ""
echo "[Test 8] Tunnel destination uses fabric_ip"

HOST_RES_FLOWS=$(docker_exec worker-1 "ovs-ofctl dump-flows br-int table=4")
# 10.100.0.12 = 0x0a64000c
if echo "$HOST_RES_FLOWS" | grep -q "0xa64000c"; then
  pass "tun_dst=10.100.0.12 (worker-2 fabric_ip)"
else
  fail "expected tun_dst 10.100.0.12 not found in: $HOST_RES_FLOWS"
fi

# ── Cleanup ──
echo ""
echo "=== Results ==="
echo "  PASS: $PASS"
echo "  FAIL: $FAIL"
echo "  SKIP: $SKIP"
echo ""

if [ "$FAIL" -gt 0 ]; then
  echo "FAILED"
  exit 1
else
  echo "ALL PASSED"
  exit 0
fi
