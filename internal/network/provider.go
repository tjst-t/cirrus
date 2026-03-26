package network

import "context"

// Provider abstracts network operations on a worker node.
type Provider interface {
	InitBridge(ctx context.Context) error
	AddTunnel(ctx context.Context, peerAddr string, peerName string) error
	RemoveTunnel(ctx context.Context, peerAddr string, peerName string) error
	AttachPort(ctx context.Context, port PortConfig) error
	DetachPort(ctx context.Context, portID string) error
	EnsureFlows(ctx context.Context, vni int, localTag int) error
	RemoveFlows(ctx context.Context, vni int) error
	ListPorts(ctx context.Context) ([]PortConfig, error)
}

type PortConfig struct {
	ID       string
	MAC      string
	VNI      int
	LocalTag int
}
