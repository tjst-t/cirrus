package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/netns"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
	domxml "github.com/tjst-t/cirrus/test/sim/libvirt/internal/xml"
)

// Handler processes libvirt RPC requests for a specific host.
type Handler struct {
	store        *state.Store
	hostID       string
	logger       *slog.Logger
	clientEvents *ClientEvents
	eventBus     *EventBus
	netns        netns.Manager
}

// NewHandler creates a new RPC handler for a host.
func NewHandler(store *state.Store, hostID string, logger *slog.Logger) *Handler {
	return &Handler{
		store:  store,
		hostID: hostID,
		logger: logger,
		netns:  netns.NewNoopManager(logger),
	}
}

// SetNetnsManager sets the network namespace manager for VM lifecycle operations.
func (h *Handler) SetNetnsManager(m netns.Manager) {
	h.netns = m
}

// SetClientEvents sets the client event tracker for this handler.
func (h *Handler) SetClientEvents(ce *ClientEvents) {
	h.clientEvents = ce
}

// SetEventBus sets the event bus for this handler.
func (h *Handler) SetEventBus(eb *EventBus) {
	h.eventBus = eb
}

// HandleMessage dispatches an RPC message to the appropriate handler.
func (h *Handler) HandleMessage(msg *Message) *Message {
	h.logger.Debug("handling RPC", "procedure", msg.Header.Procedure, "serial", msg.Header.Serial)

	switch msg.Header.Procedure {
	case ProcAuthList:
		return h.handleAuthList(msg)
	case ProcConnectOpen:
		return h.handleConnectOpen(msg)
	case ProcConnectClose:
		return h.handleConnectClose(msg)
	case ProcConnectGetHostname:
		return h.handleGetHostname(msg)
	case ProcConnectGetCapabilities:
		return h.handleGetCapabilities(msg)
	case ProcNodeGetInfo:
		return h.handleNodeGetInfo(msg)
	case ProcDomainDefineXMLFlags:
		return h.handleDomainDefineXMLFlags(msg)
	case ProcDomainCreateWithFlags:
		return h.handleDomainCreateWithFlags(msg)
	case ProcDomainDestroyFlags:
		return h.handleDomainDestroyFlags(msg)
	case ProcDomainShutdownFlags:
		return h.handleDomainShutdownFlags(msg)
	case ProcDomainSuspend:
		return h.handleDomainSuspend(msg)
	case ProcDomainResume:
		return h.handleDomainResume(msg)
	case ProcDomainReboot:
		return h.handleDomainReboot(msg)
	case ProcDomainGetState:
		return h.handleDomainGetState(msg)
	case ProcDomainGetInfo:
		return h.handleDomainGetInfo(msg)
	case ProcDomainGetXMLDesc:
		return h.handleDomainGetXMLDesc(msg)
	case ProcDomainLookupByUUID:
		return h.handleDomainLookupByUUID(msg)
	case ProcDomainLookupByName:
		return h.handleDomainLookupByName(msg)
	case ProcDomainListAllDomains:
		return h.handleDomainListAllDomains(msg)
	case ProcDomainUndefineFlags:
		return h.handleDomainUndefineFlags(msg)
	case ProcConnectGetVersion:
		return h.handleConnectGetVersion(msg)
	case ProcConnectGetLibVersion:
		return h.handleConnectGetLibVersion(msg)
	case ProcNodeGetFreeMemory:
		return h.handleNodeGetFreeMemory(msg)
	case ProcNodeGetCPUStats:
		return h.handleNodeGetCPUStats(msg)
	case ProcNodeGetMemoryStats:
		return h.handleNodeGetMemoryStats(msg)
	case ProcConnectGetAllDomainStats:
		return h.handleGetAllDomainStats(msg)
	case ProcConnectDomainEventRegisterAny:
		return h.handleDomainEventRegisterAny(msg)
	case ProcConnectDomainEventDeregisterAny:
		return h.handleDomainEventDeregisterAny(msg)
	case ProcDomainMigratePrepare3Params:
		return h.handleMigratePrepare3Params(msg)
	case ProcDomainMigratePerform3Params:
		return h.handleMigratePerform3Params(msg)
	case ProcDomainMigrateFinish3Params:
		return h.handleMigrateFinish3Params(msg)
	case ProcDomainMigrateConfirm3Params:
		return h.handleMigrateConfirm3Params(msg)
	case ProcDomainMigrateGetMaxSpeed:
		return h.handleMigrateGetMaxSpeed(msg)
	case ProcDomainMigrateSetMaxSpeed:
		return h.handleMigrateSetMaxSpeed(msg)
	default:
		h.logger.Warn("unhandled procedure", "procedure", msg.Header.Procedure)
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("unhandled procedure %d", msg.Header.Procedure))
	}
}

