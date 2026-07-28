package main

import (
	"context"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	accountportfolio "henukit.dev/account-portfolio"
)

func main() {
	databaseURL := os.Getenv("ACCOUNT_PORTFOLIO_DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("ACCOUNT_PORTFOLIO_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := accountportfolio.ApplyMigrations(context.Background(), pool); err != nil {
		log.Fatal(err)
	}
}
