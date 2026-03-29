package eventlog

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	el := New()
	if el == nil {
		t.Fatal("New() returned nil")
	}
}

func TestRecord(t *testing.T) {
	ctx := context.Background()
	el := New()

	event := Event{
		Simulator:      "libvirt-sim",
		HostID:         "host-042",
		Operation:      "domain_create",
		RequestSummary: "vm-001, 4vcpu, 8192MB",
		Result:         "success",
		DurationMs:     12,
		FaultInjected:  false,
	}

	id := el.Record(ctx, event)
	if id == "" {
		t.Fatal("Record() returned empty ID")
	}

	events, total := el.Query(ctx, Filter{})
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if events[0].ID != id {
		t.Fatalf("expected ID %s, got %s", id, events[0].ID)
	}
	if events[0].Simulator != "libvirt-sim" {
		t.Fatalf("expected simulator libvirt-sim, got %s", events[0].Simulator)
	}
	if events[0].Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestQuery_Filters(t *testing.T) {
	ctx := context.Background()
	el := New()

	now := time.Now().UTC()

	events := []Event{
		{Simulator: "libvirt-sim", HostID: "host-001", Operation: "domain_create", Result: "success", DurationMs: 10},
		{Simulator: "libvirt-sim", HostID: "host-002", Operation: "domain_delete", Result: "success", DurationMs: 5},
		{Simulator: "storage-sim", HostID: "host-001", Operation: "volume_create", Result: "failure", DurationMs: 20},
		{Simulator: "storage-sim", HostID: "host-003", Operation: "volume_delete", Result: "success", DurationMs: 8},
	}
	for _, e := range events {
		el.Record(ctx, e)
	}

	tests := []struct {
		name          string
		filter        Filter
		expectedCount int
	}{
		{
			name:          "no filter returns all",
			filter:        Filter{},
			expectedCount: 4,
		},
		{
			name:          "filter by simulator",
			filter:        Filter{Simulator: "libvirt-sim"},
			expectedCount: 2,
		},
		{
			name:          "filter by host_id",
			filter:        Filter{HostID: "host-001"},
			expectedCount: 2,
		},
		{
			name:          "filter by operation",
			filter:        Filter{Operation: "domain_create"},
			expectedCount: 1,
		},
		{
			name:          "filter by simulator and host_id",
			filter:        Filter{Simulator: "storage-sim", HostID: "host-001"},
			expectedCount: 1,
		},
		{
			name:          "filter with no matches",
			filter:        Filter{Simulator: "nonexistent"},
			expectedCount: 0,
		},
		{
			name:          "filter by after timestamp",
			filter:        Filter{After: now.Add(-1 * time.Second)},
			expectedCount: 4,
		},
		{
			name:          "filter by future after timestamp",
			filter:        Filter{After: now.Add(1 * time.Hour)},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total := el.Query(ctx, tt.filter)
			if total != tt.expectedCount {
				t.Errorf("expected total %d, got %d", tt.expectedCount, total)
			}
			if len(results) != tt.expectedCount {
				t.Errorf("expected %d results, got %d", tt.expectedCount, len(results))
			}
		})
	}
}

func TestQuery_Pagination(t *testing.T) {
	ctx := context.Background()
	el := New()

	for i := 0; i < 25; i++ {
		el.Record(ctx, Event{
			Simulator: "libvirt-sim",
			HostID:    fmt.Sprintf("host-%03d", i),
			Operation: "domain_create",
			Result:    "success",
		})
	}

	tests := []struct {
		name           string
		filter         Filter
		expectedLen    int
		expectedTotal  int
	}{
		{
			name:          "default limit",
			filter:        Filter{},
			expectedLen:   25,
			expectedTotal: 25,
		},
		{
			name:          "limit 10",
			filter:        Filter{Limit: 10},
			expectedLen:   10,
			expectedTotal: 25,
		},
		{
			name:          "limit 10 offset 20",
			filter:        Filter{Limit: 10, Offset: 20},
			expectedLen:   5,
			expectedTotal: 25,
		},
		{
			name:          "offset beyond total",
			filter:        Filter{Limit: 10, Offset: 30},
			expectedLen:   0,
			expectedTotal: 25,
		},
		{
			name:          "offset 0 limit 5",
			filter:        Filter{Limit: 5, Offset: 0},
			expectedLen:   5,
			expectedTotal: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total := el.Query(ctx, tt.filter)
			if total != tt.expectedTotal {
				t.Errorf("expected total %d, got %d", tt.expectedTotal, total)
			}
			if len(results) != tt.expectedLen {
				t.Errorf("expected %d results, got %d", tt.expectedLen, len(results))
			}
		})
	}
}

func TestQuery_PaginationOrder(t *testing.T) {
	ctx := context.Background()
	el := New()

	for i := 0; i < 5; i++ {
		el.Record(ctx, Event{
			Simulator: "libvirt-sim",
			HostID:    fmt.Sprintf("host-%03d", i),
			Operation: "domain_create",
			Result:    "success",
		})
	}

	results, _ := el.Query(ctx, Filter{Limit: 2, Offset: 0})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].HostID != "host-000" {
		t.Errorf("expected first event host-000, got %s", results[0].HostID)
	}
	if results[1].HostID != "host-001" {
		t.Errorf("expected second event host-001, got %s", results[1].HostID)
	}

	results2, _ := el.Query(ctx, Filter{Limit: 2, Offset: 2})
	if results2[0].HostID != "host-002" {
		t.Errorf("expected third event host-002, got %s", results2[0].HostID)
	}
}

func TestClear(t *testing.T) {
	ctx := context.Background()
	el := New()

	el.Record(ctx, Event{Simulator: "libvirt-sim", Operation: "domain_create"})
	el.Record(ctx, Event{Simulator: "storage-sim", Operation: "volume_create"})

	_, total := el.Query(ctx, Filter{})
	if total != 2 {
		t.Fatalf("expected 2 events before clear, got %d", total)
	}

	el.Clear(ctx)

	_, total = el.Query(ctx, Filter{})
	if total != 0 {
		t.Fatalf("expected 0 events after clear, got %d", total)
	}
}

func TestConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	el := New()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			el.Record(ctx, Event{
				Simulator: "libvirt-sim",
				HostID:    fmt.Sprintf("host-%03d", n),
				Operation: "domain_create",
				Result:    "success",
			})
		}(i)
	}
	wg.Wait()

	_, total := el.Query(ctx, Filter{})
	if total != 100 {
		t.Fatalf("expected 100 events, got %d", total)
	}
}
