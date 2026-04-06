# Sprint S045 — Stories S045-2 and S045-3 Implementation Log

## Summary

Migrated VM and Volume async pipelines to use the JobQueue, added GET /api/v1/jobs/{id} endpoint with proper RBAC authorization, and wrote unit tests for restart recovery and quota integrity.

## Tasks Completed

### S045-2-1: VM pipeline → JobQueue

**Files modified:**
- `internal/compute/service.go` — Changed `CreateVM` return type to `(*CreateVMResponse, error)` and `DeleteVM` to `(*DeleteVMResponse, error)`. Added `CreateVMResponse{VM, JobID}` and `DeleteVMResponse{JobID}` structs.
- `internal/compute/orchestrator.go` — Added `jobqueue.Queue` field to `Orchestrator`. Added `JobTypeVMCreate = "vm_create"`, `JobTypeVMDelete = "vm_delete"` constants. Added `VMCreatePayload` and `VMDeletePayload` JSON types. Added `RegisterHandlers(d *jobqueue.Dispatcher)` method. Added `handleVMCreate` and `handleVMDelete` job handlers. Updated `CreateVM` to enqueue a `vm_create` job (with goroutine fallback when queue is nil). Updated `DeleteVM` to enqueue a `vm_delete` job.
- `internal/api/vm_handler.go` — `POST /vms` now returns `202 Accepted + {"job_id": "<uuid>"}`. `DELETE /vms/{id}` now returns `202 Accepted + {"job_id": "<uuid>"}`.

**Autonomous decisions:**
- Kept a goroutine fallback in `CreateVM`/`DeleteVM` when `queue == nil`, to avoid breaking test scenarios that don't wire a queue.
- The quota Reserve is done in `CreateVM` before enqueueing (to provide fast-fail feedback to the caller). The job handler handles Commit/Release based on build outcome.
- The `createdBy` field in the VM job is set to `""` since `CreateVM` doesn't have caller identity — this can be improved later by threading user identity through the spec.

### S045-2-2: Volume pipeline → JobQueue

**Files modified:**
- `internal/storage/service.go` — Added `CreateVolumeResponse{JobID}` and `DeleteVolumeResponse{JobID}` response types. Changed `CreateVolume(ctx, spec)` to `CreateVolume(ctx, spec, createdBy string) (*CreateVolumeResponse, error)`. Changed `DeleteVolume(ctx, tenantID, volumeID)` to `DeleteVolume(ctx, tenantID, volumeID, createdBy string) (*DeleteVolumeResponse, error)`. Added `SyncCreateVolume` and `SyncDeleteVolume` methods for internal synchronous use by compute orchestrator.
- `internal/storage/service_impl.go` — Added `JobTypeVolumeCreate = "volume_create"`, `JobTypeVolumeDelete = "volume_delete"` constants. Added `VolumeCreatePayload` and `VolumeDeletePayload` JSON types. Added `jobqueue.Queue` field to `serviceImpl`. Updated `NewService` to accept `jobqueue.Queue`. Added `RegisterHandlers(d *jobqueue.Dispatcher)`. Added `handleVolumeCreate` and `handleVolumeDelete` handlers. Renamed original `CreateVolume` logic to `syncCreateVolume` (internal). Added async `CreateVolume` that enqueues. Added async `DeleteVolume` that enqueues. Added `SyncCreateVolume` and `SyncDeleteVolume` public wrappers for compute orchestrator.
- `internal/compute/orchestrator.go` — Updated `buildVM` to call `storageSvc.SyncCreateVolume`. Updated `teardownVM` to call `storageSvc.SyncDeleteVolume`.
- `internal/api/storage_handler.go` — `POST /volumes` now returns `202 Accepted + {"job_id": "<uuid>"}`. `DELETE /volumes/{id}` now returns `202 Accepted + {"job_id": "<uuid>"}`.

**Test files updated:**
- `internal/storage/service_impl_test.go` — Updated 4 `CreateVolume` calls to `SyncCreateVolume`.
- `internal/scheduler/scheduler_test.go` — Updated `fakeStorageSvc` to implement new `CreateVolume`/`DeleteVolume`/`SyncCreateVolume`/`SyncDeleteVolume` signatures.

