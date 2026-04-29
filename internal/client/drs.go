package client

import "context"

// DRSRunReport is the JSON shape returned by POST /api/v1/admin/drs/run.
type DRSRunReport struct {
	StartedAt  string        `json:"started_at"`
	FinishedAt string        `json:"finished_at"`
	DurationMs int64         `json:"duration_ms"`
	AZResults  []DRSAZResult `json:"az_results"`
	Successes  int           `json:"successes"`
	Failures   int           `json:"failures"`
	Errors     []string      `json:"errors"`
}

// DRSAZResult is the per-AZ summary returned in a DRS run report.
type DRSAZResult struct {
	AZID           string  `json:"az_id"`
	StddevBefore   float64 `json:"stddev_before"`
	PlannedCount   int     `json:"planned_count"`
	EvaluatedHosts int     `json:"evaluated_hosts"`
}

// DRSStatus is the JSON shape returned by GET /api/v1/admin/drs/status.
type DRSStatus struct {
	Enabled         bool          `json:"enabled"`
	IntervalSeconds int           `json:"interval_seconds"`
	LastReport      *DRSRunReport `json:"last_report"`
}

// DRSRun triggers a manual DRS cycle (POST /api/v1/admin/drs/run).
func (c *Client) DRSRun(ctx context.Context) (*DRSRunReport, error) {
	resp, err := c.do(ctx, "POST", "/api/v1/admin/drs/run", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*DRSRunReport](resp)
}

// DRSStatus returns the current DRS status and last run report (GET /api/v1/admin/drs/status).
func (c *Client) DRSStatus(ctx context.Context) (*DRSStatus, error) {
	resp, err := c.do(ctx, "GET", "/api/v1/admin/drs/status", nil)
	if err != nil {
		return nil, err
	}
	return decodeResponse[*DRSStatus](resp)
}
