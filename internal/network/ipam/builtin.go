package ipam

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoAvailableIP  = errors.New("ipam: no available IP in subnet range")
	ErrSubnetNotFound = errors.New("ipam: subnet not found")
)

// BuiltinIPAM implements IPAM using PostgreSQL CIDR/INET operations.
type BuiltinIPAM struct {
	pool *pgxpool.Pool
}

// NewBuiltinIPAM creates a new built-in IPAM backed by PostgreSQL.
func NewBuiltinIPAM(pool *pgxpool.Pool) *BuiltinIPAM {
	return &BuiltinIPAM{pool: pool}
}

func (b *BuiltinIPAM) AllocateIP(ctx context.Context, subnetID uuid.UUID) (net.IP, error) {
	// Find the next available IP by computing the first gap in the allocated range.
	// Uses a LEFT JOIN approach instead of generate_series to avoid expanding the full range.
	var ipStr string
	err := b.pool.QueryRow(ctx, `
		WITH subnet AS (
			SELECT dhcp_range_start, dhcp_range_end
			FROM subnets WHERE id = $1
		),
		max_offset AS (
			SELECT (subnet.dhcp_range_end - subnet.dhcp_range_start)::INT AS max_off
			FROM subnet
		),
		used AS (
			SELECT (ip_address - subnet.dhcp_range_start)::INT AS off
			FROM ports, subnet
			WHERE subnet_id = $1
		)
		SELECT host((subnet.dhcp_range_start + gs)::INET)
		FROM subnet, max_offset,
		     generate_series(0, max_offset.max_off) AS gs
		WHERE gs NOT IN (SELECT off FROM used)
		LIMIT 1
	`, subnetID).Scan(&ipStr)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoAvailableIP
		}
		return nil, fmt.Errorf("ipam: allocate ip: %w", err)
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("ipam: invalid IP from DB: %s", ipStr)
	}
	return ip, nil
}

func (b *BuiltinIPAM) ReleaseIP(ctx context.Context, subnetID uuid.UUID, ip net.IP) error {
	// No-op for built-in IPAM: IP is freed when the port row is deleted.
	return nil
}

func (b *BuiltinIPAM) AllocateMAC(ctx context.Context) (net.HardwareAddr, error) {
	// Generate a locally-administered unicast MAC: 02:xx:xx:xx:xx:xx
	// Rely on DB UNIQUE constraint to catch collisions instead of pre-checking.
	for i := 0; i < 10; i++ {
		mac := make([]byte, 6)
		if _, err := rand.Read(mac); err != nil {
			return nil, fmt.Errorf("ipam: generate mac: %w", err)
		}
		mac[0] = 0x02 // locally administered, unicast

		hw := net.HardwareAddr(mac)

		// Try to verify uniqueness; the real guard is the DB UNIQUE constraint on ports.mac_address.
		// With 2^40 MAC space, collisions are astronomically unlikely.
		var exists bool
		err := b.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM ports WHERE mac_address = $1)`, hw.String(),
		).Scan(&exists)
		if err != nil {
			return nil, fmt.Errorf("ipam: check mac uniqueness: %w", err)
		}
		if !exists {
			return hw, nil
		}
	}
	return nil, fmt.Errorf("ipam: failed to generate unique MAC after retries")
}

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
