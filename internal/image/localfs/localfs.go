package localfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LocalFS struct {
	dir string
}

func New(dir string) *LocalFS {
	return &LocalFS{dir: dir}
}

func (l *LocalFS) Upload(ctx context.Context, id string, reader io.Reader) error {
	if err := os.MkdirAll(l.dir, 0755); err != nil {
		return fmt.Errorf("create image dir: %w", err)
	}
	path := filepath.Join(l.dir, id+".qcow2")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create image file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("write image: %w", err)
	}
	return nil
}

func (l *LocalFS) GetPath(_ context.Context, id string) (string, error) {
	path := filepath.Join(l.dir, id+".qcow2")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("image not found: %s", id)
	}
	return path, nil
}

func (l *LocalFS) Delete(_ context.Context, id string) error {
	path := filepath.Join(l.dir, id+".qcow2")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *LocalFS) EnsureLocal(ctx context.Context, id string) (string, error) {
	return l.GetPath(ctx, id)
}
