package quota

import (
	"errors"
	"testing"
)

func TestCheckAgainst_Unlimited(t *testing.T) {
	// All limits zero → unlimited, nothing should be exceeded.
	if err := checkAgainst(9999, 9999, 9999, 9999, 9999, 9999, 9999, Limits{}); err != nil {
		t.Fatalf("expected no error for unlimited limits, got %v", err)
	}
}

func TestCheckAgainst_AtLimit(t *testing.T) {
	l := Limits{Vcpus: 4, RAMMB: 8192, VolumeGB: 100, VMs: 5, Volumes: 10, Snapshots: 20, Networks: 3}
	// Exactly at limit should be allowed.
	if err := checkAgainst(4, 8192, 100, 5, 10, 20, 3, l); err != nil {
		t.Fatalf("expected no error at limit, got %v", err)
	}
}

func TestCheckAgainst_Exceeded(t *testing.T) {
	cases := []struct {
		name    string
		vcpus   int
		ramMB   int
		volGB   int
		vms     int
		volumes int
		snaps   int
		nets    int
		limits  Limits
	}{
		{"vcpus", 5, 0, 0, 0, 0, 0, 0, Limits{Vcpus: 4}},
		{"ram_mb", 0, 9000, 0, 0, 0, 0, 0, Limits{RAMMB: 8192}},
		{"volume_gb", 0, 0, 101, 0, 0, 0, 0, Limits{VolumeGB: 100}},
		{"vms", 0, 0, 0, 6, 0, 0, 0, Limits{VMs: 5}},
		{"volumes", 0, 0, 0, 0, 11, 0, 0, Limits{Volumes: 10}},
		{"snapshots", 0, 0, 0, 0, 0, 21, 0, Limits{Snapshots: 20}},
		{"networks", 0, 0, 0, 0, 0, 0, 4, Limits{Networks: 3}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkAgainst(tc.vcpus, tc.ramMB, tc.volGB, tc.vms, tc.volumes, tc.snaps, tc.nets, tc.limits)
			if err == nil {
				t.Fatal("expected ErrQuotaExceeded, got nil")
			}
			if !isQuotaExceeded(err) {
				t.Fatalf("expected ErrQuotaExceeded, got %v", err)
			}
		})
	}
}

func isQuotaExceeded(err error) bool {
	return errors.Is(err, ErrQuotaExceeded)
}
