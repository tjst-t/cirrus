// Package netns provides network namespace and veth pair management
// for simulating VMs using Linux network namespaces.
//
// When a VM is "started", a network namespace is created with a veth pair
// connecting it to the OVS bridge. This allows real network connectivity
// testing (ping, curl, dig) without actual QEMU/KVM.
package netns

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// Manager handles network namespace and veth pair lifecycle for simulated VMs.
type Manager interface {
	// CreateVM sets up the network namespace and veth pair for a VM.
	// It creates:
	//   - namespace: vm-{uuid}
	//   - veth pair: vm-{uuid}-tap (host side) <-> eth0 (namespace side)
	//   - OVS port: vm-{uuid}-tap on br-int with external_ids:iface-id={interfaceID}
	CreateVM(ctx context.Context, uuid string, interfaceIDs []string) error

	// DestroyVM tears down the network namespace and veth pair for a VM.
	DestroyVM(ctx context.Context, uuid string, interfaceIDs []string) error

	// MigrateOut removes namespace/veth on the source host (same as DestroyVM).
	MigrateOut(ctx context.Context, uuid string, interfaceIDs []string) error

	// MigrateIn recreates namespace/veth on the destination host (same as CreateVM).
	MigrateIn(ctx context.Context, uuid string, interfaceIDs []string) error
}

// LinuxManager implements Manager using real Linux network namespace operations.
// Requires CAP_NET_ADMIN (typically running in a privileged container).
type LinuxManager struct {
	bridge string // OVS bridge name (default: br-int)
	logger *slog.Logger
}

// NewLinuxManager creates a new LinuxManager.
func NewLinuxManager(bridge string, logger *slog.Logger) *LinuxManager {
	if bridge == "" {
		bridge = "br-int"
	}
	return &LinuxManager{bridge: bridge, logger: logger}
}

func (m *LinuxManager) CreateVM(ctx context.Context, uuid string, interfaceIDs []string) error {
	nsName := nsName(uuid)
	tapName := tapName(uuid)

	m.logger.Info("creating VM network namespace", "uuid", uuid, "ns", nsName)

	// 1. Create network namespace
	if err := run(ctx, "ip", "netns", "add", nsName); err != nil {
		return fmt.Errorf("create namespace %s: %w", nsName, err)
	}

	// 2. Create veth pair: tap (host) <-> eth0 (ns)
	if err := run(ctx, "ip", "link", "add", tapName, "type", "veth", "peer", "name", "eth0", "netns", nsName); err != nil {
		_ = run(ctx, "ip", "netns", "delete", nsName)
		return fmt.Errorf("create veth pair: %w", err)
	}

	// 3. Bring up host side
	if err := run(ctx, "ip", "link", "set", tapName, "up"); err != nil {
		_ = m.cleanup(ctx, uuid, len(interfaceIDs))
		return fmt.Errorf("bring up tap: %w", err)
	}

	// 4. Bring up loopback and eth0 inside namespace
	if err := run(ctx, "ip", "netns", "exec", nsName, "ip", "link", "set", "lo", "up"); err != nil {
		_ = m.cleanup(ctx, uuid, len(interfaceIDs))
		return fmt.Errorf("bring up loopback: %w", err)
	}
	if err := run(ctx, "ip", "netns", "exec", nsName, "ip", "link", "set", "eth0", "up"); err != nil {
		_ = m.cleanup(ctx, uuid, len(interfaceIDs))
		return fmt.Errorf("bring up eth0: %w", err)
	}

	// 5. Add tap to OVS bridge with external_ids
	ovsArgs := []string{"add-port", m.bridge, tapName}
	if len(interfaceIDs) > 0 {
		ovsArgs = append(ovsArgs, "--", "set", "Interface", tapName,
			fmt.Sprintf("external_ids:iface-id=%s", interfaceIDs[0]))
	}
	if err := run(ctx, "ovs-vsctl", ovsArgs...); err != nil {
		_ = m.cleanup(ctx, uuid, len(interfaceIDs))
		return fmt.Errorf("add OVS port: %w", err)
	}

	// Set additional interface IDs if multiple NICs
	for i := 1; i < len(interfaceIDs); i++ {
		extraTap := fmt.Sprintf("vm-%s-tap%d", shortUUID(uuid), i)
		extraEth := fmt.Sprintf("eth%d", i)

		if err := run(ctx, "ip", "link", "add", extraTap, "type", "veth", "peer", "name", extraEth, "netns", nsName); err != nil {
			m.logger.Error("failed to create extra veth pair", "uuid", uuid, "nic", i, "error", err)
			_ = m.cleanup(ctx, uuid, len(interfaceIDs))
			return fmt.Errorf("create extra veth pair %d: %w", i, err)
		}
		if err := run(ctx, "ip", "link", "set", extraTap, "up"); err != nil {
			_ = m.cleanup(ctx, uuid, len(interfaceIDs))
			return fmt.Errorf("bring up extra tap %d: %w", i, err)
		}
		if err := run(ctx, "ip", "netns", "exec", nsName, "ip", "link", "set", extraEth, "up"); err != nil {
			_ = m.cleanup(ctx, uuid, len(interfaceIDs))
			return fmt.Errorf("bring up extra eth %d: %w", i, err)
		}
		if err := run(ctx, "ovs-vsctl", "add-port", m.bridge, extraTap,
			"--", "set", "Interface", extraTap,
			fmt.Sprintf("external_ids:iface-id=%s", interfaceIDs[i])); err != nil {
			_ = m.cleanup(ctx, uuid, len(interfaceIDs))
			return fmt.Errorf("add extra OVS port %d: %w", i, err)
		}
	}

	m.logger.Info("VM network namespace created", "uuid", uuid, "interfaces", len(interfaceIDs))
	return nil
}

