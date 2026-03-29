package rpc

import (
	"log/slog"
	"os"
	"testing"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

func setupMigrationTest(t *testing.T) (*state.Store, *Handler, *Handler) {
	t.Helper()

	store := state.NewStore()
	// Set zero delays for tests
	store.SetMigrationConfig(state.MigrationConfig{
		PrepareDurationMs:      0,
		BaseTransferDurationMs: 0,
		PerGBMemoryMs:          0,
		FinishDurationMs:       0,
	})

	src := &state.Host{
		HostID:             "src-host",
		LibvirtPort:        16509,
		CPUModel:           "Test CPU",
		CPUSockets:         2,
		CoresPerSocket:     4,
		ThreadsPerCore:     2,
		MemoryMB:           32768,
		CPUOvercommitRatio: 4.0,
		MemOvercommitRatio: 1.5,
	}
	dest := &state.Host{
		HostID:             "dest-host",
		LibvirtPort:        16510,
		CPUModel:           "Test CPU",
		CPUSockets:         2,
		CoresPerSocket:     4,
		ThreadsPerCore:     2,
		MemoryMB:           32768,
		CPUOvercommitRatio: 4.0,
		MemOvercommitRatio: 1.5,
	}

	if err := store.AddHost(src); err != nil {
		t.Fatal(err)
	}
	if err := store.AddHost(dest); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srcHandler := NewHandler(store, "src-host", logger)
	destHandler := NewHandler(store, "dest-host", logger)

	return store, srcHandler, destHandler
}

func defineAndStartDomain(t *testing.T, handler *Handler, store *state.Store, hostID string) *state.Domain {
	t.Helper()

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

	reply := handler.HandleMessage(defineMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("DEFINE failed: status=%d", reply.Header.Status)
	}

	uuid := [16]byte{0x12, 0x34, 0x56, 0x78, 0x12, 0x34, 0x12, 0x34,
		0x12, 0x34, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}

	// Start the domain
	startMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcDomainCreateWithFlags,
			Type:      MessageTypeCall,
			Serial:    2,
			Status:    StatusOK,
		},
		Body: func() []byte {
			enc := NewXDREncoder()
			enc.WriteString("test-vm")
			enc.WriteUUID(uuid)
			enc.WriteInt32(-1)
			enc.WriteUint32(0)
			return enc.Bytes()
		}(),
	}

	reply = handler.HandleMessage(startMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("START failed: status=%d", reply.Header.Status)
	}

	dom, err := store.GetDomainByName(hostID, "test-vm")
	if err != nil {
		t.Fatal(err)
	}
	return dom
}

