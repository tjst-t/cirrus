package network

import "testing"

func TestGatewayIPFromVM(t *testing.T) {
	tests := []struct {
		vmIP    string
		want    string
	}{
		{"100.64.0.1", "100.64.0.2"},
		{"100.64.0.5", "100.64.0.6"},
		{"100.64.0.9", "100.64.0.10"},
		{"100.64.4.1", "100.64.4.2"},
		{"100.64.255.253", "100.64.255.254"},
		{"", ""},
		{"invalid", ""},
	}

	for _, tt := range tests {
		got := gatewayIPFromVM(tt.vmIP)
		if got != tt.want {
			t.Errorf("gatewayIPFromVM(%q) = %q, want %q", tt.vmIP, got, tt.want)
		}
	}
}
