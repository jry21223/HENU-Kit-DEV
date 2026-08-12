package consolegateway

import (
	"net/http"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/overview"
)

func TestNewRejectsSummarySecretReusedFromPlatformCore(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = redisClient.Close() })
	platformSecret := "platform-oauth-secret-with-entropy"
	config := Config{
		PlatformCoreURL: "https://account.henukit.test", PlatformAccountOrigin: "https://account.henukit.test",
		ClientID: "console-gateway", ClientSecret: platformSecret, KeyID: "platform-active-key", RedirectURI: "https://console.henukit.test/api/v1/auth/callback",
		SessionKey: []byte("0123456789abcdef0123456789abcdef"), Redis: redisClient, HTTPClient: &http.Client{},
		OverviewEndpoints:   map[string]string{"portal": "https://portal.internal/api/v1/console-summary"},
		OverviewCredentials: map[string]overview.Credentials{"portal": {ClientID: "console-gateway-portal", ClientSecret: platformSecret, KeyID: "portal-active-key"}},
	}
	if _, err := New(config); err == nil || !strings.Contains(err.Error(), "separate from Platform Core") {
		t.Fatalf("New() error = %v, want OAuth secret separation failure", err)
	}
	config.OverviewCredentials["portal"] = overview.Credentials{ClientID: "console-gateway-portal", ClientSecret: "portal-only-secret-with-entropy", KeyID: "portal-active-key"}
	if _, err := New(config); err != nil {
		t.Fatalf("New() rejected isolated summary credentials: %v", err)
	}
}

func TestNewRejectsAccountPortfolioConsoleSecretReusedFromPlatformCore(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = redisClient.Close() })
	platformSecret := "platform-oauth-secret-with-entropy"
	config := Config{
		PlatformCoreURL: "https://account.henukit.test", PlatformAccountOrigin: "https://account.henukit.test",
		ClientID: "console-gateway", ClientSecret: platformSecret, KeyID: "platform-active-key", RedirectURI: "https://console.henukit.test/api/v1/auth/callback",
		SessionKey: []byte("0123456789abcdef0123456789abcdef"), Redis: redisClient, HTTPClient: &http.Client{},
		OverviewEndpoints:           map[string]string{"portal": "https://portal.internal/api/v1/console-summary"},
		OverviewCredentials:         map[string]overview.Credentials{"portal": {ClientID: "console-gateway-portal", ClientSecret: "portal-only-secret-with-entropy", KeyID: "portal-active-key"}},
		AccountPortfolioAPIURL:      "https://account-portfolio.internal",
		AccountPortfolioCredentials: overview.Credentials{ClientID: "console-gateway", ClientSecret: platformSecret, KeyID: "account-console-key"},
	}
	if _, err := New(config); err == nil || !strings.Contains(err.Error(), "account portfolio secret") {
		t.Fatalf("New() error = %v, want account portfolio secret separation failure", err)
	}
	config.AccountPortfolioCredentials.ClientSecret = "account-portfolio-console-secret-with-entropy"
	if _, err := New(config); err != nil {
		t.Fatalf("New() rejected distinct Account Portfolio Console credentials: %v", err)
	}
}

func TestNewAcceptsProductionLibraryServiceConfiguration(t *testing.T) {
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = redisClient.Close() })
	config := Config{
		PlatformCoreURL: "http://platform-core:8081", PlatformAccountOrigin: "https://henukit.cn/account-auth",
		ClientID: "console-gateway", ClientSecret: "platform-oauth-secret-with-entropy", KeyID: "platform-active-key", RedirectURI: "https://console.henukit.cn/api/v1/auth/callback",
		SessionKey: []byte("0123456789abcdef0123456789abcdef"), Redis: redisClient, HTTPClient: &http.Client{},
		LibraryAPIURL: "http://library:8095",
		LibraryCredentials: overview.Credentials{
			ClientID: "console-gateway-library", ClientSecret: "library-summary-secret-with-entropy", KeyID: "library-summary-active",
		},
	}
	if _, err := New(config); err != nil {
		t.Fatalf("New() rejected the production Library service configuration: %v", err)
	}
}
