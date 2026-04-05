package rpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/tjst-t/cirrus/test/sim/common/pkg/fault"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/netns"
	"github.com/tjst-t/cirrus/test/sim/libvirt/internal/state"
)

// Server manages TCP listeners for libvirt RPC connections.
type Server struct {
	store     *state.Store
	logger    *slog.Logger
	mu        sync.Mutex
	listeners map[string]net.Listener // key: hostID
	wg        sync.WaitGroup
	eventBus    *EventBus
	netns       netns.Manager
	faultEngine *fault.Engine
}

// NewServer creates a new RPC server.
func NewServer(store *state.Store, logger *slog.Logger) *Server {
	return &Server{
		store:     store,
		logger:    logger,
		listeners: make(map[string]net.Listener),
		eventBus:  NewEventBus(),
		netns:     netns.NewNoopManager(logger),
	}
}

// SetNetnsManager sets the network namespace manager for all handlers.
func (s *Server) SetNetnsManager(m netns.Manager) {
	s.netns = m
}

// CreateVMNetns creates the network namespace and veth pair for a VM.
// Called by the management handler when a domain is defined+started via the HTTP API.
func (s *Server) CreateVMNetns(ctx context.Context, uuid string, interfaceIDs []string) error {
	return s.netns.CreateVM(ctx, uuid, interfaceIDs)
}

// DestroyVMNetns tears down the network namespace and veth pair for a VM.
// Called by the management handler when a domain is undefined via the HTTP API.
func (s *Server) DestroyVMNetns(ctx context.Context, uuid string, interfaceIDs []string) error {
	return s.netns.DestroyVM(ctx, uuid, interfaceIDs)
}

// SetFaultEngine sets the fault injection engine for RPC-level fault injection.
func (s *Server) SetFaultEngine(e *fault.Engine) {
	s.faultEngine = e
}

// EventBusRef returns the server's event bus.
func (s *Server) EventBusRef() *EventBus {
	return s.eventBus
}

// StartListener starts a TCP listener for a host on the given port.
func (s *Server) StartListener(ctx context.Context, hostID string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.listeners[hostID]; exists {
		return fmt.Errorf("listener already exists for host %s", hostID)
	}

	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	s.listeners[hostID] = ln
	s.logger.Info("started libvirt listener", "host_id", hostID, "port", port)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx, ln, hostID)
	}()

	return nil
}

// StopListener stops the TCP listener for a host.
func (s *Server) StopListener(hostID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ln, exists := s.listeners[hostID]
	if !exists {
		return fmt.Errorf("no listener for host %s", hostID)
	}

	delete(s.listeners, hostID)
	return ln.Close()
}

// StopAll stops all listeners.
func (s *Server) StopAll() {
	s.mu.Lock()
	listeners := make(map[string]net.Listener)
	for k, v := range s.listeners {
		listeners[k] = v
	}
	s.listeners = make(map[string]net.Listener)
	s.mu.Unlock()

	for hostID, ln := range listeners {
		s.logger.Info("stopping listener", "host_id", hostID)
		ln.Close()
	}
}

// Wait waits for all goroutines to finish.
func (s *Server) Wait() {
	s.wg.Wait()
}

func (s *Server) acceptLoop(ctx context.Context, ln net.Listener, hostID string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Check if listener was closed
			select {
			case <-ctx.Done():
				return
			default:
			}
			// Check if the error is due to closed listener
			if opErr, ok := err.(*net.OpError); ok && !opErr.Temporary() {
				s.logger.Debug("listener closed", "host_id", hostID)
				return
			}
			s.logger.Error("accept failed", "host_id", hostID, "error", err)
			return
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn, hostID)
		}()
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn, hostID string) {
	defer conn.Close()

	handler := NewHandler(s.store, hostID, s.logger.With("host_id", hostID, "remote", conn.RemoteAddr()))
	handler.SetEventBus(s.eventBus)
	handler.SetNetnsManager(s.netns)
	if s.faultEngine != nil {
		handler.SetFaultEngine(s.faultEngine)
	}
	clientEvents := s.eventBus.RegisterClient(conn, hostID)
	handler.SetClientEvents(clientEvents)
	defer s.eventBus.UnregisterClient(conn)
	s.logger.Debug("new connection", "host_id", hostID, "remote", conn.RemoteAddr())

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := ReadMessage(conn)
		if err != nil {
			// Connection closed or read error
			s.logger.Debug("connection closed", "host_id", hostID, "error", err)
			return
		}

		reply := handler.HandleMessage(msg)
		if reply == nil {
			continue
		}

		if err := WriteMessage(conn, reply); err != nil {
			s.logger.Error("write reply failed", "host_id", hostID, "error", err)
			return
		}

		// Close connection after CONNECT_CLOSE
		if msg.Header.Procedure == ProcConnectClose {
			return
		}
	}
}
