package rpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

func TestServerConnectOpenClose(t *testing.T) {
	store := state.NewStore()
	host := &state.Host{
		HostID:         "test-host",
		LibvirtPort:    0, // We'll use a random port
		CPUModel:       "Test CPU",
		CPUSockets:     1,
		CoresPerSocket: 2,
		ThreadsPerCore: 1,
		MemoryMB:       4096,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Use net.Pipe for testing instead of real TCP
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	handler := NewHandler(store, "test-host", logger)

	// Send CONNECT_OPEN
	openMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcConnectOpen,
			Type:      MessageTypeCall,
			Serial:    1,
			Status:    StatusOK,
		},
		Body: func() []byte {
			enc := NewXDREncoder()
			uri := "qemu:///system"
			enc.WriteOptionalString(&uri)
			enc.WriteUint32(0) // flags
			return enc.Bytes()
		}(),
	}

	reply := handler.HandleMessage(openMsg)
	if reply.Header.Status != StatusOK {
		t.Errorf("CONNECT_OPEN status = %d, want OK", reply.Header.Status)
	}
	if reply.Header.Serial != 1 {
		t.Errorf("serial = %d, want 1", reply.Header.Serial)
	}

	// Send CONNECT_CLOSE
	closeMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcConnectClose,
			Type:      MessageTypeCall,
			Serial:    2,
			Status:    StatusOK,
		},
	}

	reply = handler.HandleMessage(closeMsg)
	if reply.Header.Status != StatusOK {
		t.Errorf("CONNECT_CLOSE status = %d, want OK", reply.Header.Status)
	}
}

func TestServerDomainLifecycle(t *testing.T) {
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

	// Define a domain
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
			enc.WriteUint32(0) // flags
			return enc.Bytes()
		}(),
	}

	reply := handler.HandleMessage(defineMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("DEFINE status = %d, want OK", reply.Header.Status)
	}

	// Verify domain was created
	uuid := [16]byte{0x12, 0x34, 0x56, 0x78, 0x12, 0x34, 0x12, 0x34,
		0x12, 0x34, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}

	// Get state - should be shutoff
	getStateMsg := makeDomainMsg(ProcDomainGetState, 2, "test-vm", uuid, -1)
	reply = handler.HandleMessage(getStateMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("GET_STATE status = %d, want OK", reply.Header.Status)
	}
	dec := NewXDRDecoder(reply.Body)
	domState, _ := dec.ReadInt32()
	if domState != int32(state.DomainStateShutoff) {
		t.Errorf("state = %d, want shutoff (%d)", domState, state.DomainStateShutoff)
	}

	// Start the domain
	startMsg := makeDomainMsg(ProcDomainCreateWithFlags, 3, "test-vm", uuid, -1)
	// Append flags
	enc := NewXDREncoder()
	enc.WriteString("test-vm")
	enc.WriteUUID(uuid)
	enc.WriteInt32(-1)
	enc.WriteUint32(0) // flags
	startMsg.Body = enc.Bytes()

	reply = handler.HandleMessage(startMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("CREATE_WITH_FLAGS status = %d, want OK", reply.Header.Status)
	}

	// Get state - should be running
	getStateMsg = makeDomainMsg(ProcDomainGetState, 4, "test-vm", uuid, -1)
	reply = handler.HandleMessage(getStateMsg)
	dec = NewXDRDecoder(reply.Body)
	domState, _ = dec.ReadInt32()
	if domState != int32(state.DomainStateRunning) {
		t.Errorf("state = %d, want running (%d)", domState, state.DomainStateRunning)
	}

	// Destroy
	destroyMsg := makeDomainMsg(ProcDomainDestroyFlags, 5, "test-vm", uuid, -1)
	enc = NewXDREncoder()
	enc.WriteString("test-vm")
	enc.WriteUUID(uuid)
	enc.WriteInt32(-1)
	enc.WriteUint32(0)
	destroyMsg.Body = enc.Bytes()

	reply = handler.HandleMessage(destroyMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("DESTROY status = %d, want OK", reply.Header.Status)
	}
}

func TestServerIntegration(t *testing.T) {
	store := state.NewStore()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	host := &state.Host{
		HostID:         "int-test-host",
		LibvirtPort:    0,
		CPUModel:       "Test CPU",
		CPUSockets:     1,
		CoresPerSocket: 2,
		ThreadsPerCore: 1,
		MemoryMB:       4096,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(store, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Find a free port
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Update host port
	host.LibvirtPort = port

	if err := srv.StartListener(ctx, "int-test-host", port); err != nil {
		t.Fatal(err)
	}
	defer srv.StopAll()

	// Give listener time to start
	time.Sleep(50 * time.Millisecond)

	// Connect
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send CONNECT_OPEN
	openEnc := NewXDREncoder()
	uri := "qemu:///system"
	openEnc.WriteOptionalString(&uri)
	openEnc.WriteUint32(0)

	err = WriteMessage(conn, &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcConnectOpen,
			Type:      MessageTypeCall,
			Serial:    1,
			Status:    StatusOK,
		},
		Body: openEnc.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := ReadMessage(conn)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Header.Status != StatusOK {
		t.Errorf("CONNECT_OPEN status = %d, want OK", reply.Header.Status)
	}

	// Get hostname
	err = WriteMessage(conn, &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcConnectGetHostname,
			Type:      MessageTypeCall,
			Serial:    2,
			Status:    StatusOK,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	reply, err = ReadMessage(conn)
	if err != nil {
		t.Fatal(err)
	}
	if reply.Header.Status != StatusOK {
		t.Errorf("GET_HOSTNAME status = %d, want OK", reply.Header.Status)
	}

	dec := NewXDRDecoder(reply.Body)
	hostname, err := dec.ReadString()
	if err != nil {
		t.Fatal(err)
	}
	if hostname != "int-test-host" {
		t.Errorf("hostname = %q, want %q", hostname, "int-test-host")
	}
}

func makeDomainMsg(proc int32, serial uint32, name string, uuid [16]byte, id int32) *Message {
	enc := NewXDREncoder()
	enc.WriteString(name)
	enc.WriteUUID(uuid)
	enc.WriteInt32(id)

	return &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: proc,
			Type:      MessageTypeCall,
			Serial:    serial,
			Status:    StatusOK,
		},
		Body: enc.Bytes(),
	}
}

