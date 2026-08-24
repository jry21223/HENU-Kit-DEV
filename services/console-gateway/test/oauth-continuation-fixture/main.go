package main

import (
	"log"
	"net/http"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	consolegateway "henukit.dev/console-gateway"
)

const (
	coreAddress    = "127.0.0.1:3231"
	gatewayAddress = "127.0.0.1:3230"
	consoleOrigin  = "http://127.0.0.1:4175"
)

func main() {
	redisServer, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer redisServer.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()

	handler, err := consolegateway.New(consolegateway.Config{
		PlatformCoreURL:       "http://" + coreAddress,
		PlatformAccountOrigin: "http://" + coreAddress,
		ClientID:              "console-gateway",
		ClientSecret:          "console-e2e-client-secret-with-enough-entropy",
		KeyID:                 "primary",
		RedirectURI:           consoleOrigin + "/api/v1/auth/callback",
		SessionKey:            []byte("0123456789abcdef0123456789abcdef"),
		Redis:                 redisClient,
		HTTPClient:            &http.Client{Timeout: 10 * time.Second},
		OverviewEndpoints:     map[string]string{},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Console OAuth E2E Gateway listening on %s", gatewayAddress)
	log.Fatal(http.ListenAndServe(gatewayAddress, handler))
}
