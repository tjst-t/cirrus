package reconcile

import (
	"testing"
)

func TestStatusesMatch(t *testing.T) {
	tests := []struct {
		dbStatus string
		hbStatus string
		want     bool
	}{
		{"running", "running", true},
		{"running", "shutoff", false},
		{"running", "crashed", false},
		{"stopped", "shutoff", true},
		{"stopped", "running", false},
		{"error", "crashed", true},
		{"error", "shutoff", true},
		{"error", "running", false},
	}

	for _, tc := range tests {
		got := statusesMatch(tc.dbStatus, tc.hbStatus)
		if got != tc.want {
			t.Errorf("statusesMatch(%q, %q) = %v, want %v",
				tc.dbStatus, tc.hbStatus, got, tc.want)
		}
	}
}

func TestClassifyMismatch(t *testing.T) {
	tests := []struct {
		dbStatus string
		hbStatus string
		wantSev  string
	}{
		{"running", "crashed", DriftSeverityCritical},
		{"running", "shutoff", DriftSeverityMedium},
		{"stopped", "running", DriftSeverityHigh},
		{"error", "running", DriftSeverityMedium},
	}

	for _, tc := range tests {
		sev := classifyMismatch(tc.dbStatus, tc.hbStatus)
		if sev != tc.wantSev {
			t.Errorf("classifyMismatch(%q, %q) = %q, want %q",
				tc.dbStatus, tc.hbStatus, sev, tc.wantSev)
		}
	}
}
