package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	portalgateway "henukit.dev/portal-gateway"
	"henukit.dev/portal-gateway/internal/config"
)

func main() {
	if err := validateFoodPostDeploymentSecrets(os.Getenv); err != nil {
		log.Fatalf("config: %v", err)
	}
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	handler, err := portalgateway.New(cfg)
	if err != nil {
		log.Fatalf("gateway: %v", err)
	}

	addr := cfg.ListenAddr
	log.Printf("portal-gateway listening on %s", addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
		os.Exit(1)
	}
}

// validateFoodPostDeploymentSecrets keeps the public/example Food Post secrets
// usable only for an explicitly loopback-shaped local Portal. Any non-loopback
// deployment with the Food Post boundary enabled must provide complete,
// non-placeholder create/read secrets before the gateway starts.
func validateFoodPostDeploymentSecrets(getenv func(string) string) error {
	if strings.TrimSpace(getenv("FOOD_POSTS_URL")) == "" {
		return nil
	}
	if isLoopbackOrigin(getenv("PORTAL_ORIGIN")) {
		return nil
	}
	for _, key := range []string{"FOOD_POST_CREATE_SECRET", "FOOD_POST_READ_SECRET"} {
		secret := getenv(key)
		if len(secret) < 32 || isPlaceholderSecret(secret) {
			return fmt.Errorf("%s must be an explicit non-placeholder secret for non-loopback Portal deployments", key)
		}
	}
	return nil
}

func isLoopbackOrigin(origin string) bool {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func isPlaceholderSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	return strings.HasPrefix(value, "replace-") ||
		strings.HasPrefix(value, "change-me") ||
		strings.HasPrefix(value, "example-") ||
		strings.Contains(value, "placeholder")
}
