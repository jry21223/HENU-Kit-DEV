package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	quizcraft "henukit.dev/quizcraft"
)

func main() {
	databaseURL := requiredEnv("DATABASE_URL")
	authSecret := requiredEnv("QUIZCRAFT_AUTH_HMAC_SECRET")
	address := os.Getenv("QUIZCRAFT_HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	fail(err)
	defer pool.Close()
	fail(pool.Ping(ctx))
	summaryKeys := map[string]string{}
	if keyID := os.Getenv("QUIZCRAFT_SUMMARY_KEY_ID"); keyID != "" {
		summaryKeys[keyID] = os.Getenv("QUIZCRAFT_SUMMARY_CLIENT_SECRET")
	}
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:             pool,
		AuthHMACSecret:       []byte(authSecret),
		LegacyBaseURL:        os.Getenv("QUIZCRAFT_LEGACY_BASE_URL"),
		LegacyCompareSecret:  os.Getenv("QUIZCRAFT_LEGACY_COMPARE_SECRET"),
		SummaryClientID:      os.Getenv("QUIZCRAFT_SUMMARY_CLIENT_ID"),
		SummaryKeys:          summaryKeys,
		PlatformCoreURL:      os.Getenv("PLATFORM_CORE_URL"),
		PlatformClientID:     os.Getenv("QUIZCRAFT_PLATFORM_CLIENT_ID"),
		PlatformClientSecret: os.Getenv("QUIZCRAFT_PLATFORM_CLIENT_SECRET"),
		PlatformKeyID:        os.Getenv("QUIZCRAFT_PLATFORM_KEY_ID"),
		PublicURL:            os.Getenv("QUIZCRAFT_PUBLIC_URL"),
		SessionEncryptionKey: []byte(os.Getenv("QUIZCRAFT_SESSION_ENCRYPTION_KEY")),
		InboxExchangeToken:   os.Getenv("QUIZCRAFT_INBOX_EXCHANGE_TOKEN"),
	})
	fail(err)
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("QuizCraft Practice shadow service listening on %s", address)
	err = server.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		fail(err)
	}
}

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		fail(fmt.Errorf("%s is required", name))
	}
	return value
}

func fail(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
