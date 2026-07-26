package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"henukit.dev/platform-core/internal/smtpprovider"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	listenAddress := env("PLATFORM_CORE_SMTP_PROVIDER_ADDRESS", "127.0.0.1:18081")
	host, _, err := net.SplitHostPort(listenAddress)
	allowDocker := os.Getenv("PLATFORM_CORE_SMTP_ALLOW_DOCKER") == "1"
	ip := net.ParseIP(host)
	// Default: loopback only (host process). Docker compose may bind 0.0.0.0
	// when PLATFORM_CORE_SMTP_ALLOW_DOCKER=1 so mail-worker can reach it on the
	// internal network; do not expose this port on the public host.
	okBind := err == nil && ip != nil && (ip.IsLoopback() || (allowDocker && (ip.IsUnspecified() || ip.IsPrivate())))
	if !okBind {
		logger.Error("SMTP provider must listen on loopback, or 0.0.0.0/private with PLATFORM_CORE_SMTP_ALLOW_DOCKER=1")
		os.Exit(1)
	}
	mailer, err := smtpprovider.NewSMTPMailer(
		os.Getenv("PLATFORM_CORE_SMTP_ADDRESS"), os.Getenv("PLATFORM_CORE_SMTP_USERNAME"),
		os.Getenv("PLATFORM_CORE_SMTP_PASSWORD"), os.Getenv("PLATFORM_CORE_SMTP_FROM"), 10*time.Second,
	)
	if err != nil {
		logger.Error("invalid SMTP configuration")
		os.Exit(1)
	}
	provider, err := smtpprovider.New(smtpprovider.Config{
		Token:           os.Getenv("PLATFORM_CORE_MAIL_PROVIDER_TOKEN"),
		LedgerDirectory: env("PLATFORM_CORE_SMTP_LEDGER_DIR", "/var/lib/henukit-smtp-provider"), Mailer: mailer,
		Logger: logger, ProviderID: env("PLATFORM_CORE_SMTP_PROVIDER_ID", "local-smtp"),
		KeyID: env("PLATFORM_CORE_MAIL_PROVIDER_KEY_ID", "mail-provider-active"),
	})
	if err != nil {
		logger.Error("invalid local provider configuration")
		os.Exit(1)
	}
	server := &http.Server{Addr: listenAddress, Handler: provider, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("local SMTP provider listening", "address", listenAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("local SMTP provider stopped")
		os.Exit(1)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
