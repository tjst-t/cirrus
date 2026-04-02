package controller

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tjst-t/cirrus/proto/agentpb"
)

// WorkerClient wraps the gRPC WorkerServiceClient for a single worker host.
type WorkerClient struct {
	conn   *grpc.ClientConn
	client pb.WorkerServiceClient
}

// NewWorkerClient dials the worker's gRPC address and returns a WorkerClient.
func NewWorkerClient(addr string) (*WorkerClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("worker client: dial %s: %w", addr, err)
	}
	return &WorkerClient{conn: conn, client: pb.NewWorkerServiceClient(conn)}, nil
}

// CreateVM calls CreateVM on the worker.
func (c *WorkerClient) CreateVM(ctx context.Context, req *pb.CreateVMRequest) (*pb.CreateVMResponse, error) {
	return c.client.CreateVM(ctx, req)
}

// DeleteVM calls DeleteVM on the worker.
func (c *WorkerClient) DeleteVM(ctx context.Context, req *pb.DeleteVMRequest) (*pb.DeleteVMResponse, error) {
	return c.client.DeleteVM(ctx, req)
}

// StartVM calls StartVM on the worker.
func (c *WorkerClient) StartVM(ctx context.Context, req *pb.StartVMRequest) (*pb.StartVMResponse, error) {
	return c.client.StartVM(ctx, req)
}

// StopVM calls StopVM on the worker.
func (c *WorkerClient) StopVM(ctx context.Context, req *pb.StopVMRequest) (*pb.StopVMResponse, error) {
	return c.client.StopVM(ctx, req)
}

// ForceStopVM calls ForceStopVM on the worker.
func (c *WorkerClient) ForceStopVM(ctx context.Context, req *pb.ForceStopVMRequest) (*pb.ForceStopVMResponse, error) {
	return c.client.ForceStopVM(ctx, req)
}

// RebootVM calls RebootVM on the worker.
func (c *WorkerClient) RebootVM(ctx context.Context, req *pb.RebootVMRequest) (*pb.RebootVMResponse, error) {
	return c.client.RebootVM(ctx, req)
}

// Close closes the gRPC connection.
func (c *WorkerClient) Close() error {
	return c.conn.Close()
}

// WorkerClientPool maintains a pool of WorkerClients keyed by host address.
// It is safe for concurrent use.
type WorkerClientPool struct {
	mu      sync.Mutex
	clients map[string]*WorkerClient
}

// NewWorkerClientPool creates an empty WorkerClientPool.
func NewWorkerClientPool() *WorkerClientPool {
	return &WorkerClientPool{clients: make(map[string]*WorkerClient)}
}

// Get returns a WorkerClient for the given address, creating one if needed.
func (p *WorkerClientPool) Get(addr string) (*WorkerClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[addr]; ok {
		return c, nil
	}
	c, err := NewWorkerClient(addr)
	if err != nil {
		return nil, err
	}
	p.clients[addr] = c
	return c, nil
}

// Close closes all pooled connections.
func (p *WorkerClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.clients {
		c.Close()
	}
	p.clients = make(map[string]*WorkerClient)
}
