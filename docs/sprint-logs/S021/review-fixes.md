# S021 Sprint Review Fixes

## Reuse findings
1. **validateName unified with validate.Name** — `internal/api/validate.go` was using permissive rules (64 chars, any case). Now delegates to `validate.Name` (lowercase alphanumeric + hyphens, max 63 chars) for consistency with existing handlers.
2. **parseUUID removed** — was defined but unused; all handlers still use `uuid.Parse` directly.
3. **cursorValues helper added** — `internal/api/pagination.go`: replaces the `if cursor == nil { zeroTime, uuid.Nil } else { cursor.CreatedAt, cursor.ID }` pattern repeated in every handler.

## Quality findings
4. **vol != nil guard removed** — `internal/compute/orchestrator.go:buildVM`: `vol != nil` in defer was always true at that point; removed.
5. **errors.Join in teardownVM** — replaced `errs[0]` with `errors.Join(errs...)` so all step errors are preserved in the returned error.
6. **OVSFlowVerifier comment** — removed impl-detail ("cirrus-sim does not run OVS"); now describes the interface abstractly.
7. **Blank line removed** — `internal/api/flavor_handler.go` extra blank line after import block.
8. **_ = uuid.Nil removed** — `test/integration/e2e_multitenant_test.go`: removed unused import workaround.

## Efficiency findings
9. **Pagination indexes migration added** — `000022_pagination_indexes.up.sql`: added `(created_at, id)` composite indexes for hosts, flavors, organizations; `(organization_id, created_at, id)` for tenants; `(tenant_id, created_at, id)` for networks, volumes, vms.
