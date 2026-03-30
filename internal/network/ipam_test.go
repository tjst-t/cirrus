package network

import (
	"net"
	"strings"
	"testing"
)

func TestAllocateBlock_FirstBlock(t *testing.T) {
	vmIP, gwIP, err := AllocateBlock("100.64.0.0/22", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vmIP != "100.64.0.1" {
		t.Errorf("vmIP = %s, want 100.64.0.1", vmIP)
	}
	if gwIP != "100.64.0.2" {
		t.Errorf("gwIP = %s, want 100.64.0.2", gwIP)
	}
}

func TestAllocateBlock_Sequential(t *testing.T) {
	// First block already used (.1 is the VM IP)
	vmIP, gwIP, err := AllocateBlock("100.64.0.0/22", []string{"100.64.0.1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vmIP != "100.64.0.5" {
		t.Errorf("vmIP = %s, want 100.64.0.5", vmIP)
	}
	if gwIP != "100.64.0.6" {
		t.Errorf("gwIP = %s, want 100.64.0.6", gwIP)
	}
}

func TestAllocateBlock_MultipleExisting(t *testing.T) {
	// Blocks 0,1,2 used
	existing := []string{"100.64.0.1", "100.64.0.5", "100.64.0.9"}
	vmIP, gwIP, err := AllocateBlock("100.64.0.0/22", existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vmIP != "100.64.0.13" {
		t.Errorf("vmIP = %s, want 100.64.0.13", vmIP)
	}
	if gwIP != "100.64.0.14" {
		t.Errorf("gwIP = %s, want 100.64.0.14", gwIP)
	}
}

func TestAllocateBlock_CIDRExhausted(t *testing.T) {
	// /30 = exactly 1 block, already used
	_, _, err := AllocateBlock("100.64.0.0/30", []string{"100.64.0.1"})
	if err != ErrCIDRExhausted {
		t.Errorf("err = %v, want ErrCIDRExhausted", err)
	}
}

func TestAllocateBlock_InvalidCIDR(t *testing.T) {
	_, _, err := AllocateBlock("invalid", nil)
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestGenerateMAC_Format(t *testing.T) {
	mac, err := GenerateMAC()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(mac, "02:") {
		t.Errorf("MAC should start with 02:, got %s", mac)
	}
	// Verify it's a valid MAC
	_, err = net.ParseMAC(mac)
	if err != nil {
		t.Errorf("invalid MAC address %s: %v", mac, err)
	}
}

func TestGenerateMAC_Unique(t *testing.T) {
	macs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		mac, err := GenerateMAC()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if macs[mac] {
			t.Fatalf("duplicate MAC generated: %s", mac)
		}
		macs[mac] = true
	}
}

func TestAssignCIDR_First(t *testing.T) {
	cidr, err := AssignCIDR(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "100.64.0.0/22" {
		t.Errorf("cidr = %s, want 100.64.0.0/22", cidr)
	}
}

func TestAssignCIDR_SkipsExisting(t *testing.T) {
	cidr, err := AssignCIDR([]string{"100.64.0.0/22"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "100.64.4.0/22" {
		t.Errorf("cidr = %s, want 100.64.4.0/22", cidr)
	}
}

func TestAssignCIDR_SkipsMultiple(t *testing.T) {
	existing := []string{"100.64.0.0/22", "100.64.4.0/22", "100.64.8.0/22"}
	cidr, err := AssignCIDR(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "100.64.12.0/22" {
		t.Errorf("cidr = %s, want 100.64.12.0/22", cidr)
	}
}

func TestAssignCIDR_SkipsOverlappingSmallerBlock(t *testing.T) {
	// An existing /24 within the first /22 range should cause it to be skipped
	existing := []string{"100.64.1.0/24"}
	cidr, err := AssignCIDR(existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cidr != "100.64.4.0/22" {
		t.Errorf("cidr = %s, want 100.64.4.0/22", cidr)
	}
}
