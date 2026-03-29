// Package snapshot provides state snapshot and restore capabilities.
package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Snapshot represents a saved state snapshot.
type Snapshot struct {
	ID        string                       `json:"id"`
	CreatedAt time.Time                    `json:"created_at"`
	Data      map[string]json.RawMessage   `json:"data"`
}

// Snapshotable is implemented by components that can be snapshot/restored.
type Snapshotable interface {
	SnapshotState(ctx context.Context) (json.RawMessage, error)
	RestoreState(ctx context.Context, data json.RawMessage) error
}

// Manager manages snapshots of registered components.
type Manager struct {
	mu         sync.RWMutex
	snapshots  map[string]*Snapshot
	components map[string]Snapshotable
	counter    int
}

// NewManager creates a new snapshot Manager.
func NewManager() *Manager {
	return &Manager{
		snapshots:  make(map[string]*Snapshot),
		components: make(map[string]Snapshotable),
	}
}

// Register adds a named component for snapshot management.
func (m *Manager) Register(name string, component Snapshotable) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components[name] = component
}

// TakeSnapshot saves the current state of all registered components.
func (m *Manager) TakeSnapshot(ctx context.Context) (*Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counter++
	snap := &Snapshot{
		ID:        fmt.Sprintf("snap-%03d", m.counter),
		CreatedAt: time.Now().UTC(),
		Data:      make(map[string]json.RawMessage),
	}

	for name, component := range m.components {
		data, err := component.SnapshotState(ctx)
		if err != nil {
			return nil, fmt.Errorf("snapshot component %q: %w", name, err)
		}
		snap.Data[name] = data
	}

	m.snapshots[snap.ID] = snap
	return snap, nil
}

// RestoreSnapshot restores all components to a saved snapshot.
func (m *Manager) RestoreSnapshot(ctx context.Context, id string) error {
	m.mu.RLock()
	snap, ok := m.snapshots[id]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("restore snapshot %q: snapshot not found", id)
	}
	m.mu.RUnlock()

	for name, data := range snap.Data {
		component, ok := m.components[name]
		if !ok {
			continue
		}
		if err := component.RestoreState(ctx, data); err != nil {
			return fmt.Errorf("restore component %q: %w", name, err)
		}
	}

	return nil
}

// ListSnapshots returns all saved snapshots (without data for brevity).
func (m *Manager) ListSnapshots(_ context.Context) []Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Snapshot, 0, len(m.snapshots))
	for _, s := range m.snapshots {
		result = append(result, Snapshot{
			ID:        s.ID,
			CreatedAt: s.CreatedAt,
		})
	}
	return result
}

// DeleteSnapshot removes a snapshot.
func (m *Manager) DeleteSnapshot(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.snapshots[id]; !ok {
		return fmt.Errorf("delete snapshot %q: snapshot not found", id)
	}
	delete(m.snapshots, id)
	return nil
}
