# S045-1 Implementation Log — Job Queue Database Foundation

## Date
2026-04-06

## Files Created

### Migration (Task S045-1-1)
- `internal/state/migrations/000023_jobs.up.sql` — creates `jobs` table with all required columns, `idx_jobs_status_created_at` and `idx_jobs_tenant_created_by` indexes
- `internal/state/migrations/000023_jobs.down.sql` — drops the `jobs` table

### Package `internal/jobqueue/` (Task S045-1-2)
- `internal/jobqueue/jobqueue.go` — `Status` constants, `Job`, `EnqueueParams` types, and `Queue` interface
- `internal/jobqueue/store.go` — PostgreSQL implementation of `Queue` using `*pgxpool.Pool`; `Dequeue` uses `FOR UPDATE SKIP LOCKED`
- `internal/jobqueue/dispatcher.go` — `HandlerFunc` type, `Dispatcher` struct with `Register` / `Start`; N worker goroutines, graceful shutdown on ctx cancel
- `internal/jobqueue/recovery.go` — `RecoverStuckJobs(ctx, pool, logger)` function

### Controller Startup (Task S045-1-3)
- `cmd/cirrus/main.go` — added `internal/jobqueue` import and call to `jobqueue.RecoverStuckJobs` after migrations, before service wiring

### Tests
- `internal/jobqueue/dispatcher_test.go` — 4 table-driven unit tests using an in-memory `mockQueue`:
  - `TestDispatcher_Register_and_Start_processes_job` — happy path: job is dequeued, handler runs, Complete is called
  - `TestDispatcher_handler_error_marks_failed` — handler returns error → Fail is called
  - `TestDispatcher_no_handler_marks_failed` — nil handler → Fail is called with descriptive message
  - `TestDispatcher_graceful_shutdown` — ctx cancel causes all workers to exit cleanly

## Build Output
```
go build ./...  →  (no output, success)
```

## Test Output
```
ok  github.com/tjst-t/cirrus/internal/jobqueue   0.204s
(all other packages: ok or no test files)
```

## Autonomous Decisions
- Used `go test ./... 2>&1` full suite rather than just `jobqueue` package, to confirm no regressions.
- `ListStuck` passes the duration as a PostgreSQL interval string (`stuckAfter.String()`) — this works because Go's `time.Duration.String()` produces values like `"30m0s"` which PostgreSQL accepts as interval literals.
- `Dispatcher.Start` logs a warning and blocks on `<-ctx.Done()` if no handlers are registered, to avoid a goroutine no-op spin.
- `for i := range d.workerCount` (Go 1.22+ range-over-integer) used in dispatcher; confirmed compatible with go 1.25.
