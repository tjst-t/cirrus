package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tjst-t/cirrus/internal/storage"
)

type Local struct {
	diskDir  string
	imageDir string
}

func New(diskDir, imageDir string) *Local {
	return &Local{diskDir: diskDir, imageDir: imageDir}
}

func (l *Local) CreateDisk(ctx context.Context, vmID string, baseImage string, sizeGB int) (*storage.DiskResult, error) {
	if err := os.MkdirAll(l.diskDir, 0755); err != nil {
		return nil, fmt.Errorf("create disk dir: %w", err)
	}

	path := filepath.Join(l.diskDir, vmID+".qcow2")
	cmd := exec.CommandContext(ctx, "qemu-img", "create",
		"-b", baseImage, "-F", "qcow2", "-f", "qcow2",
		path, fmt.Sprintf("%dG", sizeGB))
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("qemu-img create: %s: %w", string(out), err)
	}

	return &storage.DiskResult{DriverData: nil}, nil
}

func (l *Local) DeleteDisk(ctx context.Context, vmID string, _ json.RawMessage) error {
	path := filepath.Join(l.diskDir, vmID+".qcow2")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove disk: %w", err)
	}
	// Also remove cloud-init ISO if present
	ciPath := filepath.Join(l.diskDir, vmID+"-cidata.iso")
	os.Remove(ciPath)
	return nil
}

func (l *Local) DiskSpec(vmID string, _ json.RawMessage) storage.LibvirtDiskSpec {
	return storage.LibvirtDiskSpec{
		Type:   "file",
		Source: filepath.Join(l.diskDir, vmID+".qcow2"),
		Format: "qcow2",
	}
}
