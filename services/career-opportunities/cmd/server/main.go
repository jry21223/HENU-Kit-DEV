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
	career "henukit.dev/career"
)

func main() {
	databaseURL, redisURL := os.Getenv("CAREER_DATABASE_URL"), os.Getenv("CAREER_REDIS_URL")
	clientID, keyID, secret := os.Getenv("CAREER_SERVICE_CLIENT_ID"), os.Getenv("CAREER_SERVICE_KEY_ID"), os.Getenv("CAREER_SERVICE_SECRET")
	if databaseURL == "" || redisURL == "" || clientID == "" || keyID == "" || secret == "" {
		log.Fatal("Career configuration is incomplete")
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
	service, err := career.New(career.Config{
		Database: pool, Redis: redisClient, ClientID: clientID, Keys: map[string]string{keyID: secret},
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := service.Claims().Run(ctx); err != nil && err != context.Canceled {
			log.Printf("career worker stopped: %v", err)
		}
	}()
	address := strings.TrimSpace(os.Getenv("CAREER_ADDR"))
	if address == "" {
		address = ":8097"
	}
	server := &http.Server{Addr: address, Handler: service, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Career service listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
