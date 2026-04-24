package fencing

import (
	"context"

	"github.com/google/uuid"
)

// FencingAgent fences (power-offs) a host to ensure it is completely stopped
// before failover proceeds. Returns nil on success, error if fencing fails or
// times out. The caller must treat a non-nil error as a fencing failure and
// abort failover (safe-mode).
type FencingAgent interface {
	Fence(ctx context.Context, hostID uuid.UUID) error
}
