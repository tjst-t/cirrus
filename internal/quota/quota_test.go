package quota

import (
	"errors"
	"testing"
)

func TestCheckAgainst_Unlimited(t *testing.T) {
	// All limits zero → unlimited, nothing should be exceeded.
	if err := checkAgainst(9999, 9999, 9999, 9999, 9999, 9999, 9999, 9999, 9999, Limits{}, ResourceDelta{}); err != nil {
		t.Fatalf("expected no error for unlimited limits, got %v", err)
	}
}

func TestCheckAgainst_AtLimit(t *testing.T) {
	l := Limits{Vcpus: 4, RAMMB: 8192, VolumeGB: 100, VMs: 5, Volumes: 10, Snapshots: 20, Networks: 3, Egresses: 5, Ingresses: 5}
	// Exactly at limit should be allowed.
	if err := checkAgainst(4, 8192, 100, 5, 10, 20, 3, 5, 5, l, ResourceDelta{}); err != nil {
		t.Fatalf("expected no error at limit, got %v", err)
	}
}

func TestCheckAgainst_Exceeded(t *testing.T) {
	cases := []struct {
		name             string
		vcpus            int
		ramMB            int
		volGB            int
		vms              int
		volumes          int
		snaps            int
		nets             int
		egresses         int
		ingresses        int
		limits           Limits
		wantResource     string
		wantLimit        int
	}{
		{"vcpus", 5, 0, 0, 0, 0, 0, 0, 0, 0, Limits{Vcpus: 4}, "vcpu", 4},
		{"ram_mb", 0, 9000, 0, 0, 0, 0, 0, 0, 0, Limits{RAMMB: 8192}, "memory_mb", 8192},
		{"volume_gb", 0, 0, 101, 0, 0, 0, 0, 0, 0, Limits{VolumeGB: 100}, "volume_gb", 100},
		{"vms", 0, 0, 0, 6, 0, 0, 0, 0, 0, Limits{VMs: 5}, "vm_count", 5},
		{"volumes", 0, 0, 0, 0, 11, 0, 0, 0, 0, Limits{Volumes: 10}, "volume_count", 10},
		{"snapshots", 0, 0, 0, 0, 0, 21, 0, 0, 0, Limits{Snapshots: 20}, "snapshot_count", 20},
		{"networks", 0, 0, 0, 0, 0, 0, 4, 0, 0, Limits{Networks: 3}, "network_count", 3},
		{"egresses", 0, 0, 0, 0, 0, 0, 0, 6, 0, Limits{Egresses: 5}, "egress_count", 5},
		{"ingresses", 0, 0, 0, 0, 0, 0, 0, 0, 6, Limits{Ingresses: 5}, "ingress_count", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkAgainst(tc.vcpus, tc.ramMB, tc.volGB, tc.vms, tc.volumes, tc.snaps, tc.nets, tc.egresses, tc.ingresses, tc.limits, ResourceDelta{})
			if err == nil {
				t.Fatal("expected ErrQuotaExceeded, got nil")
			}
			if !isQuotaExceeded(err) {
				t.Fatalf("expected ErrQuotaExceeded, got %v", err)
			}
			var v *ViolationError
			if !errors.As(err, &v) {
				t.Fatal("expected *ViolationError")
			}
			if v.Resource != tc.wantResource {
				t.Errorf("Resource = %q, want %q", v.Resource, tc.wantResource)
			}
			if v.Limit != tc.wantLimit {
				t.Errorf("Limit = %d, want %d", v.Limit, tc.wantLimit)
			}
			if v.Current < 0 {
				t.Errorf("Current = %d, want >= 0", v.Current)
			}
		})
	}
}

func isQuotaExceeded(err error) bool {
	return errors.Is(err, ErrQuotaExceeded)
}
