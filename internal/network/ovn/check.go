package ovn

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// CheckConnection verifies TCP connectivity to an OVN Northbound DB endpoint.
// addr format: "tcp:host:port"
func CheckConnection(ctx context.Context, addr string) error {
	// OVN uses "tcp:host:port" format
	tcpAddr := strings.TrimPrefix(addr, "tcp:")

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", tcpAddr)
	if err != nil {
		return fmt.Errorf("ovn: connection check %s: %w", tcpAddr, err)
	}
	conn.Close()
	return nil
}
