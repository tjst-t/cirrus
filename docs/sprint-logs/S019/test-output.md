# S019 Test Output

## Unit Tests (quota package)

```
=== RUN   TestCheckAgainst_Unlimited
--- PASS: TestCheckAgainst_Unlimited (0.00s)
=== RUN   TestCheckAgainst_AtLimit
--- PASS: TestCheckAgainst_AtLimit (0.00s)
=== RUN   TestCheckAgainst_Exceeded
=== RUN   TestCheckAgainst_Exceeded/vcpus
=== RUN   TestCheckAgainst_Exceeded/ram_mb
=== RUN   TestCheckAgainst_Exceeded/volume_gb
=== RUN   TestCheckAgainst_Exceeded/vms
=== RUN   TestCheckAgainst_Exceeded/volumes
=== RUN   TestCheckAgainst_Exceeded/snapshots
=== RUN   TestCheckAgainst_Exceeded/networks
--- PASS: TestCheckAgainst_Exceeded (0.00s)
PASS
ok  github.com/tjst-t/cirrus/internal/quota 0.002s
```

## Full build (make build)

```
go build ./... — PASS (no errors)
```

## All unit tests

All packages: ok (no failures)

## Integration tests

Skipped (require running stack + CIRRUS_ENDPOINT env var).
Run with: go test -tags integration ./test/integration/ -run TestQuota
