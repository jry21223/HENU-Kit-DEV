package config

import (
	"encoding/base64"
	"testing"
)

func TestFromEnvReadsDedicatedAccountPortfolioConsoleCredentials(t *testing.T) {
	t.Setenv("CONSOLE_SESSION_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("PLATFORM_CORE_URL", "https://account.henukit.test")
	t.Setenv("PLATFORM_ACCOUNT_ORIGIN", "https://henukit.test/account-auth")
	t.Setenv("PLATFORM_CLIENT_ID", "console-gateway")
	t.Setenv("PLATFORM_CLIENT_SECRET", "platform-oauth-secret-with-entropy")
	t.Setenv("PLATFORM_KEY_ID", "platform-key")
	t.Setenv("CONSOLE_REDIRECT_URI", "https://console.henukit.test/api/v1/auth/callback")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("ACCOUNT_PORTFOLIO_API_URL", "https://account-portfolio.internal")
	t.Setenv("ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID", "console-gateway")
	t.Setenv("ACCOUNT_PORTFOLIO_CONSOLE_KEY_ID", "account-console-key")
	t.Setenv("ACCOUNT_PORTFOLIO_CONSOLE_SECRET", "account-portfolio-console-secret-with-entropy")

	config, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.AccountPortfolioAPIURL != "https://account-portfolio.internal" || config.AccountPortfolioCredentials.ClientID != "console-gateway" || config.AccountPortfolioCredentials.KeyID != "account-console-key" || config.AccountPortfolioCredentials.ClientSecret != "account-portfolio-console-secret-with-entropy" {
		t.Fatalf("Account Portfolio Console config = %+v", config)
	}
}

func TestFromEnvRejectsPartialAccountPortfolioConsoleConfiguration(t *testing.T) {
	t.Setenv("CONSOLE_SESSION_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("PLATFORM_CORE_URL", "https://account.henukit.test")
	t.Setenv("PLATFORM_ACCOUNT_ORIGIN", "https://henukit.test/account-auth")
	t.Setenv("PLATFORM_CLIENT_ID", "console-gateway")
	t.Setenv("PLATFORM_CLIENT_SECRET", "platform-oauth-secret-with-entropy")
	t.Setenv("PLATFORM_KEY_ID", "platform-key")
	t.Setenv("CONSOLE_REDIRECT_URI", "https://console.henukit.test/api/v1/auth/callback")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("ACCOUNT_PORTFOLIO_API_URL", "https://account-portfolio.internal")
	t.Setenv("ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID", "console-gateway")

	if _, err := FromEnv(); err == nil {
		t.Fatal("FromEnv() accepted a partial Account Portfolio Console credential tuple")
	}
}

func TestFromEnvRejectsAccountPortfolioConsolePlaceholderWhenStrongSecretsAreRequired(t *testing.T) {
	t.Setenv("CONSOLE_SESSION_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	t.Setenv("PLATFORM_CORE_URL", "https://account.henukit.test")
	t.Setenv("PLATFORM_ACCOUNT_ORIGIN", "https://henukit.test/account-auth")
	t.Setenv("PLATFORM_CLIENT_ID", "console-gateway")
	t.Setenv("PLATFORM_CLIENT_SECRET", "platform-oauth-secret-with-entropy")
	t.Setenv("PLATFORM_KEY_ID", "platform-key")
	t.Setenv("CONSOLE_REDIRECT_URI", "https://console.henukit.test/api/v1/auth/callback")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("ACCOUNT_PORTFOLIO_API_URL", "https://account-portfolio.internal")
	t.Setenv("ACCOUNT_PORTFOLIO_CONSOLE_CLIENT_ID", "console-gateway")
	t.Setenv("ACCOUNT_PORTFOLIO_CONSOLE_KEY_ID", "account-console-key")
	t.Setenv("ACCOUNT_PORTFOLIO_CONSOLE_SECRET", "replace-account-portfolio-console-secret")
	t.Setenv("ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET", "1")

	if _, err := FromEnv(); err == nil {
		t.Fatal("FromEnv() accepted an Account Portfolio Console placeholder secret in strong-secret mode")
	}
}
