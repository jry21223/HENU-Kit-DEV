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

	notice "henukit.dev/notice"
)

func main() {
	databaseURL := os.Getenv("NOTICE_DATABASE_URL")
	clientID := os.Getenv("NOTICE_SERVICE_CLIENT_ID")
	keyID := os.Getenv("NOTICE_SERVICE_KEY_ID")
	secret := os.Getenv("NOTICE_SERVICE_SECRET")
	portalClientID := os.Getenv("NOTICE_PORTAL_CLIENT_ID")
	portalKeyID := os.Getenv("NOTICE_PORTAL_KEY_ID")
	portalSecret := os.Getenv("NOTICE_PORTAL_SECRET")
	redisURL := os.Getenv("NOTICE_REDIS_URL")
	if databaseURL == "" || redisURL == "" || clientID == "" || keyID == "" || secret == "" || portalClientID == "" || portalKeyID == "" || portalSecret == "" {
		log.Fatal("Notice database and service credentials are required")
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
	handler, err := notice.New(notice.Config{Database: pool, Redis: redisClient, ClientID: clientID, Keys: map[string]string{keyID: secret}, PortalClientID: portalClientID, PortalKeys: map[string]string{portalKeyID: portalSecret}})
	if err != nil {
		log.Fatal(err)
	}
	address := os.Getenv("NOTICE_ADDR")
	if strings.TrimSpace(address) == "" {
		address = ":8094"
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
	log.Printf("Notice service listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