func (m *LinuxManager) DestroyVM(ctx context.Context, uuid string, interfaceIDs []string) error {
	m.logger.Info("destroying VM network namespace", "uuid", uuid)
	return m.cleanup(ctx, uuid, len(interfaceIDs))
}

func (m *LinuxManager) MigrateOut(ctx context.Context, uuid string, interfaceIDs []string) error {
	return m.DestroyVM(ctx, uuid, interfaceIDs)
}

func (m *LinuxManager) MigrateIn(ctx context.Context, uuid string, interfaceIDs []string) error {
	return m.CreateVM(ctx, uuid, interfaceIDs)
}

func (m *LinuxManager) cleanup(ctx context.Context, uuid string, nicCount int) error {
	nsName := nsName(uuid)
	tapName := tapName(uuid)

	// Remove OVS port (ignore errors - may not exist)
	_ = run(ctx, "ovs-vsctl", "--if-exists", "del-port", m.bridge, tapName)

	// Remove extra tap ports
	for i := 1; i < nicCount; i++ {
		extraTap := fmt.Sprintf("vm-%s-tap%d", shortUUID(uuid), i)
		_ = run(ctx, "ovs-vsctl", "--if-exists", "del-port", m.bridge, extraTap)
	}

	// Delete veth (deleting one side removes the pair)
	_ = run(ctx, "ip", "link", "delete", tapName)

	// Delete namespace
	_ = run(ctx, "ip", "netns", "delete", nsName)

	return nil
}

// NoopManager is a no-op implementation for testing without privileges.
type NoopManager struct {
	logger *slog.Logger
}

// NewNoopManager creates a Manager that logs but doesn't execute namespace operations.
func NewNoopManager(logger *slog.Logger) *NoopManager {
	return &NoopManager{logger: logger}
}

func (m *NoopManager) CreateVM(_ context.Context, uuid string, interfaceIDs []string) error {
	m.logger.Debug("noop: create VM namespace", "uuid", uuid, "interfaces", len(interfaceIDs))
	return nil
}

func (m *NoopManager) DestroyVM(_ context.Context, uuid string, _ []string) error {
	m.logger.Debug("noop: destroy VM namespace", "uuid", uuid)
	return nil
}

func (m *NoopManager) MigrateOut(_ context.Context, uuid string, _ []string) error {
	m.logger.Debug("noop: migrate out VM namespace", "uuid", uuid)
	return nil
}

func (m *NoopManager) MigrateIn(_ context.Context, uuid string, interfaceIDs []string) error {
	m.logger.Debug("noop: migrate in VM namespace", "uuid", uuid, "interfaces", len(interfaceIDs))
	return nil
}

// shortUUID returns first 8 chars of the UUID for naming.
func shortUUID(uuid string) string {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) > 8 {
		return clean[:8]
	}
	return clean
}

func nsName(uuid string) string {
	return fmt.Sprintf("vm-%s", shortUUID(uuid))
}

func tapName(uuid string) string {
	return fmt.Sprintf("vm-%s-tap", shortUUID(uuid))
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w (output: %s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
