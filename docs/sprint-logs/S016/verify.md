# S016 Sprint Verify Log

Date: 2026-04-02

## Phase 1: Completeness Check

All tasks in S016 confirmed implemented:
- [x] VMStatus enum + state machine helpers (models.go)
- [x] Service interface extended: StartVM/StopVM/ForceStopVM/RebootVM/RepairVM
- [x] Orchestrator: StartVM/StopVM/ForceStopVM/RebootVM/RepairVM
- [x] Orchestrator: DeleteVM guards (IsTransitional + CanDelete)
- [x] teardownVM: correct cleanup order (DestroyVM→Detach→UndefineVM→DeletePort→Unexport→Delete)
- [x] proto: StartVM/StopVM/ForceStopVM/RebootVM/GetVMState RPCs
- [x] WorkerServer handlers for all 5 new RPCs
- [x] WorkerClient wrapper methods
- [x] network.Service: GetPortByVMID + DeletePort
- [x] API: POST /vms/{id}/actions + POST /admin/vms/{id}/repair
- [x] RBAC: ActionForceStopVM + ActionRepairVM
- [x] CLI: vm start/stop/force-stop/reboot + admin vm repair
- [x] client.VMAction + client.RepairVM
- [x] Makefile seed: StorageBackend + VolumeType + host-sd association
- [x] Unit tests: models_test.go
- [x] Integration tests: vm_lifecycle_test.go

## Phase 2: Code Review Findings & Fixes

### Fixed
1. `ErrConflict` defined with `fmt.Errorf` — changed to `errors.New` (models.go)
   - Reason: `fmt.Errorf` without `%w` breaks `errors.Is` sentinel pattern
2. `resolveWorker` returned `(*host.Host, *WorkerClient, string, error)` — reduced to `(*WorkerClient, string, error)`
   - Updated all 4 callers (StartVM, StopVM, ForceStopVM, RebootVM)
   - Removed unused `host` package import from orchestrator.go
3. `teardownVM` called `listVMVolumeIDs` twice — collapsed to single call at top
4. `teardownVM` called `ExportVolume` to get disk specs for worker — replaced with `GetVolume` + JSON unmarshal of `vol.ExportInfo`
   - Reason: `ExportVolume` returns `ErrVolumeInUse` on already-exported volumes; would silently omit disk specs → worker wouldn't detach
5. Removed numbered "Step 1/2/3" comments from worker_server.go DeleteVM (narrating comments)

### Not Fixed (false positives)
- Double switch in vm_handler.go `vmAction`: acceptable given the auth/dispatch separation; would require a larger refactor for minimal gain

## Phase 3: Build & Test

```
go build ./...   → PASS
go test ./...    → all PASS (compute: 0.004s, api: 0.007s, all others cached)
```
