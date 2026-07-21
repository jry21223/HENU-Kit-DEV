package config

import (
	"fmt"
	"os"
)

type ServiceAuth struct {
	ClientID     string
	ClientSecret string
	KeyID        string
}

type Config struct {
	ListenAddr string

	PlatformCoreURL   string
	PlatformClientID  string
	PlatformSecret    string
	PlatformKeyID     string
	PortalRedirectURI string
	SessionKey        []byte

	RedisURL string

	LibraryURL  string
	FoodURL     string
	PracticeURL string
	NoticeURL   string

	LibraryAuth  ServiceAuth
	FoodAuth     ServiceAuth
	PracticeAuth ServiceAuth
	NoticeAuth   ServiceAuth

	PortalOrigin string
}

func FromEnv() (Config, error) {
	sessionKey := envBytes("PORTAL_SESSION_KEY")
	if len(sessionKey) != 32 {
		return Config{}, fmt.Errorf("PORTAL_SESSION_KEY must be 32 bytes (base64)")
	}

	cfg := Config{
		ListenAddr:        envOrDefault("LISTEN_ADDR", ":8084"),
		PlatformCoreURL:   mustEnv("PLATFORM_CORE_URL"),
		PlatformClientID:  mustEnv("PLATFORM_CLIENT_ID"),
		PlatformSecret:    mustEnv("PLATFORM_CLIENT_SECRET"),
		PlatformKeyID:     mustEnv("PLATFORM_KEY_ID"),
		PortalRedirectURI: mustEnv("PORTAL_REDIRECT_URI"),
		SessionKey:        sessionKey,
		RedisURL:          envOrDefault("REDIS_URL", "redis://127.0.0.1:6379/2"),
		LibraryURL:        mustEnv("LIBRARY_SERVICE_URL"),
		FoodURL:           mustEnv("FOOD_SERVICE_URL"),
		PracticeURL:       mustEnv("PRACTICE_SERVICE_URL"),
		NoticeURL:         mustEnv("NOTICE_SERVICE_URL"),
		LibraryAuth: ServiceAuth{
			ClientID:     mustEnv("LIBRARY_CLIENT_ID"),
			ClientSecret: mustEnv("LIBRARY_CLIENT_SECRET"),
			KeyID:        mustEnv("LIBRARY_KEY_ID"),
		},
		FoodAuth: ServiceAuth{
			ClientID:     mustEnv("FOOD_CLIENT_ID"),
			ClientSecret: mustEnv("FOOD_CLIENT_SECRET"),
			KeyID:        mustEnv("FOOD_KEY_ID"),
		},
		PracticeAuth: ServiceAuth{
			ClientID:     mustEnv("PRACTICE_CLIENT_ID"),
			ClientSecret: mustEnv("PRACTICE_CLIENT_SECRET"),
			KeyID:        mustEnv("PRACTICE_KEY_ID"),
		},
		NoticeAuth: ServiceAuth{
			ClientID:     mustEnv("NOTICE_CLIENT_ID"),
			ClientSecret: mustEnv("NOTICE_CLIENT_SECRET"),
			KeyID:        mustEnv("NOTICE_KEY_ID"),
		},
		PortalOrigin: mustEnv("PORTAL_ORIGIN"),
	}
	return cfg, nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("missing required env var %s", key))
	}
	return v
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBytes(key string) []byte {
	v := os.Getenv(key)
	return []byte(v)
}
