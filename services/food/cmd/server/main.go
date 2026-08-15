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
	food "henukit.dev/food"
)

func main() {
	databaseURL, redisURL := os.Getenv("FOOD_DATABASE_URL"), os.Getenv("FOOD_REDIS_URL")
	clientID, keyID, secret := os.Getenv("FOOD_SERVICE_CLIENT_ID"), os.Getenv("FOOD_SERVICE_KEY_ID"), os.Getenv("FOOD_SERVICE_SECRET")
	if databaseURL == "" || redisURL == "" || clientID == "" || keyID == "" || secret == "" {
		log.Fatal("Food configuration is incomplete")
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
	handler, err := food.New(food.Config{
		Database: pool, Redis: redisClient, ClientID: clientID, Keys: map[string]string{keyID: secret},
		PostCreateClientID: os.Getenv("FOOD_POST_CREATE_CLIENT_ID"),
		PostCreateKeys:     optionalKeyRing("FOOD_POST_CREATE_KEY_ID", "FOOD_POST_CREATE_SECRET"),
		PostReadClientID:   os.Getenv("FOOD_POST_READ_CLIENT_ID"),
		PostReadKeys:       optionalKeyRing("FOOD_POST_READ_KEY_ID", "FOOD_POST_READ_SECRET"),
	})
	if err != nil {
		log.Fatal(err)
	}
	address := strings.TrimSpace(os.Getenv("FOOD_ADDR"))
	if address == "" {
		address = ":8096"
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
	log.Printf("Food service listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// optionalKeyRing builds an all-or-nothing Food Post key ring from a key id
// and secret environment variable pair. A fully absent pair yields nil, which
// leaves the corresponding routes answering 401 while the service still
// starts (fail closed without a hard crash).
func optionalKeyRing(keyIDEnv, secretEnv string) map[string]string {
	keyID, secret := os.Getenv(keyIDEnv), os.Getenv(secretEnv)
	if keyID == "" && secret == "" {
		return nil
	}
	return map[string]string{keyID: secret}
}
