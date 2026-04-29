package api

// drs_handler.go — admin HTTP handlers for the DRS (Distributed Resource Scheduler).
//
// POST /api/v1/admin/drs/run   — triggers a DRS cycle synchronously.
// GET  /api/v1/admin/drs/status — returns last report + policy metadata.
//
// Both endpoints require the infra_admin role (enforced via identity.Authorizer).
//
// Note on synchronous execution:
//   A single DRS cycle is bounded in time (max_concurrent migrations, each
//   typically seconds).  For an admin-only endpoint, waiting inline is
//   acceptable — it gives the operator immediate feedback on what happened.
//   If the cycle is still running (triggered by the periodic ticker or another
//   concurrent /run call), the handler returns 409 Conflict instead of
//   blocking indefinitely.

import (
	"context"
	"net/http"

	controllerdrs "github.com/tjst-t/cirrus/internal/controller/drs"
	"github.com/tjst-t/cirrus/internal/apierror"
	"github.com/tjst-t/cirrus/internal/identity"
)

// DRSRunner is the subset of *controllerdrs.Runner used by the handler.
// Defined here (not in drs package) to avoid coupling the handler tightly to
// the concrete runner, and to keep the handler testable with a simple fake.
type DRSRunner interface {
	// TryAcquire atomically sets the in-flight flag; returns false if already set.
	TryAcquire() bool
	// Release clears the in-flight flag set by TryAcquire.
	Release()
	// RunOnce executes one DRS evaluation + migration cycle.
	RunOnce(ctx context.Context) (*controllerdrs.RunReport, error)
	// LastReport returns the most recently completed report, or nil.
	LastReport() *controllerdrs.RunReport
}

// drsAZResultJSON is the JSON shape for a per-AZ result in the run report.
type drsAZResultJSON struct {
	AZID           string  `json:"az_id"`
	StddevBefore   float64 `json:"stddev_before"`
	PlannedCount   int     `json:"planned_count"`
	EvaluatedHosts int     `json:"evaluated_hosts"`
}

// drsRunReportJSON is the stable JSON shape returned by both /run and /status.
type drsRunReportJSON struct {
	StartedAt  string            `json:"started_at"`
	FinishedAt string            `json:"finished_at"`
	DurationMs int64             `json:"duration_ms"`
	AZResults  []drsAZResultJSON `json:"az_results"`
	Successes  int               `json:"successes"`
	Failures   int               `json:"failures"`
	Errors     []string          `json:"errors"`
}

// drsStatusJSON is the response body for GET /status.
type drsStatusJSON struct {
	Enabled         bool              `json:"enabled"`
	IntervalSeconds int               `json:"interval_seconds"`
	LastReport      *drsRunReportJSON `json:"last_report"`
}

// drsHandlers holds dependencies for DRS admin endpoints.
type drsHandlers struct {
	runner          DRSRunner
	authz           identity.Authorizer
	drsEnabled      bool
	intervalSeconds int
}

// toRunReportJSON converts a controllerdrs.RunReport to the stable JSON shape.
func toRunReportJSON(r *controllerdrs.RunReport) *drsRunReportJSON {
	if r == nil {
		return nil
	}
	azJSON := make([]drsAZResultJSON, 0, len(r.AZResults))
	for _, az := range r.AZResults {
		azJSON = append(azJSON, drsAZResultJSON{
			AZID:           az.AZID.String(),
			StddevBefore:   az.StddevBefore,
			PlannedCount:   len(az.PlannedMoves),
			EvaluatedHosts: az.EvaluatedHosts,
		})
	}
	errs := r.Errors
	if errs == nil {
		errs = []string{}
	}
	return &drsRunReportJSON{
		StartedAt:  r.StartedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
		FinishedAt: r.FinishedAt.UTC().Format("2006-01-02T15:04:05.999999999Z"),
		DurationMs: r.FinishedAt.Sub(r.StartedAt).Milliseconds(),
		AZResults:  azJSON,
		Successes:  r.Successes,
		Failures:   r.Failures,
		Errors:     errs,
	}
}

// run handles POST /api/v1/admin/drs/run.
func (h *drsHandlers) run(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDRSRun, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	if h.runner == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, apierror.CodeInternal, "DRS runner not configured", nil)
		return
	}

	// Try to acquire the in-flight slot.  If it cannot be acquired the runner is
	// already executing (periodic tick or a concurrent /run call).
	if !h.runner.TryAcquire() {
		writeErrorCode(w, http.StatusConflict, apierror.CodeConflict, "drs run already in progress", nil)
		return
	}
	defer h.runner.Release()

	report, err := h.runner.RunOnce(r.Context())
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, apierror.CodeInternal, "DRS run failed: "+err.Error(), nil)
		return
	}

	writeJSON(w, http.StatusOK, toRunReportJSON(report))
}

// status handles GET /api/v1/admin/drs/status.
func (h *drsHandlers) status(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	decision, err := h.authz.Authorize(r.Context(), user, identity.ActionDRSStatus, identity.Resource{})
	if err != nil || decision == identity.Deny {
		writeErrorCode(w, http.StatusForbidden, apierror.CodeForbidden, "forbidden", nil)
		return
	}

	var lastReport *drsRunReportJSON
	if h.runner != nil {
		lastReport = toRunReportJSON(h.runner.LastReport())
	}

	writeJSON(w, http.StatusOK, drsStatusJSON{
		Enabled:         h.drsEnabled,
		IntervalSeconds: h.intervalSeconds,
		LastReport:      lastReport,
	})
}
