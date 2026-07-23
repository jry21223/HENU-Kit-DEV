package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/user"

	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/operatorbootstrap"
)

func main() {
	email := flag.String("email", "", "existing verified henu.edu.cn account")
	requestID := flag.String("request-id", "", "audit request ID beginning with req_")
	reason := flag.String("reason", "", "operator grant reason")
	flag.Parse()
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "grant-initial-operator must run as root")
		os.Exit(1)
	}
	databaseURL := os.Getenv("PLATFORM_CORE_DATABASE_URL")
	verificationKey, err := base64.StdEncoding.DecodeString(os.Getenv("PLATFORM_CORE_VERIFICATION_KEY"))
	currentUser, userErr := user.Current()
	if databaseURL == "" || err != nil || len(verificationKey) != 32 || userErr != nil {
		fmt.Fprintln(os.Stderr, "invalid root-owned Platform Core configuration")
		os.Exit(1)
	}
	ctx := context.Background()
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database initialization failed")
		os.Exit(1)
	}
	defer database.Close()
	result, err := operatorbootstrap.Grant(ctx, database, verificationKey, operatorbootstrap.Input{Email: *email, ActorUnixUser: currentUser.Username, RequestID: *requestID, Reason: *reason})
	if err != nil {
		fmt.Fprintln(os.Stderr, "initial operator grant failed")
		os.Exit(1)
	}
	fmt.Printf("initial operator grant audited for user %s (changed=%t)\n", result.UserID, result.Changed)
}
