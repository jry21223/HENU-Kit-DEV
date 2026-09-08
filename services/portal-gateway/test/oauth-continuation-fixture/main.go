package main

import (
	"log"
	"net/http"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/httpapi"
)

const (
	coreAddress    = "127.0.0.1:3211"
	gatewayAddress = "127.0.0.1:3210"
	portalOrigin   = "http://127.0.0.1:3111"
)

func main() {
	redisServer, err := miniredis.Run()
	if err != nil {
		log.Fatal(err)
	}
	defer redisServer.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer func() { _ = redisClient.Close() }()

	handler, err := httpapi.New(config.Config{
		PlatformCoreURL:        "http://" + coreAddress,
		PlatformCorePublicURL:  "http://" + coreAddress,
		PlatformClientID:       "portal-gateway",
		PlatformSecret:         "portal-e2e-client-secret-with-enough-entropy",
		PlatformKeyID:          "primary",
		PortalRedirectURI:      portalOrigin + "/api/v1/auth/callback",
		PortalOrigin:           portalOrigin,
		SessionKey:             []byte("0123456789abcdef0123456789abcdef"),
		LocalOAuthCookieName:   "henukit_portal_oauth_e2e",
		LocalSessionCookieName: "henukit_portal_session_e2e",
	}, redisClient)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("OAuth E2E Portal Gateway listening on %s", gatewayAddress)
	log.Fatal(http.ListenAndServe(gatewayAddress, handler.Router()))
}
