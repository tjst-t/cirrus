// createport is an internal tool for creating ports directly via the store.
// Usage: go run ./cmd/internal/createport <dsn> <network_id> <group_id> <tenant_id> <host_id> <vm_name>
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tjst-t/cirrus/internal/network"
)

func main() {
	if len(os.Args) != 7 {
		fmt.Fprintf(os.Stderr, "Usage: createport <dsn> <network_id> <group_id> <tenant_id> <host_id> <vm_name>\n")
		os.Exit(1)
	}

	dsn := os.Args[1]
	netID := uuid.MustParse(os.Args[2])
	groupID := uuid.MustParse(os.Args[3])
	tenantID := uuid.MustParse(os.Args[4])
	hostID := uuid.MustParse(os.Args[5])
	vmName := os.Args[6]

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	store := network.NewStore(pool, slog.Default())
	port, err := store.CreatePort(ctx, network.PortSpec{
		TenantID:  tenantID,
		NetworkID: netID,
		GroupID:   groupID,
		HostID:    hostID,
		VMName:    vmName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create port: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("port_id=%s ip=%s mac=%s\n", port.ID, port.IPAddress, port.MACAddress)
}
