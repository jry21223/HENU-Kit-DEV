package config

import (
	"os"
	"strconv"
	"strings"
)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type Config struct {
	Environment        string
	Port               string
	Version            string
	DatabaseURL        string
	Redis              RedisConfig
	CORSAllowedOrigins []string
	RateLimitRPS       float64
	RateLimitBurst     int
	AutoMigrate        bool
	DevFixedCode       string
	JWT                JWTConfig
}

type JWTConfig struct {
	Issuer           string
	AccessTTLMinutes int
	RefreshTTLHours  int
	PrivateKeyPEM    string
	PrivateKeyPath   string
	PublicKeyPEM     string
	PublicKeyPath    string
}

func Load() Config {
	return Config{
		Environment:        env("APP_ENV", "development"),
		Port:               env("API_PORT", "8080"),
		Version:            env("APP_VERSION", "0.1.0"),
		DatabaseURL:        env("DATABASE_URL", "postgres://final_review:final_review_dev@localhost:5432/final_review_v2?sslmode=disable"),
		Redis:              loadRedisConfig(),
		CORSAllowedOrigins: csvEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		RateLimitRPS:       floatEnv("RATE_LIMIT_RPS", 20),
		RateLimitBurst:     intEnv("RATE_LIMIT_BURST", 40),
		AutoMigrate:        boolEnv("AUTO_MIGRATE", true),
		DevFixedCode:       env("DEV_FIXED_VERIFICATION_CODE", "123456"),
		JWT: JWTConfig{
			Issuer:           env("JWT_ISSUER", "final-review-platform"),
			AccessTTLMinutes: intEnv("JWT_ACCESS_TTL_MINUTES", 15),
			RefreshTTLHours:  intEnv("JWT_REFRESH_TTL_HOURS", 168),
			PrivateKeyPEM:    env("JWT_PRIVATE_KEY", ""),
			PrivateKeyPath:   env("JWT_PRIVATE_KEY_PATH", ""),
			PublicKeyPEM:     env("JWT_PUBLIC_KEY", ""),
			PublicKeyPath:    env("JWT_PUBLIC_KEY_PATH", ""),
		},
	}
}

func loadRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:     env("REDIS_ADDR", "localhost:6379"),
		Password: env("REDIS_PASSWORD", ""),
		DB:       intEnv("REDIS_DB", 0),
	}
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func csvEnv(key string, fallback string) []string {
	raw := env(key, fallback)
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" && value != "*" {
			values = append(values, value)
		}
	}
	return values
}

func intEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func boolEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func floatEnv(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
