package rpc

import (
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

func TestEventRegistrationAndDeregistration(t *testing.T) {
	store := state.NewStore()
	host := &state.Host{
		HostID:             "test-host",
		LibvirtPort:        0,
		CPUModel:           "Test CPU",
		CPUSockets:         2,
		CoresPerSocket:     4,
		ThreadsPerCore:     2,
		MemoryMB:           32768,
		CPUOvercommitRatio: 4.0,
		MemOvercommitRatio: 1.5,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := NewHandler(store, "test-host", logger)

	// Set up event system
	client, _ := net.Pipe()
	defer client.Close()

	eb := NewEventBus()
	ce := eb.RegisterClient(client, "test-host")
	handler.SetEventBus(eb)
	handler.SetClientEvents(ce)

	// Register for lifecycle events
	regMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcConnectDomainEventRegisterAny,
			Type:      MessageTypeCall,
			Serial:    1,
			Status:    StatusOK,
		},
		Body: func() []byte {
			enc := NewXDREncoder()
			enc.WriteInt32(VirDomainEventIDLifecycle)
			return enc.Bytes()
		}(),
	}

	reply := handler.HandleMessage(regMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("REGISTER status=%d, want OK", reply.Header.Status)
	}

	// Decode callback ID
	dec := NewXDRDecoder(reply.Body)
	cbID, err := dec.ReadInt32()
	if err != nil {
		t.Fatal(err)
	}
	if cbID <= 0 {
		t.Errorf("callback ID=%d, want >0", cbID)
	}

	// Verify callback is registered
	callbacks := ce.GetCallbacksForEvent(VirDomainEventIDLifecycle)
	if len(callbacks) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(callbacks))
	}
	if callbacks[0] != cbID {
		t.Errorf("callback ID=%d, want %d", callbacks[0], cbID)
	}

	// Deregister by eventID (matching go-libvirt protocol)
	deregMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcConnectDomainEventDeregisterAny,
			Type:      MessageTypeCall,
			Serial:    2,
			Status:    StatusOK,
		},
		Body: func() []byte {
			enc := NewXDREncoder()
			enc.WriteInt32(VirDomainEventIDLifecycle) // eventID, not callbackID
			return enc.Bytes()
		}(),
	}

	reply = handler.HandleMessage(deregMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("DEREGISTER status=%d, want OK", reply.Header.Status)
	}

	callbacks = ce.GetCallbacksForEvent(VirDomainEventIDLifecycle)
	if len(callbacks) != 0 {
		t.Errorf("expected 0 callbacks after deregister, got %d", len(callbacks))
	}
}

func TestEventDeregisterInvalidCallback(t *testing.T) {
	store := state.NewStore()
	host := &state.Host{
		HostID:      "test-host",
		LibvirtPort: 0,
		MemoryMB:    4096,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := NewHandler(store, "test-host", logger)

	client, _ := net.Pipe()
	defer client.Close()

	eb := NewEventBus()
	ce := eb.RegisterClient(client, "test-host")
	handler.SetEventBus(eb)
	handler.SetClientEvents(ce)

	deregMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcConnectDomainEventDeregisterAny,
			Type:      MessageTypeCall,
			Serial:    1,
			Status:    StatusOK,
		},
		Body: func() []byte {
			enc := NewXDREncoder()
			enc.WriteInt32(999) // non-existent callback
			return enc.Bytes()
		}(),
	}

	reply := handler.HandleMessage(deregMsg)
	if reply.Header.Status != StatusError {
		t.Errorf("DEREGISTER invalid status=%d, want Error", reply.Header.Status)
	}
}

