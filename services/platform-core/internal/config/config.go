package config

import (
	"encoding/base64"
	"errors"
	"os"
	"time"
)

type Config struct {
	Address                  string
	DatabaseURL              string
	RedisURL                 string
	CoreCookieName           string
	AuthorizationTTL         time.Duration
	ExchangeSessionTTL       time.Duration
	IdempotencyEncryptionKey []byte
	IdempotencyTTL           time.Duration
}

func Load() (Config, error) {
	config := Config{
		Address:            env("PLATFORM_CORE_ADDRESS", ":8081"),
		DatabaseURL:        os.Getenv("PLATFORM_CORE_DATABASE_URL"),
		RedisURL:           env("PLATFORM_CORE_REDIS_URL", "redis://localhost:6379/0"),
		CoreCookieName:     env("PLATFORM_CORE_COOKIE_NAME", "__Host-henukit_core_session"),
		AuthorizationTTL:   durationEnv("PLATFORM_CORE_AUTHORIZATION_TTL", 90*time.Second),
		ExchangeSessionTTL: durationEnv("PLATFORM_CORE_EXCHANGE_SESSION_TTL", 5*time.Minute),
		IdempotencyTTL:     durationEnv("PLATFORM_CORE_IDEMPOTENCY_TTL", 24*time.Hour),
	}
	if config.DatabaseURL == "" {
		return Config{}, errors.New("PLATFORM_CORE_DATABASE_URL is required")
	}
	encodedKey := os.Getenv("PLATFORM_CORE_IDEMPOTENCY_KEY")
	decodedKey, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(decodedKey) != 32 {
		return Config{}, errors.New("PLATFORM_CORE_IDEMPOTENCY_KEY must be base64 for exactly 32 bytes")
	}
	config.IdempotencyEncryptionKey = decodedKey
	if config.AuthorizationTTL < 60*time.Second || config.AuthorizationTTL > 120*time.Second {
		return Config{}, errors.New("PLATFORM_CORE_AUTHORIZATION_TTL must be between 60s and 120s")
	}
	if config.ExchangeSessionTTL <= 0 || config.ExchangeSessionTTL > 15*time.Minute {
		return Config{}, errors.New("PLATFORM_CORE_EXCHANGE_SESSION_TTL must be greater than zero and at most 15m")
	}
	if config.IdempotencyTTL < 24*time.Hour {
		return Config{}, errors.New("PLATFORM_CORE_IDEMPOTENCY_TTL must be at least 24h")
	}
	return config, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return -1
	}
	return parsed
}
