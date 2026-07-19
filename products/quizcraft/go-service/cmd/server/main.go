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
	handler, err := quizcraft.NewPracticeHTTP(quizcraft.PracticeHTTPConfig{
		Database:            pool,
		AuthHMACSecret:      []byte(authSecret),
		LegacyBaseURL:       os.Getenv("QUIZCRAFT_LEGACY_BASE_URL"),
		LegacyCompareSecret: os.Getenv("QUIZCRAFT_LEGACY_COMPARE_SECRET"),
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
