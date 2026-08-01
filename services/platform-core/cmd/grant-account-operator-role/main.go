package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/accountoperatorgrant"
)

func main() {
	roleCode := flag.String("role-code", "", "active Platform Core role receiving Account Console permissions")
	requestID := flag.String("request-id", "", "immutable audit request ID beginning with req_")
	reason := flag.String("reason", "", "audited production grant reason")
	flag.Parse()
	databaseURL := os.Getenv("PLATFORM_CORE_DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "Platform Core database configuration is unavailable")
		os.Exit(1)
	}
	ctx := context.Background()
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database initialization failed")
		os.Exit(1)
	}
	defer database.Close()
	result, err := accountoperatorgrant.Grant(ctx, database, accountoperatorgrant.Input{
		RoleCode: *roleCode, Actor: "henukit-release", RequestID: *requestID, Reason: *reason,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Account operator role grant failed")
		os.Exit(1)
	}
	fmt.Printf("Account operator role grant audited (changed=%t)\n", result.Changed)
}
