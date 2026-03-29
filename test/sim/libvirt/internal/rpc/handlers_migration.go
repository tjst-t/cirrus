package rpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

func (h *Handler) handleMigratePrepare3Params(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)

	// Read typed parameters array
	// params: array of remote_typed_param
	nparams, err := dec.ReadUint32()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read nparams: %v", err))
	}

	var domName, domUUID, destURI string
	for i := uint32(0); i < nparams; i++ {
		field, err := dec.ReadString()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param name: %v", err))
		}
		// type: int32
		paramType, err := dec.ReadInt32()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param type: %v", err))
		}
		switch paramType {
		case 7: // VIR_TYPED_PARAM_STRING
			val, err := dec.ReadString()
			if err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param string value: %v", err))
			}
			switch field {
			case "domain_name":
				domName = val
			case "domain_uuid":
				domUUID = val
			case "dest_uri":
				destURI = val
			}
		default:
			// Skip 8 bytes for other typed param values
			if _, err := dec.ReadUint64(); err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("skip param value: %v", err))
			}
		}
	}

	_ = destURI

	// Look up the domain by name or UUID on any host to get its resource spec
	var sourceDom *state.Domain
	if domUUID != "" {
		// Try to find the domain across all hosts
		for _, host := range h.store.ListHosts() {
			d, err := h.store.GetDomain(host.HostID, domUUID)
			if err == nil {
				sourceDom = d
				break
			}
		}
	}
	if sourceDom == nil && domName != "" {
		for _, host := range h.store.ListHosts() {
			d, err := h.store.GetDomainByName(host.HostID, domName)
			if err == nil {
				sourceDom = d
				break
			}
		}
	}

	if sourceDom == nil {
		return h.errorReply(msg, VirErrNoDomain, "source domain not found for migration")
	}

	// Simulate prepare delay
	cfg := h.store.GetMigrationConfig()
	if cfg.PrepareDurationMs > 0 {
		time.Sleep(time.Duration(cfg.PrepareDurationMs) * time.Millisecond)
	}

	// Reserve resources on this (destination) host
	if err := h.store.MigratePrepare(h.hostID, sourceDom); err != nil {
		if errors.Is(err, state.ErrOperationDenied) {
			return h.errorReply(msg, VirErrOperationDenied, err.Error())
		}
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	h.logger.Info("migration prepared", "domain", sourceDom.Name, "dest_host", h.hostID)

	// Return cookie data and optional URI
	enc := NewXDREncoder()
	// cookie_out: opaque (length-prefixed)
	cookie := []byte(sourceDom.UUIDString())
	enc.WriteUint32(uint32(len(cookie)))
	enc.buf = append(enc.buf, cookie...)
	// Pad cookie to 4-byte boundary
	pad := (4 - len(cookie)%4) % 4
	for i := 0; i < pad; i++ {
		enc.buf = append(enc.buf, 0)
	}
	// uri_out: optional string (none)
	enc.WriteOptionalString(nil)

	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleMigratePerform3Params(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)

	// Read typed parameters array
	nparams, err := dec.ReadUint32()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read nparams: %v", err))
	}

	var domUUID string
	for i := uint32(0); i < nparams; i++ {
		field, err := dec.ReadString()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param name: %v", err))
		}
		paramType, err := dec.ReadInt32()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param type: %v", err))
		}
		switch paramType {
		case 7: // VIR_TYPED_PARAM_STRING
			val, err := dec.ReadString()
			if err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param string value: %v", err))
			}
			if field == "domain_uuid" {
				domUUID = val
			}
		default:
			if _, err := dec.ReadUint64(); err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("skip param value: %v", err))
			}
		}
	}

	if domUUID == "" {
		return h.errorReply(msg, VirErrInternalError, "domain_uuid parameter required")
	}

	// Mark domain as migrating on source
	if err := h.store.MigratePerform(h.hostID, domUUID); err != nil {
		if errors.Is(err, state.ErrNoDomain) {
			return h.errorReply(msg, VirErrNoDomain, err.Error())
		}
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		if errors.Is(err, state.ErrMigrationInProgress) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	// Simulate transfer delay based on memory size
	cfg := h.store.GetMigrationConfig()
	dom, _ := h.store.GetDomain(h.hostID, domUUID)
	delay := time.Duration(cfg.BaseTransferDurationMs) * time.Millisecond
	if dom != nil {
		memGB := dom.MemoryKiB / (1024 * 1024)
		if memGB < 1 {
			memGB = 1
		}
		delay += time.Duration(cfg.PerGBMemoryMs*memGB) * time.Millisecond
	}
	if delay > 0 {
		time.Sleep(delay)
	}

	// Tear down namespace/veth on source (migrate out)
	if dom != nil {
		if err := h.netns.MigrateOut(context.Background(), domUUID, dom.InterfaceIDs); err != nil {
			h.logger.Error("failed to migrate out VM namespace", "uuid", domUUID, "error", err)
		}
	}

	// Emit suspended event with migration detail
	if h.eventBus != nil && dom != nil {
		h.eventBus.EmitDomainLifecycleEvent(h.hostID, dom, DomainEventSuspended, DomainEventDetailMigrated)
	}

	h.logger.Info("migration perform complete", "domain_uuid", domUUID, "src_host", h.hostID)

	// Return cookie
	enc := NewXDREncoder()
	cookie := []byte(domUUID)
	enc.WriteUint32(uint32(len(cookie)))
	enc.buf = append(enc.buf, cookie...)
	pad := (4 - len(cookie)%4) % 4
	for i := 0; i < pad; i++ {
		enc.buf = append(enc.buf, 0)
	}

	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleMigrateFinish3Params(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)

	// Read typed parameters array
	nparams, err := dec.ReadUint32()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read nparams: %v", err))
	}

	var domUUID string
	for i := uint32(0); i < nparams; i++ {
		field, err := dec.ReadString()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param name: %v", err))
		}
		paramType, err := dec.ReadInt32()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param type: %v", err))
		}
		switch paramType {
		case 7:
			val, err := dec.ReadString()
			if err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param string value: %v", err))
			}
			if field == "domain_uuid" {
				domUUID = val
			}
		default:
			if _, err := dec.ReadUint64(); err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("skip param value: %v", err))
			}
		}
	}

	if domUUID == "" {
		return h.errorReply(msg, VirErrInternalError, "domain_uuid parameter required")
	}

	// Simulate finish delay
	cfg := h.store.GetMigrationConfig()
	if cfg.FinishDurationMs > 0 {
		time.Sleep(time.Duration(cfg.FinishDurationMs) * time.Millisecond)
	}

	// Activate domain on destination
	dom, err := h.store.MigrateFinish(h.hostID, domUUID)
	if err != nil {
		if errors.Is(err, state.ErrNoDomain) {
			return h.errorReply(msg, VirErrNoDomain, err.Error())
		}
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	// Recreate namespace/veth on destination (migrate in)
	if err := h.netns.MigrateIn(context.Background(), domUUID, dom.InterfaceIDs); err != nil {
		h.logger.Error("failed to migrate in VM namespace", "uuid", domUUID, "error", err)
	}

	// Emit started event with migration detail on destination
	if h.eventBus != nil {
		h.eventBus.EmitDomainLifecycleEvent(h.hostID, dom, DomainEventStarted, DomainEventDetailMigrated)
		h.eventBus.EmitDomainLifecycleEvent(h.hostID, dom, DomainEventResumed, DomainEventDetailMigrated)
	}

	h.logger.Info("migration finished", "domain", dom.Name, "dest_host", h.hostID)
	return h.domainReply(msg, dom)
}

