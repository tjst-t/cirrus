package hypervisor

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// CheckLibvirtConnection verifies TCP connectivity to a libvirt endpoint.
// uri format: "tcp://host:port"
func CheckLibvirtConnection(ctx context.Context, uri string) error {
	addr := strings.TrimPrefix(uri, "tcp://")

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("hypervisor: libvirt connection check %s: %w", addr, err)
	}
	conn.Close()
	return nil
}
