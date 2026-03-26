package stub

import (
	"context"
	"log/slog"
	"sync"

	"github.com/tjst-t/cirrus/internal/network"
)

type Provider struct {
	mu      sync.Mutex
	ports   map[string]network.PortConfig
	tunnels map[string]string // peerAddr -> peerName
	log     *slog.Logger
}

func New(log *slog.Logger) *Provider {
	return &Provider{
		ports:   make(map[string]network.PortConfig),
		tunnels: make(map[string]string),
		log:     log,
	}
}

func (p *Provider) InitBridge(_ context.Context) error {
	p.log.Info("[stub/network] InitBridge")
	return nil
}

func (p *Provider) AddTunnel(_ context.Context, peerAddr string, peerName string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tunnels[peerAddr] = peerName
	p.log.Info("[stub/network] AddTunnel", "peer_addr", peerAddr, "peer_name", peerName)
	return nil
}

func (p *Provider) RemoveTunnel(_ context.Context, peerAddr string, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.tunnels, peerAddr)
	p.log.Info("[stub/network] RemoveTunnel", "peer_addr", peerAddr)
	return nil
}

func (p *Provider) AttachPort(_ context.Context, port network.PortConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ports[port.ID] = port
	p.log.Info("[stub/network] AttachPort", "port_id", port.ID, "mac", port.MAC, "vni", port.VNI)
	return nil
}

func (p *Provider) DetachPort(_ context.Context, portID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.ports, portID)
	p.log.Info("[stub/network] DetachPort", "port_id", portID)
	return nil
}

func (p *Provider) EnsureFlows(_ context.Context, vni int, localTag int) error {
	p.log.Info("[stub/network] EnsureFlows", "vni", vni, "local_tag", localTag)
	return nil
}

func (p *Provider) RemoveFlows(_ context.Context, vni int) error {
	p.log.Info("[stub/network] RemoveFlows", "vni", vni)
	return nil
}

func (p *Provider) ListPorts(_ context.Context) ([]network.PortConfig, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var result []network.PortConfig
	for _, port := range p.ports {
		result = append(result, port)
	}
	return result, nil
}
