package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"final-review-platform/services/worker/internal/worker"
	"final-review-platform/services/worker/pkg/config"
)

func main() {
	cfg := config.Load()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	runner := worker.NewRunner(cfg, log)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := runner.Run(ctx); err != nil {
		log.Error("worker stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
