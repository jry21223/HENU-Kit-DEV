package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	LLMMode       string
}

func Load() Config {
	return Config{
		Environment:   env("APP_ENV", "development"),
		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		RedisPassword: env("REDIS_PASSWORD", ""),
		RedisDB:       intEnv("REDIS_DB", 0),
		LLMMode:       env("LLM_MODE", "mock"),
	}
}

func env(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
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
