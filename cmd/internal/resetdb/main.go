package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: resetdb <dsn>")
		os.Exit(1)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		fmt.Fprintf(os.Stderr, "reset: %v\n", err)
		os.Exit(1)
	}
}
