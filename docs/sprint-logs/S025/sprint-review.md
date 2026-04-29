# Sprint S025 Sprint-Level Review

**Reviewed at:** 2026-04-29
**Diff scope:** commit 22b3274 → bd540bb (6 commits, +4459/-8, 23 files)

## Acceptance Criteria Traceability

| Story | Acceptance Criterion | Test | Status |
|---|---|---|---|
| S025-1 | AC-1: stddev computation | TestDRS_NoPlanWhenStddevBelowThreshold | PASS |
| S025-1 | AC-2: greedy picks most-loaded → least-loaded | TestDRS_GreedyPicksMostLoadedToLeastLoaded | PASS |
| S025-1 | AC-3: respects MaxConcurrent cap | TestDRS_RespectsMaxConcurrentCap | PASS |
| S025-1 | AC-4: skips !PhysicalKnown hosts | TestDRS_SkipsHostsWithoutPhysicalKnown | PASS |
| S025-1 | AC-5: greedy does not pick same VM twice | TestDRS_GreedyDoesNotPickSameVMTwice | PASS |
| S025-1 | AC-6: two AZs do not cross-pollinate | TestDRS_TwoAZsDoNotCrosspollinate | PASS |
| S025-1 | AC-7: imbalance reduces after plan | TestAC_S025_1_DRS_RedistributesLoad | PASS |
| S025-1 | AC-8: runner skips overlapping ticks | TestRunner_ConcurrentTicksDoNotOverlap | PASS |
| S025-1 | AC-9: runner failures don't abort cycle | TestRunner_FailureDoesNotAbortCycle | PASS |
| S025-2 | AC-1: POST /run returns 200 + report | TestDRSRun_Returns200WithReport | PASS |
| S025-2 | AC-2: POST /run returns 409 in-progress | TestDRSRun_Returns409WhenInProgress | PASS |
| S025-2 | AC-3: GET /status before any run | TestDRSStatus_Returns200WithNullLastReport | PASS |
| S025-2 | AC-4: admin auth required | TestDRSRun_Returns403ForNonAdmin / TestDRSStatus_Returns401WhenNoToken | PASS |
| S025-2 | AC-5: full round-trip | TestAC_S025_2_DRSAdminEndpoints | PASS |

## Build / Test / Vet

- `go build ./...` → success
- `go test ./...` → 31 packages PASS, 0 FAIL
- `go vet ./...` → 2 pre-existing warnings in `test/sim/libvirt/internal/state/db.go` (out of scope)
- `make lint` → golangci-lint not installed in this environment; go vet used as fallback

## Cross-Story Coherence

- AZ scoping: `Scheduler.CandidateHostsForAZ` introduced in S025-1, consumed by DRS engine. Handler in S025-2 does not bypass this.
- In-flight guard: shared `atomic.Int32` between ticker (Runner.Start goroutine) and admin handler (Runner.TryAcquire). No race.
- Leader-gating TODOs at `internal/controller/drs/runner.go:Start` and `cmd/cirrus/main.go` near `drsRunner.Start(gCtx)`.

## Deferred items (backlog candidates)

1. RunOnce: record failed cycles in lastReport so /status surfaces engine.Plan errors
2. Remove dead `Runner.IsRunning()` method (no callers after S025-2 review fixes)

## Verdict

Sprint S025 implementation is complete and acceptable.
