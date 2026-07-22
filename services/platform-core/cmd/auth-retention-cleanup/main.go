package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/authretention"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL := os.Getenv("PLATFORM_CORE_DATABASE_URL")
	if databaseURL == "" {
		logger.Error("invalid configuration", "error", "PLATFORM_CORE_DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Error("database initialization failed", "error", "connection configuration rejected")
		os.Exit(1)
	}
	defer database.Close()
	result, err := authretention.Cleanup(ctx, database, time.Now().UTC())
	if err != nil {
		logger.Error("auth retention cleanup failed", "error", "cleanup transaction rejected")
		os.Exit(1)
	}
	logger.Info("auth retention cleanup completed",
		"verification_records_scrubbed", result.VerificationRecordsScrubbed,
		"exchange_idempotency_deleted", result.ExchangeIdempotencyDeleted,
	)
}