func (h *Handler) handleAuthList(msg *Message) *Message {
	// Return auth types: [AUTH_NONE (0)]
	enc := NewXDREncoder()
	enc.WriteUint32(1)  // count of auth types
	enc.WriteUint32(0)  // AUTH_NONE = 0
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleConnectGetLibVersion(msg *Message) *Message {
	// Return libvirt version as uint64: major*1000000 + minor*1000 + micro
	// Simulate libvirt 9.0.0
	enc := NewXDREncoder()
	enc.WriteUint64(9000000)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleConnectOpen(msg *Message) *Message {
	// Request: optional_string name, uint32 flags
	// Just accept any connection
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleConnectClose(msg *Message) *Message {
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleGetHostname(msg *Message) *Message {
	enc := NewXDREncoder()
	enc.WriteString(h.hostID)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleConnectGetVersion(msg *Message) *Message {
	enc := NewXDREncoder()
	// Return libvirt version as a number, e.g., 9.0.0 = 9000000
	enc.WriteInt64(9000000)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleGetCapabilities(msg *Message) *Message {
	host, err := h.store.GetHost(h.hostID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	capsXML := fmt.Sprintf(`<capabilities>
  <host>
    <cpu>
      <arch>x86_64</arch>
      <model>%s</model>
    </cpu>
    <topology>
      <cells num='1'>
        <cell id='0'>
          <memory unit='KiB'>%d</memory>
          <cpus num='%d'/>
        </cell>
      </cells>
    </topology>
  </host>
  <guest>
    <os_type>hvm</os_type>
    <arch name='x86_64'>
      <wordsize>64</wordsize>
      <domain type='kvm'/>
    </arch>
  </guest>
</capabilities>`, host.CPUModel, host.MemoryMB*1024, host.TotalVCPUs())

	enc := NewXDREncoder()
	enc.WriteString(capsXML)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleNodeGetInfo(msg *Message) *Message {
	host, err := h.store.GetHost(h.hostID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	enc := NewXDREncoder()
	// model: [32]int8 - each char is an XDR int32 (go-libvirt convention)
	modelBytes := make([]byte, 32)
	copy(modelBytes, host.CPUModel)
	for i := 0; i < 32; i++ {
		enc.WriteInt32(int32(int8(modelBytes[i])))
	}
	// memory: uint64 (in KiB)
	enc.WriteUint64(uint64(host.MemoryMB) * 1024)
	// cpus: uint32
	enc.WriteUint32(uint32(host.TotalVCPUs()))
	// mhz: uint32
	enc.WriteUint32(2100) // Simulated MHz
	// nodes: uint32
	enc.WriteUint32(1)
	// sockets: uint32
	enc.WriteUint32(uint32(host.CPUSockets))
	// cores: uint32
	enc.WriteUint32(uint32(host.CoresPerSocket))
	// threads: uint32
	enc.WriteUint32(uint32(host.ThreadsPerCore))

	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleDomainDefineXMLFlags(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)
	xmlStr, err := dec.ReadString()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read XML: %v", err))
	}
	// flags
	_, _ = dec.ReadUint32()

	domDef, err := domxml.ParseDomainXML(xmlStr)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("parse domain XML: %v", err))
	}

	uuid, err := domxml.ParseUUID(domDef.UUID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("parse UUID: %v", err))
	}

	dom := &state.Domain{
		Name:         domDef.Name,
		UUID:         uuid,
		VCPUs:        domDef.VCPU,
		MemoryKiB:    domDef.MemoryKiB(),
		XML:          xmlStr,
		InterfaceIDs: domDef.InterfaceIDs(),
	}

	if err := h.store.DefineDomain(h.hostID, dom); err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("define domain: %v", err))
	}

	h.logger.Info("domain defined", "name", dom.Name, "uuid", dom.UUIDString())
	h.emitEvent(dom, DomainEventDefined, DomainEventDetailAdded)
	return h.domainReply(msg, dom)
}

func (h *Handler) handleDomainCreateWithFlags(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	if err := h.store.StartDomain(h.hostID, dom.UUIDString()); err != nil {
		if errors.Is(err, state.ErrOperationDenied) {
			return h.errorReply(msg, VirErrOperationDenied, err.Error())
		}
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	// Create network namespace + veth pair for VM simulation
	if err := h.netns.CreateVM(context.Background(), dom.UUIDString(), dom.InterfaceIDs); err != nil {
		h.logger.Error("failed to create VM namespace", "uuid", dom.UUIDString(), "error", err)
		// Rollback domain state
		_ = h.store.DestroyDomain(h.hostID, dom.UUIDString())
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("create namespace: %v", err))
	}

	h.logger.Info("domain started", "name", dom.Name, "uuid", dom.UUIDString())
	h.emitEvent(dom, DomainEventStarted, DomainEventDetailBooted)
	h.emitEvent(dom, DomainEventResumed, DomainEventDetailUnpaused)
	return h.domainReply(msg, dom)
}

func (h *Handler) handleDomainDestroyFlags(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	// Destroy network namespace + veth pair
	if err := h.netns.DestroyVM(context.Background(), dom.UUIDString(), dom.InterfaceIDs); err != nil {
		h.logger.Error("failed to destroy VM namespace", "uuid", dom.UUIDString(), "error", err)
	}

	if err := h.store.DestroyDomain(h.hostID, dom.UUIDString()); err != nil {
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	h.logger.Info("domain destroyed", "name", dom.Name, "uuid", dom.UUIDString())
	h.emitEvent(dom, DomainEventStopped, DomainEventDetailDestroyed)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleDomainShutdownFlags(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	if err := h.store.ShutdownDomain(h.hostID, dom.UUIDString()); err != nil {
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	h.logger.Info("domain shutdown", "name", dom.Name, "uuid", dom.UUIDString())
	h.emitEvent(dom, DomainEventStopped, DomainEventDetailShutdown)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleDomainSuspend(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	if err := h.store.SuspendDomain(h.hostID, dom.UUIDString()); err != nil {
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	h.logger.Info("domain suspended", "name", dom.Name)
	h.emitEvent(dom, DomainEventSuspended, DomainEventDetailPaused)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleDomainResume(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	if err := h.store.ResumeDomain(h.hostID, dom.UUIDString()); err != nil {
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	h.logger.Info("domain resumed", "name", dom.Name)
	h.emitEvent(dom, DomainEventResumed, DomainEventDetailUnpaused)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleDomainReboot(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	if dom.State != state.DomainStateRunning {
		return h.errorReply(msg, VirErrOperationInvalid,
			fmt.Sprintf("cannot reboot domain in state %d", dom.State))
	}

	h.logger.Info("domain rebooted", "name", dom.Name)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleDomainGetState(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	enc := NewXDREncoder()
	enc.WriteInt32(int32(dom.State))
	enc.WriteInt32(0) // reason
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleDomainGetInfo(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	enc := NewXDREncoder()
	// state: uint8 (encoded as uint32 in XDR)
	enc.WriteUint8(uint8(dom.State))
	// maxMem: uint64 (KiB)
	enc.WriteUint64(uint64(dom.MemoryKiB))
	// memory: uint64 (KiB) - same as maxMem for simulator
	enc.WriteUint64(uint64(dom.MemoryKiB))
	// nrVirtCpu: uint16 (encoded as uint32)
	enc.WriteUint16(uint16(dom.VCPUs))
	// cpuTime: uint64 (nanoseconds)
	enc.WriteUint64(0)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleDomainGetXMLDesc(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	// Skip flags after domain ref
	enc := NewXDREncoder()
	enc.WriteString(dom.XML)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleDomainLookupByUUID(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)
	uuid, err := dec.ReadUUID()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read UUID: %v", err))
	}

	// Create a temporary domain to get UUID string
	tmpDom := &state.Domain{UUID: uuid}
	dom, err := h.store.GetDomain(h.hostID, tmpDom.UUIDString())
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	return h.domainReply(msg, dom)
}

func (h *Handler) handleDomainLookupByName(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)
	name, err := dec.ReadString()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read name: %v", err))
	}

	dom, err := h.store.GetDomainByName(h.hostID, name)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	return h.domainReply(msg, dom)
}

func (h *Handler) handleDomainListAllDomains(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)
	// need_results: int32
	_, _ = dec.ReadInt32()
	// flags: uint32
	_, _ = dec.ReadUint32()

	domains, err := h.store.ListDomains(h.hostID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	enc := NewXDREncoder()
	// domains array: count + elements
	enc.WriteUint32(uint32(len(domains)))
	for _, dom := range domains {
		h.encodeDomainRef(enc, dom)
	}
	// ret: int32 (number of domains)
	enc.WriteInt32(int32(len(domains)))

	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleDomainUndefineFlags(msg *Message) *Message {
	dom, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	if err := h.store.UndefineDomain(h.hostID, dom.UUIDString()); err != nil {
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	h.logger.Info("domain undefined", "name", dom.Name, "uuid", dom.UUIDString())
	h.emitEvent(dom, DomainEventUndefined, DomainEventDetailRemoved)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleNodeGetFreeMemory(msg *Message) *Message {
	host, err := h.store.GetHost(h.hostID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	freeMemKiB := (host.AvailableMemoryMB() - host.UsedMemoryMB()) * 1024
	if freeMemKiB < 0 {
		freeMemKiB = 0
	}

	enc := NewXDREncoder()
	enc.WriteUint64(uint64(freeMemKiB) * 1024) // Return in bytes
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleNodeGetCPUStats(msg *Message) *Message {
	// Request: int32 cpuNum, int32 nparams, uint32 flags
	// Response: array of {field: string, value: uint64}, int32 nparams
	host, err := h.store.GetHost(h.hostID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	totalCPUs := host.TotalVCPUs()
	usedCPUs := host.UsedVCPUs()

	enc := NewXDREncoder()
	// params array: 4 stats
	enc.WriteUint32(4)
	// kernel
	enc.WriteString("kernel")
	enc.WriteUint64(0)
	// user
	enc.WriteString("user")
	enc.WriteUint64(uint64(usedCPUs) * 1000000000)
	// idle
	enc.WriteString("idle")
	enc.WriteUint64(uint64(totalCPUs-usedCPUs) * 1000000000)
	// iowait
	enc.WriteString("iowait")
	enc.WriteUint64(0)
	// nparams
	enc.WriteInt32(4)

	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleNodeGetMemoryStats(msg *Message) *Message {
	// Request: int32 nparams, int32 cellNum, uint32 flags
	// Response: array of {field: string, value: uint64}, int32 nparams
	host, err := h.store.GetHost(h.hostID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	totalKB := uint64(host.MemoryMB) * 1024
	usedKB := uint64(host.UsedMemoryMB()) * 1024
	freeKB := totalKB - usedKB

	enc := NewXDREncoder()
	// params array: 4 stats
	enc.WriteUint32(4)
	enc.WriteString("total")
	enc.WriteUint64(totalKB)
	enc.WriteString("free")
	enc.WriteUint64(freeKB)
	enc.WriteString("buffers")
	enc.WriteUint64(0)
	enc.WriteString("cached")
	enc.WriteUint64(0)
	// nparams
	enc.WriteInt32(4)

	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleGetAllDomainStats(msg *Message) *Message {
	domains, err := h.store.ListDomains(h.hostID)
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	enc := NewXDREncoder()
	// Return as array of domain stats records
	enc.WriteUint32(uint32(len(domains)))
	for _, dom := range domains {
		// Each record: domain ref + stats
		h.encodeDomainRef(enc, dom)
		// nparams (typed parameters count) - return 0 for simplicity
		enc.WriteInt32(0)
	}
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

// readDomainRef reads a domain reference (name, uuid, id) from the message body
// and looks up the domain in the store.
func (h *Handler) readDomainRef(body []byte) (*state.Domain, error) {
	dec := NewXDRDecoder(body)

	// Domain ref: { name: string, uuid: uuid[16], id: int32 }
	_, err := dec.ReadString() // name
	if err != nil {
		return nil, fmt.Errorf("read domain name: %w", err)
	}

	uuid, err := dec.ReadUUID()
	if err != nil {
		return nil, fmt.Errorf("read domain UUID: %w", err)
	}

	_, err = dec.ReadInt32() // id
	if err != nil {
		return nil, fmt.Errorf("read domain ID: %w", err)
	}

	tmpDom := &state.Domain{UUID: uuid}
	dom, err := h.store.GetDomain(h.hostID, tmpDom.UUIDString())
	if err != nil {
		return nil, fmt.Errorf("lookup domain: %w", err)
	}

	return dom, nil
}

// domainReply creates a reply with a domain reference.
func (h *Handler) domainReply(msg *Message, dom *state.Domain) *Message {
	enc := NewXDREncoder()
	h.encodeDomainRef(enc, dom)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

// encodeDomainRef encodes a domain reference (name, uuid, id).
func (h *Handler) encodeDomainRef(enc *XDREncoder, dom *state.Domain) {
	enc.WriteString(dom.Name)
	enc.WriteUUID(dom.UUID)
	enc.WriteInt32(dom.ID)
}

// emitEvent emits a domain lifecycle event if the event bus is configured.
func (h *Handler) emitEvent(dom *state.Domain, event int32, detail int32) {
	if h.eventBus != nil {
		h.eventBus.EmitDomainLifecycleEvent(h.hostID, dom, event, detail)
	}
}

// errorReply creates an error reply message.
func (h *Handler) errorReply(msg *Message, code int32, message string) *Message {
	enc := NewXDREncoder()
	// virNetMessageError: { code, domain, message (optional_string), level, ... }
	enc.WriteInt32(code)
	enc.WriteInt32(VirFromQemu)
	// message: optional string
	enc.WriteOptionalString(&message)
	// level: int32 (VIR_ERR_ERROR = 2)
	enc.WriteInt32(2)
	// dom: optional string (none)
	enc.WriteOptionalString(nil)
	// str1: optional string (none)
	enc.WriteOptionalString(nil)
	// str2: optional string (none)
	enc.WriteOptionalString(nil)
	// str3: optional string (none)
	enc.WriteOptionalString(nil)
	// int1: int32
	enc.WriteInt32(0)
	// int2: int32
	enc.WriteInt32(0)
	// net: optional string (none)
	enc.WriteOptionalString(nil)

	return NewReply(&msg.Header, StatusError, enc.Bytes())
}
