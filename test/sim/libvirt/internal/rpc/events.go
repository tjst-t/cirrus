// Package rpc implements the libvirt RPC protocol for libvirt-sim.
package rpc

import (
	"net"
	"sync"
	"sync/atomic"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

// Domain lifecycle event types matching libvirt virDomainEventType.
const (
	// DomainEventDefined is VIR_DOMAIN_EVENT_DEFINED.
	DomainEventDefined int32 = 0
	// DomainEventUndefined is VIR_DOMAIN_EVENT_UNDEFINED.
	DomainEventUndefined int32 = 1
	// DomainEventStarted is VIR_DOMAIN_EVENT_STARTED.
	DomainEventStarted int32 = 2
	// DomainEventSuspended is VIR_DOMAIN_EVENT_SUSPENDED.
	DomainEventSuspended int32 = 3
	// DomainEventResumed is VIR_DOMAIN_EVENT_RESUMED.
	DomainEventResumed int32 = 4
	// DomainEventStopped is VIR_DOMAIN_EVENT_STOPPED.
	DomainEventStopped int32 = 5
)

// Domain lifecycle event details.
const (
	// DomainEventDetailAdded is VIR_DOMAIN_EVENT_DEFINED_ADDED.
	DomainEventDetailAdded int32 = 0
	// DomainEventDetailRemoved is VIR_DOMAIN_EVENT_UNDEFINED_REMOVED.
	DomainEventDetailRemoved int32 = 0
	// DomainEventDetailBooted is VIR_DOMAIN_EVENT_STARTED_BOOTED.
	DomainEventDetailBooted int32 = 0
	// DomainEventDetailMigrated is detail=1 for STARTED/SUSPENDED/RESUMED.
	DomainEventDetailMigrated int32 = 1
	// DomainEventDetailPaused is VIR_DOMAIN_EVENT_SUSPENDED_PAUSED.
	DomainEventDetailPaused int32 = 0
	// DomainEventDetailUnpaused is VIR_DOMAIN_EVENT_RESUMED_UNPAUSED.
	DomainEventDetailUnpaused int32 = 0
	// DomainEventDetailShutdown is VIR_DOMAIN_EVENT_STOPPED_SHUTDOWN.
	DomainEventDetailShutdown int32 = 0
	// DomainEventDetailDestroyed is VIR_DOMAIN_EVENT_STOPPED_DESTROYED.
	DomainEventDetailDestroyed int32 = 1
)

// Libvirt event IDs for event registration.
const (
	// VirDomainEventIDLifecycle is the lifecycle event ID.
	VirDomainEventIDLifecycle int32 = 0
)

// EventCallback represents a registered event callback.
type EventCallback struct {
	CallbackID int32
	EventID    int32
}

// ClientEvents tracks event registrations for a single client connection.
type ClientEvents struct {
	mu         sync.RWMutex
	conn       net.Conn
	callbacks  map[int32]*EventCallback // key: callbackID
	nextCBID   atomic.Int32
	hostID     string
}

// NewClientEvents creates a new event tracker for a client connection.
func NewClientEvents(conn net.Conn, hostID string) *ClientEvents {
	ce := &ClientEvents{
		conn:      conn,
		callbacks: make(map[int32]*EventCallback),
		hostID:    hostID,
	}
	ce.nextCBID.Store(1)
	return ce
}

// Register registers a callback for an event type and returns the callback ID.
func (ce *ClientEvents) Register(eventID int32) int32 {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	cbID := ce.nextCBID.Add(1) - 1
	ce.callbacks[cbID] = &EventCallback{
		CallbackID: cbID,
		EventID:    eventID,
	}
	return cbID
}

// Deregister removes a callback by ID. Returns false if not found.
func (ce *ClientEvents) Deregister(callbackID int32) bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	if _, ok := ce.callbacks[callbackID]; !ok {
		return false
	}
	delete(ce.callbacks, callbackID)
	return true
}

// DeregisterByEventID removes all callbacks for the given event type. Returns false if none found.
func (ce *ClientEvents) DeregisterByEventID(eventID int32) bool {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	found := false
	for cbID, cb := range ce.callbacks {
		if cb.EventID == eventID {
			delete(ce.callbacks, cbID)
			found = true
		}
	}
	return found
}

// GetCallbacksForEvent returns all callback IDs registered for the given event type.
func (ce *ClientEvents) GetCallbacksForEvent(eventID int32) []int32 {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	var ids []int32
	for _, cb := range ce.callbacks {
		if cb.EventID == eventID {
			ids = append(ids, cb.CallbackID)
		}
	}
	return ids
}

// EventBus manages event distribution across all client connections for a host.
type EventBus struct {
	mu      sync.RWMutex
	clients map[net.Conn]*ClientEvents
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		clients: make(map[net.Conn]*ClientEvents),
	}
}

// RegisterClient adds a client connection to the event bus.
func (eb *EventBus) RegisterClient(conn net.Conn, hostID string) *ClientEvents {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ce := NewClientEvents(conn, hostID)
	eb.clients[conn] = ce
	return ce
}

// UnregisterClient removes a client connection from the event bus.
func (eb *EventBus) UnregisterClient(conn net.Conn) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.clients, conn)
}

// GetClientEvents returns the event tracker for a specific connection.
func (eb *EventBus) GetClientEvents(conn net.Conn) *ClientEvents {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return eb.clients[conn]
}

// EmitDomainLifecycleEvent sends a lifecycle event to all registered clients for the host.
func (eb *EventBus) EmitDomainLifecycleEvent(hostID string, dom *state.Domain, event int32, detail int32) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for conn, ce := range eb.clients {
		if ce.hostID != hostID {
			continue
		}
		cbIDs := ce.GetCallbacksForEvent(VirDomainEventIDLifecycle)
		for _, cbID := range cbIDs {
			msg := buildLifecycleEventMessage(cbID, dom, event, detail)
			// Best effort send - ignore errors (client may have disconnected)
			_ = WriteMessage(conn, msg)
		}
	}
}

// buildLifecycleEventMessage builds a REMOTE_PROC_DOMAIN_EVENT_LIFECYCLE message.
func buildLifecycleEventMessage(callbackID int32, dom *state.Domain, event int32, detail int32) *Message {
	enc := NewXDREncoder()
	enc.WriteInt32(callbackID)
	// Domain ref
	enc.WriteString(dom.Name)
	enc.WriteUUID(dom.UUID)
	enc.WriteInt32(dom.ID)
	// Event
	enc.WriteInt32(event)
	enc.WriteInt32(detail)

	return &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcDomainEventLifecycle,
			Type:      MessageTypeMessage,
			Serial:    0,
			Status:    StatusOK,
		},
		Body: enc.Bytes(),
	}
}