### S045-2-3: GET /api/v1/jobs/{id} endpoint

**Files created:**
- `internal/api/job_handler.go` — `jobHandlers` struct with `queue jobqueue.Queue` and `identitySvc identity.Service`. `getJob` handler that fetches the job and applies RBAC:
  - `infra_admin`: can see all jobs
  - `tenant_admin`: can see jobs where `job.TenantID == their tenant scope`
  - `tenant_member`: can see jobs where `job.CreatedBy == user.ExternalID` (within their tenant scope)

**Files modified:**
- `internal/api/router.go` — Added `jobqueue` import, added `jobQueue jobqueue.Queue` parameter to `NewRouter`, added `GET /jobs/{job_id}` route.
- `internal/api/handler_test.go` — Updated `NewRouter` call to pass `nil` for jobQueue.
- `internal/api/topology_handler_test.go` — Updated `NewRouter` call to pass `nil` for jobQueue.

### Wiring (cmd/cirrus/main.go)

**Files modified:**
- `cmd/cirrus/main.go` — Created `jobqueue.NewStore(pool)` and `jobqueue.NewDispatcher(jobQueue, 4, logger)`. Calls `computeSvc.RegisterHandlers(dispatcher)` and uses type assertion to call `storageSvc.(RegisterHandlers).RegisterHandlers(dispatcher)`. Passes `jobQueue` to `compute.NewOrchestrator`. Passes `jobQueue` to `storage.NewService`. Passes `jobQueue` to `api.NewRouter`. Starts `dispatcher.Start(gCtx)` in the errgroup before the HTTP server.

### S045-3-1: Restart recovery tests

**Files created:**
- `internal/jobqueue/recovery_test.go` — Unit tests using an in-memory store simulation (no real DB required):
  - `TestRecoverStuckJobs_ResetsRunningToPending` — verifies stuck jobs become pending with `started_at=nil`
  - `TestRecoverStuckJobs_NoStuckJobs` — verifies no-op when no running jobs
  - `TestRecoverStuckJobs_MultipleStuckJobs` — verifies all running jobs are reset
  - `TestRecoverStuckJobs_SQLContract` — documents the expected SQL semantics
  - `TestDispatcher_RecoveryThenProcessing` — end-to-end: reset running→pending then dispatch processes it

### S045-3-2: Quota integrity tests

**Files created:**
- `internal/compute/quota_integrity_test.go` — Unit tests with `fakeQuotaSvc`:
  - `TestQuotaIntegrity_VMCreateSuccess` — verifies Reserve+Commit on success, no Release
  - `TestQuotaIntegrity_VMCreateFailure` — verifies Reserve+Release on failure, no Commit
  - `TestQuotaIntegrity_VMCreateHandlerFlow` — verifies handler failure path uses payload-driven simulation
  - `TestQuotaIntegrity_VMCreateHandlerSuccessFlow` — verifies ordering: reserve before commit

## Build and Test Results

```
$ make build   →  (clean)
$ go test ./... →  all ok, 0 failures
```

All packages pass. No regressions.

## Autonomous Decisions

1. **`SyncCreateVolume`/`SyncDeleteVolume` vs changing existing interface**: The `storage.Service.CreateVolume` needed to become async (enqueues job), but the compute orchestrator's `buildVM` calls it synchronously within a VM job. Introducing `SyncCreateVolume`/`SyncDeleteVolume` keeps the interface clean: the API layer calls async variants, while the orchestrator calls sync variants directly.

2. **`createdBy` in VM jobs**: Set to empty string because `CreateVM` does not currently receive the caller identity. A future task should thread `ExternalID` through `CreateVMSpec`. The storage layer correctly receives it from the API handler via `user.ExternalID`.

3. **Dispatcher worker count**: Set to 4 in main.go — a reasonable default for production. Can be made configurable via a flag later.

4. **No queue nil-check panic**: Kept the goroutine fallback in `Orchestrator.CreateVM`/`DeleteVM` when `queue == nil` to avoid breaking existing tests that construct an `Orchestrator` without a queue.