func TestEventEmissionOnStateChange(t *testing.T) {
	store := state.NewStore()
	host := &state.Host{
		HostID:             "test-host",
		LibvirtPort:        0,
		CPUModel:           "Test CPU",
		CPUSockets:         2,
		CoresPerSocket:     4,
		ThreadsPerCore:     2,
		MemoryMB:           32768,
		CPUOvercommitRatio: 4.0,
		MemOvercommitRatio: 1.5,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := NewHandler(store, "test-host", logger)

	// Use a real pipe to capture events
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	eb := NewEventBus()
	ce := eb.RegisterClient(server, "test-host")
	handler.SetEventBus(eb)
	handler.SetClientEvents(ce)

	// Register for lifecycle events
	ce.Register(VirDomainEventIDLifecycle)

	// Define a domain - should emit DEFINED event
	domXML := `<domain type="kvm">
  <name>test-vm</name>
  <uuid>12345678-1234-1234-1234-123456789abc</uuid>
  <memory unit="KiB">4194304</memory>
  <vcpu>4</vcpu>
</domain>`

	defineMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcDomainDefineXMLFlags,
			Type:      MessageTypeCall,
			Serial:    1,
			Status:    StatusOK,
		},
		Body: func() []byte {
			enc := NewXDREncoder()
			enc.WriteString(domXML)
			enc.WriteUint32(0)
			return enc.Bytes()
		}(),
	}

	// Read events from client side in a goroutine
	eventCh := make(chan *Message, 10)
	go func() {
		for {
			msg, err := ReadMessage(client)
			if err != nil {
				return
			}
			eventCh <- msg
		}
	}()

	reply := handler.HandleMessage(defineMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("DEFINE failed: status=%d", reply.Header.Status)
	}

	// Write the reply back so the goroutine doesn't block (it reads the event first)
	// The event is sent before the reply in our flow, so read the event
	select {
	case event := <-eventCh:
		if event.Header.Procedure != ProcDomainEventLifecycle {
			t.Errorf("event procedure=%d, want %d", event.Header.Procedure, ProcDomainEventLifecycle)
		}
		if event.Header.Type != MessageTypeMessage {
			t.Errorf("event type=%d, want Message(%d)", event.Header.Type, MessageTypeMessage)
		}
		// Decode event body
		dec := NewXDRDecoder(event.Body)
		cbID, _ := dec.ReadInt32()
		if cbID <= 0 {
			t.Errorf("callback ID=%d, want >0", cbID)
		}
		// domain ref
		name, _ := dec.ReadString()
		if name != "test-vm" {
			t.Errorf("event domain name=%q, want test-vm", name)
		}
		_, _ = dec.ReadUUID()
		_, _ = dec.ReadInt32()
		// event type and detail
		eventType, _ := dec.ReadInt32()
		if eventType != DomainEventDefined {
			t.Errorf("event type=%d, want DEFINED(%d)", eventType, DomainEventDefined)
		}
		eventDetail, _ := dec.ReadInt32()
		if eventDetail != DomainEventDetailAdded {
			t.Errorf("event detail=%d, want ADDED(%d)", eventDetail, DomainEventDetailAdded)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for DEFINED event")
	}
}

func TestClientEventsUnit(t *testing.T) {
	client, _ := net.Pipe()
	defer client.Close()

	ce := NewClientEvents(client, "host1")

	// Register multiple callbacks
	cb1 := ce.Register(VirDomainEventIDLifecycle)
	cb2 := ce.Register(VirDomainEventIDLifecycle)
	cb3 := ce.Register(99) // different event type

	// Get callbacks for lifecycle
	cbs := ce.GetCallbacksForEvent(VirDomainEventIDLifecycle)
	if len(cbs) != 2 {
		t.Errorf("lifecycle callbacks=%d, want 2", len(cbs))
	}

	// Get callbacks for other event type
	cbs = ce.GetCallbacksForEvent(99)
	if len(cbs) != 1 {
		t.Errorf("event 99 callbacks=%d, want 1", len(cbs))
	}

	// Deregister one
	if !ce.Deregister(cb1) {
		t.Error("deregister cb1 returned false")
	}

	cbs = ce.GetCallbacksForEvent(VirDomainEventIDLifecycle)
	if len(cbs) != 1 {
		t.Errorf("after deregister: lifecycle callbacks=%d, want 1", len(cbs))
	}
	if cbs[0] != cb2 {
		t.Errorf("remaining callback=%d, want %d", cbs[0], cb2)
	}

	// Deregister non-existent
	if ce.Deregister(999) {
		t.Error("deregister non-existent returned true")
	}

	_ = cb3 // used above
}

func TestEventBusMultipleClients(t *testing.T) {
	eb := NewEventBus()

	c1, _ := net.Pipe()
	defer c1.Close()
	c2, _ := net.Pipe()
	defer c2.Close()

	ce1 := eb.RegisterClient(c1, "host1")
	ce2 := eb.RegisterClient(c2, "host1")

	if ce1 == nil || ce2 == nil {
		t.Fatal("RegisterClient returned nil")
	}

	// Verify lookup
	got1 := eb.GetClientEvents(c1)
	if got1 != ce1 {
		t.Error("GetClientEvents returned wrong client")
	}

	// Unregister
	eb.UnregisterClient(c1)
	got1 = eb.GetClientEvents(c1)
	if got1 != nil {
		t.Error("expected nil after unregister")
	}

	// c2 should still be there
	got2 := eb.GetClientEvents(c2)
	if got2 != ce2 {
		t.Error("c2 should still be registered")
	}
}
