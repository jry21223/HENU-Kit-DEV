package main

import (
	"context"
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
	if databaseURL == "" || clientID == "" || keyID == "" || secret == "" {
		log.Fatal("Account Portfolio configuration is incomplete")
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
		Database: pool,
		ClientID: clientID,
		Keys:     map[string]string{keyID: secret},
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
