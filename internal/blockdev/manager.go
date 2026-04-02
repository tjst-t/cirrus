// Package blockdev handles volume attachment on the worker side.
// It interprets ExportInfo from the storage driver and issues the appropriate
// host-level commands to attach/detach the block device.
package blockdev

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/tjst-t/cirrus/internal/storage"
)

// ErrUnsupportedProtocol is returned when the ExportInfo protocol is not supported.
var ErrUnsupportedProtocol = errors.New("blockdev: unsupported protocol")

// AttachResult contains information about a successfully attached block device.
type AttachResult struct {
	// DevicePath is the local block device path (e.g. /dev/sdb, /dev/nbd0).
	DevicePath string
}

// Manager attaches and detaches block devices on the worker host.
type Manager interface {
	// Attach makes the volume available as a block device on this host.
	Attach(ctx context.Context, info *storage.ExportInfo) (*AttachResult, error)
	// Detach removes the block device attachment.
	Detach(ctx context.Context, info *storage.ExportInfo) error
}

// DefaultManager implements Manager using OS commands.
type DefaultManager struct {
	logger *slog.Logger
}

// New creates a new DefaultManager.
func New(logger *slog.Logger) *DefaultManager {
	return &DefaultManager{logger: logger}
}

// Attach connects the volume described by ExportInfo to a local block device.
func (m *DefaultManager) Attach(ctx context.Context, info *storage.ExportInfo) (*AttachResult, error) {
	switch info.Protocol {
	case "rbd":
		return m.attachRBD(ctx, info)
	case "iscsi":
		return m.attachISCSI(ctx, info)
	case "sim":
		// Simulator: no-op, return a fake device path.
		return &AttachResult{DevicePath: "/dev/null"}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, info.Protocol)
	}
}

// Detach disconnects the volume described by ExportInfo.
func (m *DefaultManager) Detach(ctx context.Context, info *storage.ExportInfo) error {
	switch info.Protocol {
	case "rbd":
		return m.detachRBD(ctx, info)
	case "iscsi":
		return m.detachISCSI(ctx, info)
	case "sim":
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProtocol, info.Protocol)
	}
}

// --- RBD (Ceph) ---

// attachRBD maps an RBD image using `rbd device map`.
// Required params: monitor, pool, image, client_id, keyring (optional).
func (m *DefaultManager) attachRBD(ctx context.Context, info *storage.ExportInfo) (*AttachResult, error) {
	monitor := info.Params["monitor"]
	pool := info.Params["pool"]
	image := info.Params["image"]
	clientID := info.Params["client_id"]
	keyring := info.Params["keyring"]

	if monitor == "" || pool == "" || image == "" {
		return nil, fmt.Errorf("blockdev: rbd attach: missing params (monitor=%s pool=%s image=%s)", monitor, pool, image)
	}

	args := []string{"device", "map", "--pool", pool, "--id", clientID}
	if keyring != "" {
		args = append(args, "--keyfile", keyring)
	}
	args = append(args, image)

	out, err := exec.CommandContext(ctx, "rbd", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("blockdev: rbd device map: %w", err)
	}
	devPath := strings.TrimSpace(string(out))
	m.logger.Info("rbd device mapped", "device", devPath, "image", image)
	return &AttachResult{DevicePath: devPath}, nil
}

// detachRBD unmaps an RBD device using `rbd device unmap`.
func (m *DefaultManager) detachRBD(ctx context.Context, info *storage.ExportInfo) error {
	pool := info.Params["pool"]
	image := info.Params["image"]
	clientID := info.Params["client_id"]

	if pool == "" || image == "" {
		return fmt.Errorf("blockdev: rbd detach: missing params")
	}

	args := []string{"device", "unmap", "--pool", pool, "--id", clientID, image}
	if err := exec.CommandContext(ctx, "rbd", args...).Run(); err != nil {
		return fmt.Errorf("blockdev: rbd device unmap: %w", err)
	}
	m.logger.Info("rbd device unmapped", "image", image)
	return nil
}

// --- iSCSI ---

// attachISCSI connects to an iSCSI target using iscsiadm and returns the device path.
// Required params: target (IQN), portal (host:port), lun.
func (m *DefaultManager) attachISCSI(ctx context.Context, info *storage.ExportInfo) (*AttachResult, error) {
	target := info.Params["target"]
	portal := info.Params["portal"]

	if target == "" || portal == "" {
		return nil, fmt.Errorf("blockdev: iscsi attach: missing params (target=%s portal=%s)", target, portal)
	}

	// Discovery
	if out, err := exec.CommandContext(ctx, "iscsiadm", "-m", "discovery", "-t", "sendtargets", "-p", portal).Output(); err != nil {
		m.logger.Warn("iscsi discovery failed", "portal", portal, "error", err, "output", string(out))
	}

	// Login
	if err := exec.CommandContext(ctx, "iscsiadm", "-m", "node", "-T", target, "-p", portal, "--login").Run(); err != nil {
		return nil, fmt.Errorf("blockdev: iscsiadm login: %w", err)
	}
	m.logger.Info("iscsi session connected", "target", target)

	// Return a placeholder device; real implementation would parse /dev/disk/by-path
	return &AttachResult{DevicePath: "/dev/disk/by-path/ip-" + portal + "-iscsi-" + target + "-lun-1"}, nil
}

// detachISCSI logs out of an iSCSI target using iscsiadm.
func (m *DefaultManager) detachISCSI(ctx context.Context, info *storage.ExportInfo) error {
	target := info.Params["target"]
	portal := info.Params["portal"]

	if target == "" || portal == "" {
		return fmt.Errorf("blockdev: iscsi detach: missing params")
	}

	if err := exec.CommandContext(ctx, "iscsiadm", "-m", "node", "-T", target, "-p", portal, "--logout").Run(); err != nil {
		return fmt.Errorf("blockdev: iscsiadm logout: %w", err)
	}
	m.logger.Info("iscsi session disconnected", "target", target)
	return nil
}
