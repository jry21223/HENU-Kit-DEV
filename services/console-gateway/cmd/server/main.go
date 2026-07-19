package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	consolegateway "henukit.dev/console-gateway"
	"henukit.dev/console-gateway/internal/config"
	"henukit.dev/console-gateway/internal/overview"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := config.FromEnv()
	if err != nil {
		logger.Error("invalid_config", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: config.RedisAddr})
	overviewCredentials := make(map[string]overview.Credentials, len(config.OverviewCredentials))
	for id, credential := range config.OverviewCredentials {
		overviewCredentials[id] = overview.Credentials{ClientID: credential.ClientID, ClientSecret: credential.ClientSecret, KeyID: credential.KeyID}
	}
	handler, err := consolegateway.New(consolegateway.Config{
		PlatformCoreURL: config.PlatformCoreURL, PlatformAccountOrigin: config.PlatformAuthorize,
		ClientID: config.ClientID, ClientSecret: config.ClientSecret, KeyID: config.KeyID, RedirectURI: config.RedirectURI,
		SessionKey: config.SessionKey, Redis: redisClient, Logger: logger,
		OverviewEndpoints: config.OverviewEndpoints, OverviewCredentials: overviewCredentials,
	})
	if err != nil {
		logger.Error("create_gateway", "error", err)
		os.Exit(1)
	}
	server := &http.Server{Addr: config.ListenAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("serve", "error", err)
			os.Exit(1)
		}
	}()
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-shutdown.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
