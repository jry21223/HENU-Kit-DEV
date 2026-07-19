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
	"github.com/redis/go-redis/v9"

	library "henukit.dev/library"
)

func main() {
	databaseURL, redisURL := os.Getenv("LIBRARY_DATABASE_URL"), os.Getenv("LIBRARY_REDIS_URL")
	clientID, keyID, secret := os.Getenv("LIBRARY_SERVICE_CLIENT_ID"), os.Getenv("LIBRARY_SERVICE_KEY_ID"), os.Getenv("LIBRARY_SERVICE_SECRET")
	legacyURL, legacyToken := os.Getenv("STUDY_LEGACY_API_URL"), os.Getenv("STUDY_LEGACY_ADMIN_TOKEN")
	if databaseURL == "" || redisURL == "" || clientID == "" || keyID == "" || secret == "" || legacyURL == "" || legacyToken == "" {
		log.Fatal("Library adapter configuration is incomplete")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatal(err)
	}
	redisClient := redis.NewClient(redisOptions)
	defer redisClient.Close()
	handler, err := library.New(library.Config{Database: pool, Redis: redisClient, ClientID: clientID, Keys: map[string]string{keyID: secret}, LegacyBaseURL: legacyURL, LegacyToken: legacyToken})
	if err != nil {
		log.Fatal(err)
	}
	address := os.Getenv("LIBRARY_ADDR")
	if strings.TrimSpace(address) == "" {
		address = ":8095"
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Library adapter listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
