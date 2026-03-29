// Package eventlog provides an in-memory event log store for cirrus-sim simulators.
// It is safe for concurrent use and designed to be imported by all simulator modules.
package eventlog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Event represents a single simulator event.
type Event struct {
	// ID is the unique identifier for this event, assigned on recording.
	ID string `json:"id"`
	// Timestamp is the time the event was recorded.
	Timestamp time.Time `json:"timestamp"`
	// Simulator is the name of the simulator that generated this event.
	Simulator string `json:"simulator"`
	// HostID is the identifier of the host associated with this event.
	HostID string `json:"host_id"`
	// Operation is the type of operation that was performed.
	Operation string `json:"operation"`
	// RequestSummary is a brief description of the request.
	RequestSummary string `json:"request_summary"`
	// Result is the outcome of the operation (e.g., "success", "failure").
	Result string `json:"result"`
	// DurationMs is the duration of the operation in milliseconds.
	DurationMs int64 `json:"duration_ms"`
	// FaultInjected indicates whether a fault was injected during this operation.
	FaultInjected bool `json:"fault_injected"`
}

// Filter specifies criteria for querying events.
type Filter struct {
	// Simulator filters events by simulator name.
	Simulator string
	// HostID filters events by host identifier.
	HostID string
	// Operation filters events by operation type.
	Operation string
	// After filters events that occurred after this timestamp.
	After time.Time
	// Limit is the maximum number of events to return. Zero means no limit.
	Limit int
	// Offset is the number of events to skip before returning results.
	Offset int
}

// EventLog stores events in memory with thread-safe access.
type EventLog struct {
	mu      sync.RWMutex
	events  []Event
	counter uint64
}

// New creates a new EventLog instance.
func New() *EventLog {
	return &EventLog{
		events: make([]Event, 0),
	}
}

// Record adds a new event to the log. It assigns an ID and timestamp,
// then returns the generated event ID.
func (el *EventLog) Record(ctx context.Context, event Event) string {
	el.mu.Lock()
	defer el.mu.Unlock()

	el.counter++
	event.ID = fmt.Sprintf("evt-%06d", el.counter)
	event.Timestamp = time.Now().UTC()

	el.events = append(el.events, event)

	slog.DebugContext(ctx, "event recorded",
		"event_id", event.ID,
		"simulator", event.Simulator,
		"operation", event.Operation,
	)

	return event.ID
}

// Query returns events matching the given filter, with pagination.
// It returns the matching events (respecting limit/offset) and the total count of matching events.
func (el *EventLog) Query(ctx context.Context, filter Filter) ([]Event, int) {
	el.mu.RLock()
	defer el.mu.RUnlock()

	// Filter events
	matched := make([]Event, 0)
	for _, e := range el.events {
		if filter.Simulator != "" && e.Simulator != filter.Simulator {
			continue
		}
		if filter.HostID != "" && e.HostID != filter.HostID {
			continue
		}
		if filter.Operation != "" && e.Operation != filter.Operation {
			continue
		}
		if !filter.After.IsZero() && !e.Timestamp.After(filter.After) {
			continue
		}
		matched = append(matched, e)
	}

	total := len(matched)

	// Apply pagination
	if filter.Offset > 0 {
		if filter.Offset >= len(matched) {
			return []Event{}, total
		}
		matched = matched[filter.Offset:]
	}

	if filter.Limit > 0 && filter.Limit < len(matched) {
		matched = matched[:filter.Limit]
	}

	slog.DebugContext(ctx, "events queried",
		"total", total,
		"returned", len(matched),
	)

	return matched, total
}

// Clear removes all events from the log.
func (el *EventLog) Clear(ctx context.Context) {
	el.mu.Lock()
	defer el.mu.Unlock()

	el.events = make([]Event, 0)
	el.counter = 0

	slog.InfoContext(ctx, "event log cleared")
}
