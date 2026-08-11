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
	"henukit.dev/library/internal/listenaddr"
)

func main() {
	databaseURL, redisURL := os.Getenv("LIBRARY_DATABASE_URL"), os.Getenv("LIBRARY_REDIS_URL")
	clientID, keyID, secret := os.Getenv("LIBRARY_SERVICE_CLIENT_ID"), os.Getenv("LIBRARY_SERVICE_KEY_ID"), os.Getenv("LIBRARY_SERVICE_SECRET")
	downloadClientID, downloadKeyID, downloadSecret := os.Getenv("LIBRARY_DOWNLOAD_SERVICE_CLIENT_ID"), os.Getenv("LIBRARY_DOWNLOAD_SERVICE_KEY_ID"), os.Getenv("LIBRARY_DOWNLOAD_SERVICE_SECRET")
	legacyURL, legacyToken := os.Getenv("STUDY_LEGACY_API_URL"), os.Getenv("STUDY_LEGACY_ADMIN_TOKEN")
	ossBucket, ossRegion := os.Getenv("LIBRARY_OSS_BUCKET"), os.Getenv("LIBRARY_OSS_REGION")
	ossInternalEndpoint, ossPublicEndpoint, ossRAMRole := os.Getenv("LIBRARY_OSS_INTERNAL_ENDPOINT"), os.Getenv("LIBRARY_OSS_PUBLIC_ENDPOINT"), os.Getenv("LIBRARY_OSS_ECS_RAM_ROLE")
	if databaseURL == "" || redisURL == "" || clientID == "" || keyID == "" || secret == "" || downloadClientID == "" || downloadKeyID == "" || downloadSecret == "" || legacyURL == "" || legacyToken == "" || ossBucket == "" || ossRegion == "" || ossInternalEndpoint == "" || ossPublicEndpoint == "" || ossRAMRole == "" {
		log.Fatal("Library service configuration is incomplete")
	}
	downloadStore, err := library.NewAliyunDownloadStore(library.DownloadOSSConfig{Bucket: ossBucket, Region: ossRegion, InternalEndpoint: ossInternalEndpoint, PublicEndpoint: ossPublicEndpoint, ECSRAMRole: ossRAMRole})
	if err != nil {
		log.Fatal(err)
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
	handler, err := library.New(library.Config{Database: pool, Redis: redisClient, ClientID: clientID, Keys: map[string]string{keyID: secret}, DownloadClientID: downloadClientID, DownloadKeys: map[string]string{downloadKeyID: downloadSecret}, DownloadStore: downloadStore, LegacyBaseURL: legacyURL, LegacyToken: legacyToken})
	if err != nil {
		log.Fatal(err)
	}
	address := os.Getenv("LIBRARY_ADDR")
	if strings.TrimSpace(address) == "" {
		address = listenaddr.DefaultAddr
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
	log.Printf("Library service listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
