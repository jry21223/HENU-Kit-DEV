package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	accountportfolio "henukit.dev/account-portfolio"
)

func main() {
	databaseURL := os.Getenv("ACCOUNT_PORTFOLIO_DATABASE_URL")
	clientID := os.Getenv("ACCOUNT_PORTFOLIO_SERVICE_CLIENT_ID")
	keyID := os.Getenv("ACCOUNT_PORTFOLIO_SERVICE_KEY_ID")
	secret := os.Getenv("ACCOUNT_PORTFOLIO_SERVICE_SECRET")
	consoleClientID := os.Getenv("ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID")
	consoleKeyID := os.Getenv("ACCOUNT_PORTFOLIO_CONSOLE_KEY_ID")
	consoleSecret := os.Getenv("ACCOUNT_PORTFOLIO_CONSOLE_SECRET")
	pointCursorKey, err := pointCursorKeyFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if databaseURL == "" || clientID == "" || keyID == "" || secret == "" {
		log.Fatal("Account Portfolio configuration is incomplete")
	}
	consoleConfigured := consoleClientID != "" || consoleKeyID != "" || consoleSecret != ""
	if consoleConfigured && (consoleClientID == "" || consoleKeyID == "" || consoleSecret == "") {
		log.Fatal("Account Portfolio Console caller configuration is incomplete")
	}
	if os.Getenv("ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET") == "1" && isPlaceholderSecret(secret) {
		log.Fatal("Account Portfolio service secret is a deployment placeholder")
	}
	if consoleConfigured && os.Getenv("ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET") == "1" && isPlaceholderSecret(consoleSecret) {
		log.Fatal("Account Portfolio Console service secret is a deployment placeholder")
	}
	paymentProvider, err := paymentProviderFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	var consoleKeys map[string]string
	if consoleConfigured {
		consoleKeys = map[string]string{consoleKeyID: consoleSecret}
	}

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := accountportfolio.ApplyMigrations(context.Background(), pool); err != nil {
		log.Fatal(err)
	}
	handler, err := accountportfolio.New(accountportfolio.Config{
		Database:        pool,
		ClientID:        clientID,
		Keys:            map[string]string{keyID: secret},
		ConsoleClientID: consoleClientID,
		ConsoleKeys:     consoleKeys,
		PointCursorKey:  pointCursorKey,
		PaymentProvider: paymentProvider,
	})
	if err != nil {
		log.Fatal(err)
	}

	address := strings.TrimSpace(os.Getenv("ACCOUNT_PORTFOLIO_ADDR"))
	if address == "" {
		address = ":8097"
	}
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Account Portfolio service listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func paymentProviderFromEnv() (accountportfolio.PaymentProvider, error) {
	if os.Getenv("ACCOUNT_PORTFOLIO_EASYPAY_ENABLED") != "1" {
		return nil, nil
	}
	key := os.Getenv("ACCOUNT_PORTFOLIO_EASYPAY_KEY")
	if os.Getenv("ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET") == "1" && isPlaceholderSecret(key) {
		return nil, errors.New("Account Portfolio EasyPay key is a deployment placeholder")
	}
	provider, err := accountportfolio.NewEasyPayProvider(accountportfolio.EasyPayConfig{
		BaseURL:   os.Getenv("ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL"),
		PID:       os.Getenv("ACCOUNT_PORTFOLIO_EASYPAY_PID"),
		Key:       key,
		NotifyURL: os.Getenv("ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL"),
		ReturnURL: os.Getenv("ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL"),
	})
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func pointCursorKeyFromEnv() ([]byte, error) {
	encoded := os.Getenv("ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY")
	if strings.TrimSpace(encoded) == "" || strings.TrimSpace(encoded) != encoded {
		return nil, errors.New("ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY must be base64 for exactly 32 bytes that are not all zero")
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 || bytes.Equal(key, make([]byte, 32)) {
		return nil, errors.New("ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY must be base64 for exactly 32 bytes that are not all zero")
	}
	if os.Getenv("ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET") == "1" && isPlaceholderSecret(string(key)) {
		return nil, errors.New("ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY is a deployment placeholder")
	}
	return key, nil
}

func isPlaceholderSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	return strings.HasPrefix(value, "replace-") ||
		strings.HasPrefix(value, "change-me") ||
		strings.HasPrefix(value, "example-") ||
		strings.Contains(value, "placeholder")
}