func (h *Handler) handleMigrateConfirm3Params(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)

	// Read typed parameters array
	nparams, err := dec.ReadUint32()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read nparams: %v", err))
	}

	var domUUID string
	for i := uint32(0); i < nparams; i++ {
		field, err := dec.ReadString()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param name: %v", err))
		}
		paramType, err := dec.ReadInt32()
		if err != nil {
			return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param type: %v", err))
		}
		switch paramType {
		case 7:
			val, err := dec.ReadString()
			if err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read param string value: %v", err))
			}
			if field == "domain_uuid" {
				domUUID = val
			}
		default:
			if _, err := dec.ReadUint64(); err != nil {
				return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("skip param value: %v", err))
			}
		}
	}

	if domUUID == "" {
		return h.errorReply(msg, VirErrInternalError, "domain_uuid parameter required")
	}

	// Get domain info before removing for event emission
	dom, _ := h.store.GetDomain(h.hostID, domUUID)

	// Remove domain from source
	if err := h.store.MigrateConfirm(h.hostID, domUUID); err != nil {
		if errors.Is(err, state.ErrNoDomain) {
			return h.errorReply(msg, VirErrNoDomain, err.Error())
		}
		if errors.Is(err, state.ErrOperationInvalid) {
			return h.errorReply(msg, VirErrOperationInvalid, err.Error())
		}
		return h.errorReply(msg, VirErrInternalError, err.Error())
	}

	// Emit stopped event on source
	if h.eventBus != nil && dom != nil {
		h.eventBus.EmitDomainLifecycleEvent(h.hostID, dom, DomainEventStopped, DomainEventDetailMigrated)
	}

	h.logger.Info("migration confirmed", "domain_uuid", domUUID, "src_host", h.hostID)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleMigrateGetMaxSpeed(msg *Message) *Message {
	// Read and ignore domain ref
	_, err := h.readDomainRef(msg.Body)
	if err != nil {
		return h.errorReply(msg, VirErrNoDomain, err.Error())
	}

	speed := h.store.GetMigrationSpeed()

	enc := NewXDREncoder()
	enc.WriteUint64(speed)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleMigrateSetMaxSpeed(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)

	// Domain ref
	_, err := dec.ReadString() // name
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read domain name: %v", err))
	}
	_, err = dec.ReadUUID()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read domain UUID: %v", err))
	}
	_, err = dec.ReadInt32() // id
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read domain ID: %v", err))
	}

	// bandwidth: uint64
	bandwidth, err := dec.ReadUint64()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read bandwidth: %v", err))
	}

	// flags: uint32
	_, _ = dec.ReadUint32()

	h.store.SetMigrationSpeed(bandwidth)
	h.logger.Info("migration max speed set", "bandwidth", bandwidth)
	return NewReply(&msg.Header, StatusOK, nil)
}

