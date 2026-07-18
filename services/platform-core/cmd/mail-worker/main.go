package main

import (
	"context"
	"encoding/base64"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/mailworker"
	"henukit.dev/platform-core/internal/store"
)

func main() {
	requeueOutbox := flag.String("requeue-outbox", "", "failed outbox UUID to requeue")
	requeueRequestID := flag.String("request-id", "", "operator request ID for the audit trail")
	requeueActorID := flag.String("actor-id", "", "operator identity for the audit trail")
	requeueReason := flag.String("reason", "", "operator reason for requeueing")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL := os.Getenv("PLATFORM_CORE_DATABASE_URL")
	if databaseURL == "" {
		logger.Error("invalid mail worker database configuration")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	database, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		logger.Error("database initialization failed")
		os.Exit(1)
	}
	defer database.Close()
	if *requeueOutbox != "" {
		if err := mailworker.Requeue(ctx, store.New(database), *requeueOutbox, *requeueRequestID, *requeueActorID, *requeueReason); err != nil {
			logger.Error("mail outbox requeue failed", "request_id", *requeueRequestID, "outbox_id", *requeueOutbox, "result", "failed")
			os.Exit(1)
		}
		logger.Info("mail outbox requeued", "request_id", *requeueRequestID, "outbox_id", *requeueOutbox, "actor_id", *requeueActorID, "result", "requeued")
		return
	}
	providerEndpoint := os.Getenv("PLATFORM_CORE_MAIL_PROVIDER_ENDPOINT")
	providerToken := os.Getenv("PLATFORM_CORE_MAIL_PROVIDER_TOKEN")
	workerID := os.Getenv("PLATFORM_CORE_MAIL_WORKER_ID")
	masterKey, keyErr := base64.StdEncoding.DecodeString(os.Getenv("PLATFORM_CORE_VERIFICATION_KEY"))
	if providerEndpoint == "" || providerToken == "" || workerID == "" || keyErr != nil || len(masterKey) != 32 {
		logger.Error("invalid mail worker configuration")
		os.Exit(1)
	}
	sender, err := mailworker.NewHTTPSender(providerEndpoint, providerToken, &http.Client{Timeout: 12 * time.Second})
	if err != nil {
		logger.Error("mail provider configuration failed")
		os.Exit(1)
	}
	worker, err := mailworker.New(store.New(database), sender, workerID, masterKey, 2*time.Minute, 10*time.Second)
	if err != nil {
		logger.Error("mail worker initialization failed")
		os.Exit(1)
	}
	logger.Info("mail worker started", "worker_id", workerID)
	for ctx.Err() == nil {
		outcome, processErr := worker.ProcessOne(ctx)
		if processErr != nil {
			logger.Error("mail outbox processing failed",
				"request_id", outcome.RequestID, "outbox_id", outcome.OutboxID,
				"result", outcome.Result, "attempt_count", outcome.AttemptCount,
				"duration_ms", outcome.Duration.Milliseconds(), "error_code", "OUTBOX_PROCESSING_FAILED")
			wait(ctx, time.Second)
			continue
		}
		if outcome.Processed {
			logger.Info("mail outbox processed",
				"request_id", outcome.RequestID, "outbox_id", outcome.OutboxID,
				"result", outcome.Result, "attempt_count", outcome.AttemptCount,
				"duration_ms", outcome.Duration.Milliseconds(), "error_code", outcome.ErrorCode)
		} else {
			wait(ctx, 500*time.Millisecond)
		}
	}
}

func wait(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
