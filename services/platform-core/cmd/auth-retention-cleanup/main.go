package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/authretention"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	runID := "req_retention_" + uuid.NewString()
	startedAt := time.Now()
	databaseURL := os.Getenv("PLATFORM_CORE_DATABASE_URL")
	if databaseURL == "" {
		logFailure(logger, runID, startedAt, "CONFIGURATION_INVALID")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logFailure(logger, runID, startedAt, "DATABASE_CONFIGURATION_REJECTED")
		os.Exit(1)
	}
	defer database.Close()
	result, err := authretention.Cleanup(ctx, database, time.Now().UTC())
	if err != nil {
		logFailure(logger, runID, startedAt, "CLEANUP_TRANSACTION_REJECTED")
		os.Exit(1)
	}
	logger.Info("auth retention cleanup completed",
		"request_id", runID,
		"result", "succeeded",
		"error_code", "NONE",
		"resource_type", "auth_credential_retention",
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"attempt_count", 1,
		"retry_count", 0,
		"verification_records_scrubbed", result.VerificationRecordsScrubbed,
		"outbox_payloads_scrubbed", result.OutboxPayloadsScrubbed,
		"exchange_idempotency_deleted", result.ExchangeIdempotencyDeleted,
	)
}

func logFailure(logger *slog.Logger, runID string, startedAt time.Time, errorCode string) {
	logger.Error("auth retention cleanup failed",
		"request_id", runID,
		"result", "failed",
		"error_code", errorCode,
		"resource_type", "auth_credential_retention",
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"attempt_count", 1,
		"retry_count", 0,
	)
}
