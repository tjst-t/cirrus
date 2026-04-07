# Sprint S043 — L4 LB Ingress: Implementation Notes

## What Was Implemented

Story S043-1: L4 LB Ingress — complete implementation of `ingress type=l4_lb` with 5-tuple hash-based load balancing using OVS `select` groups, session affinity by source IP, controller-directed health checks, and CLI/API support.

## Files Created

| File | Purpose |
|------|---------|
| `internal/state/migrations/000025_l4lb_backend_health.up.sql` | New `l4lb_backend_health` table migration |
| `internal/state/migrations/000025_l4lb_backend_health.down.sql` | Rollback migration |
| `internal/network/agent/healthcheck.go` | Worker-side TCP/HTTP health check loop + gRPC reporter |
| `internal/network/agent/l4lb_test.go` | Unit tests for L4 LB OVS pipeline (group creation, affinity, backend exclusion, backward compat) |
| `internal/network/l4lb_test.go` | Unit tests for L4 LB data model, config serialization, validation logic |
| `docs/sprint-logs/S043/test-output.log` | Full test run output |
| `docs/sprint-logs/S043/implementation-notes.md` | This file |

## Files Modified

| File | Changes |
|------|---------|
| `internal/network/models.go` | Added `IngressTypeL4LB` constant; added `L4LBBackend`, `L4LBHealthCheck`, `L4LBConfig` structs; extended `Ingress` and `IngressSpec` with `L4LBConfig *L4LBConfig` field |
| `internal/network/service.go` | Added `UpdateBackendHealth(ctx, ingressID, vmID, healthy) error` to `Service` interface |
| `internal/network/store.go` | Extended `CreateIngress` to accept `l4_lb` type with full validation; added `unmarshalIngressConfig` helper; added `UpdateBackendHealth` implementation; updated `GetIngress`/`ListIngresses` to use the helper |
| `internal/network/controller.go` | Extended `computeIngressRules` to query both `direct_ip` and `l4_lb` ingresses; joins with `l4lb_backend_health` for current health state; only sends healthy backends in `IngressRule` |
| `internal/network/agent/ovsclient.go` | Added `AddGroup`, `ModifyGroup`, `DeleteGroup` methods to `OVSClient` interface |
| `internal/network/agent/ovs_openflow.go` | Implemented `AddGroup`, `ModifyGroup`, `DeleteGroup` in `ExecOVSClient` using `ovs-ofctl -O OpenFlow13` |
| `internal/network/agent/ovs.go` | Added `lbGroups map[string]uint32` to `Pipeline`; extended `applyIngressRules` to handle `l4_lb` type; added `applyL4LBRule`, `buildL4LBGroupSpec`, `l4lbGroupID` helper functions |
| `internal/controller/grpc.go` | Added `networkSvc` and `networkStateSrv` fields to `GRPCServer`; added `NewGRPCServerWithNetwork` constructor; implemented `ReportBackendHealth` RPC handler |
| `test/mock/ovs/mock.go` | Added `groups map[uint32]string` field; implemented `AddGroup`, `ModifyGroup`, `DeleteGroup`, `GetGroups`, `parseGroupID` for mock OVS client |
| `proto/network.proto` | Added `L4LBBackend` message; extended `IngressRule` with `listener_port`, `protocol`, `backends`, `session_affinity` fields |
| `proto/agent.proto` | Added `ReportBackendHealth` RPC to `ControllerService`; added `BackendHealthStatus`, `ReportBackendHealthRequest`, `ReportBackendHealthResponse` messages |
| `proto/networkpb/network.pb.go` | Regenerated from proto (via `make proto`) |
| `proto/networkpb/network_grpc.pb.go` | Regenerated from proto (via `make proto`) |
| `proto/agentpb/agent.pb.go` | Regenerated from proto (via `make proto`) |
| `proto/agentpb/agent_grpc.pb.go` | Regenerated from proto (via `make proto`) |
| `cmd/cirrusctl/main.go` | Extended `ingress create` command to support `--type l4_lb` with `--backend`, `--port`, `--protocol`, `--session-affinity`, `--health-check-*` flags; updated `ingress list` to display L4 LB-specific detail |

## Design Decisions

1. **l4_lb config storage**: The full `L4LBConfig` is stored in the existing `ingresses.config` JSONB column under the key `"l4lb"`. This avoids schema changes to the `ingresses` table while keeping backward compatibility for `direct_ip`.

2. **Backend health tracking**: A separate `l4lb_backend_health` table (`ingress_id, vm_id, healthy, last_checked_at`) tracks real-time health. The `computeIngressRules` joins against this table and only sends healthy backends to the OVS pipeline. This means the OVS group is always updated with only healthy backends — no flow counters or stateful round-robin.

3. **OVS group ID derivation**: FNV-1a hash of the ingress UUID string → uint32, clamped away from 0. Deterministic and stable across restarts.

4. **Group add vs. modify**: `Pipeline` tracks installed group IDs in `lbGroups map[string]uint32`. On first apply: `add-group`. On subsequent applies (backend list changed): `mod-group`. Stale groups (ingress removed) are deleted.

5. **Health check probe**: `HealthChecker` in `internal/network/agent/healthcheck.go` probes only backends whose VM ID appears in the local port list (i.e., running on this host). TCP probe only for now; HTTP probing infrastructure is included (`probeHTTP`) but not wired to the rule's health_check config yet (proto field not yet delivered by StateController — the health check config stays in the DB JSONB).

6. **`ReportBackendHealth` broadcast**: After updating backend health in DB, the controller currently logs that gateway hosts will pick up the change on next poll (2-second interval). A targeted `TriggerRefresh` per gateway host was not implemented to avoid requiring the gRPC handler to query which gateway node serves the ingress — this is a known limitation.

7. **`NewGRPCServerWithNetwork`**: A new constructor was added rather than modifying the existing `NewGRPCServer` signature to preserve backward compatibility with all existing call sites.

## Known Limitations

- Health check config (`interval_sec`, `timeout_sec`, etc.) from the DB JSONB is not currently forwarded to the worker via proto — the health checker uses a fixed 10-second interval and 3-second timeout.
- HTTP health probing is implemented (`probeHTTP`) but not yet used; the probe always falls back to TCP.
- `triggerRefreshForAffectedHosts` is a no-op logging stub — the 2-second poll interval is relied upon for convergence instead of targeted push.
- Session affinity `selection_method=ip_src` requires OVS support for the `selection_method` group field (OpenFlow 1.3+). This is set with `-O OpenFlow13` flags in `ExecOVSClient`.
