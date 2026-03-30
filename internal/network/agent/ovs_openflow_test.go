package agent

import (
	"testing"
)

func TestParseFlowLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    FlowEntry
		wantErr bool
	}{
		{
			name: "basic flow with metadata",
			line: " cookie=0x0, duration=123.456s, table=0, n_packets=100, n_bytes=5000, priority=100,ip,in_port=1 actions=resubmit(,1)",
			want: FlowEntry{
				Table:    0,
				Priority: 100,
				Match:    "ip,in_port=1",
				Actions:  "resubmit(,1)",
			},
		},
		{
			name: "drop action",
			line: " cookie=0x0, duration=10.0s, table=3, n_packets=0, n_bytes=0, priority=0 actions=drop",
			want: FlowEntry{
				Table:    3,
				Priority: 0,
				Match:    "",
				Actions:  "drop",
			},
		},
		{
			name: "complex match",
			line: " cookie=0x0, duration=5.0s, table=0, n_packets=0, n_bytes=0, priority=200,udp,in_port=1,tp_dst=67 actions=LOCAL",
			want: FlowEntry{
				Table:    0,
				Priority: 200,
				Match:    "udp,in_port=1,tp_dst=67",
				Actions:  "LOCAL",
			},
		},
		{
			name: "load and resubmit actions",
			line: " cookie=0x0, duration=1.0s, table=0, n_packets=0, n_bytes=0, priority=100,ip,dl_src=02:aa:bb:cc:dd:ee,nw_src=10.0.0.1,in_port=5 actions=load:0x12345678->NXM_NX_REG1[],resubmit(,1)",
			want: FlowEntry{
				Table:    0,
				Priority: 100,
				Match:    "ip,dl_src=02:aa:bb:cc:dd:ee,nw_src=10.0.0.1,in_port=5",
				Actions:  "load:0x12345678->NXM_NX_REG1[],resubmit(,1)",
			},
		},
		{
			name: "conntrack action",
			line: " cookie=0x0, duration=1.0s, table=1, n_packets=0, n_bytes=0, priority=100,ct_state=+new+trk,ip actions=resubmit(,2)",
			want: FlowEntry{
				Table:    1,
				Priority: 100,
				Match:    "ct_state=+new+trk,ip",
				Actions:  "resubmit(,2)",
			},
		},
		{
			name: "idle_age field",
			line: " cookie=0x0, duration=300.0s, table=6, n_packets=50, n_bytes=2500, idle_age=10, priority=100,ip,nw_dst=10.0.0.1 actions=output:5",
			want: FlowEntry{
				Table:    6,
				Priority: 100,
				Match:    "ip,nw_dst=10.0.0.1",
				Actions:  "output:5",
			},
		},
		{
			name:    "no actions",
			line:    " cookie=0x0, table=0, priority=100,ip",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlowLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Table != tt.want.Table {
				t.Errorf("table: got %d, want %d", got.Table, tt.want.Table)
			}
			if got.Priority != tt.want.Priority {
				t.Errorf("priority: got %d, want %d", got.Priority, tt.want.Priority)
			}
			if got.Match != tt.want.Match {
				t.Errorf("match: got %q, want %q", got.Match, tt.want.Match)
			}
			if got.Actions != tt.want.Actions {
				t.Errorf("actions: got %q, want %q", got.Actions, tt.want.Actions)
			}
		})
	}
}
