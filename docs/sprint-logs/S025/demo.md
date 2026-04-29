# Sprint S025 Demo Log

**Date:** 2026-04-29

## Demo intent

Per `sprint-runner` spec, demo Sprint S025's DRS feature on a running program (`make serve` → trigger DRS via `cirrusctl admin drs run`).

## Live `make serve` blocked — environmental conflict

`make serve` failed because **port 8243** (cirrus's `sim-common`, allocated by portman) is already bound by an unrelated `palmux` process (PID 883055) belonging to the `tjst-t/palmux2` project at `/home/ubuntu/ghq/github.com/tjst-t/palmux2`. That process is owned by another developer session on this machine; killing it would interfere with another project's work, which violates safe-action principles.

`portman release` was used to free cirrus's leases; on re-allocation portman returned the same port range and the conflict persisted. `make fresh` (which wipes Postgres data dir and restarts) reproduced the same conflict.

## What was demonstrated instead

### 1. Controller binary exposes new DRS flags

```
$ ./bin/cirrus controller --help | grep drs
      --drs-enabled                Enable Distributed Resource Scheduler (DRS) for automatic VM rebalancing
      --drs-interval int           DRS evaluation interval in seconds (default 300)
      --drs-max-concurrent int     Maximum number of DRS-triggered migrations per cycle (default 2)
      --drs-stddev-threshold float Std-dev of free-fraction across hosts that triggers DRS rebalancing (default 0.15)
```

### 2. cirrusctl admin drs subcommand wired

```
$ ./bin/cirrusctl admin drs --help
Manage DRS (Distributed Resource Scheduler)
Available Commands:
  run         Trigger a DRS cycle immediately (admin only)
  status      Show current DRS configuration and last run report
```

Both `run` and `status` honor the global `--output {table,json}` and `--token` flags.

### 3. Acceptance tests proxy for live demo

The acceptance tests exercise the same code paths the live demo would have:

**S025-1 — DRS redistributes load**
```
=== RUN   TestAC_S025_1_DRS_RedistributesLoad
    drs_acceptance_test.go:151: after 2 moves: fracA=0.688, fracB=0.812, stddev=0.062
--- PASS: TestAC_S025_1_DRS_RedistributesLoad (0.00s)
```
Initial imbalanced placement → DRS plan → 2 migrations executed → final stddev (0.062) below threshold (0.15).

**S025-2 — DRS admin endpoints**
```
=== RUN   TestAC_S025_2_DRSAdminEndpoints
    --- PASS: run_returns_200_with_report (0.00s)
    --- PASS: status_returns_enabled_and_interval (0.00s)
    --- PASS: status_returns_null_last_report_before_any_run (0.00s)
    --- PASS: run_returns_409_when_already_in_progress (0.00s)
--- PASS: TestAC_S025_2_DRSAdminEndpoints (0.00s)
```
Real HTTP requests through the chi router with auth middleware, verifying the JSON contract and the in-progress 409 path.

## Remaining gap

A full live demo (controller running, real worker registration, scheduled DRS cycle moving a sim VM between hosts) was not performed. The Story-level integration is verified by acceptance tests, but the operator-facing `cirrusctl admin drs run` command was not exercised against a live API server.

User decision: proceed to `sprint done`, or address the port conflict (e.g., reconfigure portman's allocation range to avoid 8243) before final commit.
