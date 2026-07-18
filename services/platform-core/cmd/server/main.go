package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	settings, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err.Error())
		os.Exit(1)
	}
	ctx := context.Background()
	database, err := pgxpool.New(ctx, settings.DatabaseURL)
	if err != nil {
		logger.Error("database initialization failed", "error", "connection configuration rejected")
		os.Exit(1)
	}
	defer database.Close()
	redisOptions, err := redis.ParseURL(settings.RedisURL)
	if err != nil {
		logger.Error("invalid configuration", "error", "PLATFORM_CORE_REDIS_URL is invalid")
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOptions)
	defer func() { _ = redisClient.Close() }()
	handler, err := platformcore.New(platformcore.Config{
		Database: database, Redis: redisClient, CoreCookieName: settings.CoreCookieName,
		AuthorizationTTL: settings.AuthorizationTTL, ExchangeSessionTTL: settings.ExchangeSessionTTL,
		IdempotencyEncryptionKey: settings.IdempotencyEncryptionKey, Logger: logger,
		IdempotencyTTL:            settings.IdempotencyTTL,
		VerificationEncryptionKey: settings.VerificationKey, StudentEmailDomains: settings.StudentEmailDomains,
		VerificationCodeTTL: settings.VerificationCodeTTL, VerificationResendDelay: settings.VerificationResendDelay,
		MailDeliveryWebhookToken: settings.MailDeliveryWebhookToken,
		MailDeliveryActiveKeyID:  settings.MailDeliveryActiveKeyID, MailDeliveryRetiringToken: settings.MailDeliveryRetiringToken, MailDeliveryRetiringKeyID: settings.MailDeliveryRetiringKeyID,
		TrustedProxyCIDRs: settings.TrustedProxyCIDRs,
	})
	if err != nil {
		logger.Error("server initialization failed", "error", err.Error())
		os.Exit(1)
	}
	server := &http.Server{
		Addr: settings.Address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	shutdownContext, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownContext.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err.Error())
		}
	}()
	logger.Info("platform core listening", "address", settings.Address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped unexpectedly", "error", err.Error())
		os.Exit(1)
	}
}