func makeMigrateParamsMsg(proc int32, serial uint32, params map[string]string) *Message {
	enc := NewXDREncoder()
	enc.WriteUint32(uint32(len(params)))
	for k, v := range params {
		enc.WriteString(k)
		enc.WriteInt32(7) // VIR_TYPED_PARAM_STRING
		enc.WriteString(v)
	}

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

func TestMigrationPreparePerformFinishConfirm(t *testing.T) {
	store, srcHandler, destHandler := setupMigrationTest(t)
	dom := defineAndStartDomain(t, srcHandler, store, "src-host")

	// Step 1: Prepare on destination
	prepareMsg := makeMigrateParamsMsg(ProcDomainMigratePrepare3Params, 10, map[string]string{
		"domain_name": dom.Name,
		"domain_uuid": dom.UUIDString(),
		"dest_uri":    "qemu+tcp://dest-host/system",
	})
	reply := destHandler.HandleMessage(prepareMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("PREPARE status=%d, want OK", reply.Header.Status)
	}

	// Verify domain placeholder on dest
	destDom, err := store.GetDomain("dest-host", dom.UUIDString())
	if err != nil {
		t.Fatalf("placeholder not found: %v", err)
	}
	if destDom.MigrationState != state.MigrationStatePrepared {
		t.Errorf("dest domain migration state=%d, want Prepared", destDom.MigrationState)
	}

	// Step 2: Perform on source
	performMsg := makeMigrateParamsMsg(ProcDomainMigratePerform3Params, 11, map[string]string{
		"domain_uuid": dom.UUIDString(),
	})
	reply = srcHandler.HandleMessage(performMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("PERFORM status=%d, want OK", reply.Header.Status)
	}

	srcDom, err := store.GetDomain("src-host", dom.UUIDString())
	if err != nil {
		t.Fatalf("source domain not found: %v", err)
	}
	if srcDom.MigrationState != state.MigrationStatePerforming {
		t.Errorf("src domain migration state=%d, want Performing", srcDom.MigrationState)
	}

	// Step 3: Finish on destination
	finishMsg := makeMigrateParamsMsg(ProcDomainMigrateFinish3Params, 12, map[string]string{
		"domain_uuid": dom.UUIDString(),
	})
	reply = destHandler.HandleMessage(finishMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("FINISH status=%d, want OK", reply.Header.Status)
	}

	// Verify domain running on dest
	destDomFinal, err := store.GetDomain("dest-host", dom.UUIDString())
	if err != nil {
		t.Fatalf("dest domain not found: %v", err)
	}
	if destDomFinal.State != state.DomainStateRunning {
		t.Errorf("dest domain state=%d, want Running", destDomFinal.State)
	}

	// Step 4: Confirm on source
	confirmMsg := makeMigrateParamsMsg(ProcDomainMigrateConfirm3Params, 13, map[string]string{
		"domain_uuid": dom.UUIDString(),
	})
	reply = srcHandler.HandleMessage(confirmMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("CONFIRM status=%d, want OK", reply.Header.Status)
	}

	// Verify domain removed from source
	_, err = store.GetDomain("src-host", dom.UUIDString())
	if err == nil {
		t.Error("expected domain to be removed from source")
	}
}

func TestMigrationPrepareInsufficientResources(t *testing.T) {
	store, srcHandler, destHandler := setupMigrationTest(t)
	dom := defineAndStartDomain(t, srcHandler, store, "src-host")

	// Fill destination with domains to exhaust resources
	destHost, _ := store.GetHost("dest-host")
	destHost.CPUSockets = 1
	destHost.CoresPerSocket = 1
	destHost.ThreadsPerCore = 1
	destHost.MemoryMB = 2048
	destHost.CPUOvercommitRatio = 1.0
	destHost.MemOvercommitRatio = 1.0

	prepareMsg := makeMigrateParamsMsg(ProcDomainMigratePrepare3Params, 10, map[string]string{
		"domain_name": dom.Name,
		"domain_uuid": dom.UUIDString(),
	})
	reply := destHandler.HandleMessage(prepareMsg)
	if reply.Header.Status != StatusError {
		t.Fatalf("PREPARE status=%d, want Error (insufficient resources)", reply.Header.Status)
	}
}

func TestMigrationPerformNotRunning(t *testing.T) {
	store, srcHandler, _ := setupMigrationTest(t)

	// Define but don't start
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
	srcHandler.HandleMessage(defineMsg)

	dom, _ := store.GetDomainByName("src-host", "test-vm")

	performMsg := makeMigrateParamsMsg(ProcDomainMigratePerform3Params, 10, map[string]string{
		"domain_uuid": dom.UUIDString(),
	})
	reply := srcHandler.HandleMessage(performMsg)
	if reply.Header.Status != StatusError {
		t.Fatalf("PERFORM status=%d, want Error (not running)", reply.Header.Status)
	}
}

func TestMigrateGetSetMaxSpeed(t *testing.T) {
	store, srcHandler, _ := setupMigrationTest(t)
	_ = defineAndStartDomain(t, srcHandler, store, "src-host")

	uuid := [16]byte{0x12, 0x34, 0x56, 0x78, 0x12, 0x34, 0x12, 0x34,
		0x12, 0x34, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}

	// Get max speed
	getMsg := makeDomainMsg(ProcDomainMigrateGetMaxSpeed, 20, "test-vm", uuid, -1)
	reply := srcHandler.HandleMessage(getMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("GET_MAX_SPEED status=%d, want OK", reply.Header.Status)
	}
	dec := NewXDRDecoder(reply.Body)
	speed, err := dec.ReadUint64()
	if err != nil {
		t.Fatal(err)
	}
	if speed == 0 {
		t.Error("expected non-zero default speed")
	}

	// Set max speed
	setBody := func() []byte {
		enc := NewXDREncoder()
		enc.WriteString("test-vm")
		enc.WriteUUID(uuid)
		enc.WriteInt32(-1)
		enc.WriteUint64(1000) // 1000 MiB/s
		enc.WriteUint32(0)    // flags
		return enc.Bytes()
	}()
	setMsg := &Message{
		Header: Header{
			Program:   RemoteProgram,
			Version:   RemoteProtocolVersion,
			Procedure: ProcDomainMigrateSetMaxSpeed,
			Type:      MessageTypeCall,
			Serial:    21,
			Status:    StatusOK,
		},
		Body: setBody,
	}
	reply = srcHandler.HandleMessage(setMsg)
	if reply.Header.Status != StatusOK {
		t.Fatalf("SET_MAX_SPEED status=%d, want OK", reply.Header.Status)
	}

	// Verify speed was set
	gotSpeed := store.GetMigrationSpeed()
	if gotSpeed != 1000 {
		t.Errorf("migration speed = %d, want 1000", gotSpeed)
	}
}
