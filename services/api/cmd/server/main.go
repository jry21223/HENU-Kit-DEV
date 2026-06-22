package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"final-review-platform/services/api/internal/server"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/database"
	applogger "final-review-platform/services/api/pkg/logger"
	redisclient "final-review-platform/services/api/pkg/redis"
)

func main() {
	cfg := config.Load()
	log := applogger.New(cfg.Environment)

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if cfg.AutoMigrate {
		if err := database.EnsureExtensions(db); err != nil {
			log.Error("database extension setup failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		if err := database.AutoMigrate(db); err != nil {
			log.Error("database migration failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}

	cache, err := redisclient.Connect(context.Background(), cfg.Redis)
	if err != nil {
		log.Error("redis connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer cache.Close()

	router := server.NewRouter(cfg, log, db, cache)
	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("api server starting", slog.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("api server shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	log.Info("api server stopped")
}