func (h *Handler) handleDomainEventRegisterAny(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)

	// eventID: int32
	eventID, err := dec.ReadInt32()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read eventID: %v", err))
	}

	// domain: optional domain ref (we ignore it for now, register for all domains)
	// The client sends an optional domain - discriminant + possibly domain ref
	// We just accept any event registration.

	if h.clientEvents == nil {
		return h.errorReply(msg, VirErrInternalError, "event system not initialized")
	}

	cbID := h.clientEvents.Register(eventID)
	h.logger.Info("domain event registered", "event_id", eventID, "callback_id", cbID)

	enc := NewXDREncoder()
	enc.WriteInt32(cbID)
	return NewReply(&msg.Header, StatusOK, enc.Bytes())
}

func (h *Handler) handleDomainEventDeregisterAny(msg *Message) *Message {
	dec := NewXDRDecoder(msg.Body)

	// eventID: int32 (go-libvirt sends eventID, not callbackID)
	eventID, err := dec.ReadInt32()
	if err != nil {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("read eventID: %v", err))
	}

	if h.clientEvents == nil {
		return h.errorReply(msg, VirErrInternalError, "event system not initialized")
	}

	if !h.clientEvents.DeregisterByEventID(eventID) {
		return h.errorReply(msg, VirErrInternalError, fmt.Sprintf("event %d not registered", eventID))
	}

	h.logger.Info("domain event deregistered", "event_id", eventID)
	return NewReply(&msg.Header, StatusOK, nil)
}
