package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Environment   string
	DatabaseURL   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	LLMMode       string
	TaskStream    string
	EventStream   string
	HealthPort    string
	SMTPHost      string
	SMTPPort      string
	SMTPUsername  string
	SMTPPassword  string
	SMTPFrom      string
	Version       string
	CommitSHA     string
}

func Load() Config {
	return Config{
		Environment:   env("APP_ENV", "development"),
		DatabaseURL:   env("DATABASE_URL", "postgres://final_review:final_review_dev@localhost:5432/final_review_v2?sslmode=disable"),
		RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
		RedisPassword: env("REDIS_PASSWORD", ""),
		RedisDB:       intEnv("REDIS_DB", 0),
		LLMMode:       env("LLM_MODE", "mock"),
		TaskStream:    env("AI_TASK_STREAM", "ai_tasks"),
		EventStream:   env("PLATFORM_EVENT_STREAM", "platform_events"),
		HealthPort:    env("WORKER_HEALTH_PORT", "9090"),
		SMTPHost:      env("SMTP_HOST", ""),
		SMTPPort:      env("SMTP_PORT", "587"),
		SMTPUsername:  env("SMTP_USERNAME", ""),
		SMTPPassword:  env("SMTP_PASSWORD", ""),
		SMTPFrom:      env("SMTP_FROM", ""),
		Version:       env("APP_VERSION", "0.1.0"),
		CommitSHA:     env("GIT_COMMIT_SHA", "development"),
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
