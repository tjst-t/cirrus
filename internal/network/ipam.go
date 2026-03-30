package network

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
)

// defaultPool is the system-wide CIDR pool for auto-assigning network CIDRs.
const defaultPool = "100.64.0.0/10"

// defaultBlockSize is the prefix length auto-assigned to each network.
const defaultBlockSize = 22 // /22 = 1024 addresses = 256 VMs

// AllocateBlock finds the next available /30 block within a network CIDR.
// existingIPs is the list of already-allocated VM IP addresses in the network.
// Returns the VM IP (.1) and gateway IP (.2) of the allocated /30 block.
// Deleted VM IPs are never reused (conntrack state risk).
func AllocateBlock(cidr string, existingIPs []string) (vmIP, gwIP string, err error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", "", fmt.Errorf("ipam: parse cidr: %w", err)
	}

	baseIP := ipToUint32(ipNet.IP)
	ones, bits := ipNet.Mask.Size()
	hostCount := uint32(1) << uint(bits-ones)

	// Total /30 blocks available in this CIDR
	totalBlocks := hostCount / 4
	if totalBlocks == 0 {
		return "", "", fmt.Errorf("ipam: cidr %s too small for /30 allocation", cidr)
	}

	// Find the max used block index from existing IPs
	maxBlock := int(-1)
	for _, ipStr := range existingIPs {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		offset := ipToUint32(ip4) - baseIP
		blockIdx := int(offset / 4)
		if blockIdx > maxBlock {
			maxBlock = blockIdx
		}
	}

	nextBlock := uint32(maxBlock + 1)
	if nextBlock >= totalBlocks {
		return "", "", ErrCIDRExhausted
	}

	blockBase := baseIP + nextBlock*4
	vmAddr := uint32ToIP(blockBase + 1)
	gwAddr := uint32ToIP(blockBase + 2)

	return vmAddr.String(), gwAddr.String(), nil
}

// GenerateMAC generates a random locally-administered unicast MAC address.
// Format: 02:xx:xx:xx:xx:xx
func GenerateMAC() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("ipam: generate mac: %w", err)
	}
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4]), nil
}

// AssignCIDR auto-assigns a /22 CIDR block from the default pool (100.64.0.0/10),
// avoiding overlap with any existing CIDRs.
func AssignCIDR(existingCIDRs []string) (string, error) {
	_, pool, err := net.ParseCIDR(defaultPool)
	if err != nil {
		return "", fmt.Errorf("ipam: parse pool: %w", err)
	}

	poolBase := ipToUint32(pool.IP)
	poolOnes, poolBits := pool.Mask.Size()
	poolSize := uint32(1) << uint(poolBits-poolOnes)

	blockSize := uint32(1) << uint(32-defaultBlockSize) // /22 = 1024

	// Parse existing CIDRs into ranges
	type cidrRange struct {
		start, end uint32
	}
	var existing []cidrRange
	for _, c := range existingCIDRs {
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			continue
		}
		start := ipToUint32(ipNet.IP)
		ones, bits := ipNet.Mask.Size()
		size := uint32(1) << uint(bits-ones)
		existing = append(existing, cidrRange{start: start, end: start + size})
	}

	// Walk the pool in /22 increments, find the first non-overlapping block
	for offset := uint32(0); offset < poolSize; offset += blockSize {
		candidateStart := poolBase + offset
		candidateEnd := candidateStart + blockSize

		overlap := false
		for _, ex := range existing {
			if candidateStart < ex.end && candidateEnd > ex.start {
				overlap = true
				break
			}
		}
		if !overlap {
			cidr := fmt.Sprintf("%s/%d", uint32ToIP(candidateStart).String(), defaultBlockSize)
			return cidr, nil
		}
	}

	return "", fmt.Errorf("ipam: no available CIDR blocks in pool %s", defaultPool)
}

func ipToUint32(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

func uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}
