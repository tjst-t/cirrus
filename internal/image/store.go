package image

import (
	"context"
	"io"
)

// Store abstracts image storage operations.
type Store interface {
	Upload(ctx context.Context, id string, reader io.Reader) error
	GetPath(ctx context.Context, id string) (string, error)
	Delete(ctx context.Context, id string) error
	EnsureLocal(ctx context.Context, id string) (string, error)
}
