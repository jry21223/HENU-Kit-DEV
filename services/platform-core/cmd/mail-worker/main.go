package main

import (
	"context"
	"encoding/base64"
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	databaseURL := os.Getenv("PLATFORM_CORE_DATABASE_URL")
	providerEndpoint := os.Getenv("PLATFORM_CORE_MAIL_PROVIDER_ENDPOINT")
	providerToken := os.Getenv("PLATFORM_CORE_MAIL_PROVIDER_TOKEN")
	workerID := os.Getenv("PLATFORM_CORE_MAIL_WORKER_ID")
	masterKey, keyErr := base64.StdEncoding.DecodeString(os.Getenv("PLATFORM_CORE_VERIFICATION_KEY"))
	if databaseURL == "" || providerEndpoint == "" || providerToken == "" || workerID == "" || keyErr != nil || len(masterKey) != 32 {
		logger.Error("invalid mail worker configuration")
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
		processed, processErr := worker.ProcessOne(ctx)
		if processErr != nil {
			logger.Error("mail outbox processing failed", "error_code", "OUTBOX_PROCESSING_FAILED")
			wait(ctx, time.Second)
			continue
		}
		if !processed {
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
