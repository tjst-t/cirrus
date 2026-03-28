package ipam

import (
	"context"
	"net"

	"github.com/google/uuid"
)

// IPAM defines IP address and MAC address management operations.
type IPAM interface {
	// AllocateIP picks an unused IP from the subnet's DHCP range and marks it used.
	AllocateIP(ctx context.Context, subnetID uuid.UUID) (net.IP, error)
	// ReleaseIP frees a previously allocated IP.
	ReleaseIP(ctx context.Context, subnetID uuid.UUID, ip net.IP) error
	// AllocateMAC generates a locally-administered unicast MAC address (02:xx:xx:xx:xx:xx)
	// that is unique across all ports.
	AllocateMAC(ctx context.Context) (net.HardwareAddr, error)
}
