package snapshot

import (
	"context"
	"encoding/json"
	"testing"
)

type mockComponent struct {
	value string
}

func (m *mockComponent) SnapshotState(_ context.Context) (json.RawMessage, error) {
	return json.Marshal(m.value)
}

func (m *mockComponent) RestoreState(_ context.Context, data json.RawMessage) error {
	return json.Unmarshal(data, &m.value)
}

func TestSnapshotAndRestore(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	comp := &mockComponent{value: "original"}
	mgr.Register("test", comp)

	snap, err := mgr.TakeSnapshot(ctx)
	if err != nil {
		t.Fatalf("take snapshot: %v", err)
	}
	if snap.ID == "" {
		t.Error("expected non-empty snapshot ID")
	}

	// Change state
	comp.value = "modified"

	// Restore
	if err := mgr.RestoreSnapshot(ctx, snap.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if comp.value != "original" {
		t.Errorf("value = %q, want %q", comp.value, "original")
	}
}

func TestListAndDeleteSnapshots(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	comp := &mockComponent{value: "test"}
	mgr.Register("test", comp)

	if _, err := mgr.TakeSnapshot(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.TakeSnapshot(ctx); err != nil {
		t.Fatal(err)
	}

	list := mgr.ListSnapshots(ctx)
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2", len(list))
	}

	if err := mgr.DeleteSnapshot(ctx, list[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	list = mgr.ListSnapshots(ctx)
	if len(list) != 1 {
		t.Errorf("list count after delete = %d, want 1", len(list))
	}
}

func TestRestoreNonexistent(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	if err := mgr.RestoreSnapshot(ctx, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent snapshot")
	}
}

func TestDeleteNonexistent(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	if err := mgr.DeleteSnapshot(ctx, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent snapshot")
	}
}
