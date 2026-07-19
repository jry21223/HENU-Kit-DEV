package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

type Config struct {
	ListenAddr        string
	PlatformCoreURL   string
	PlatformAuthorize string
	ClientID          string
	ClientSecret      string
	KeyID             string
	RedirectURI       string
	RedisAddr         string
	SessionKey        []byte
}

func FromEnv() (Config, error) {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("CONSOLE_SESSION_KEY"))
	if err != nil || len(key) != 32 {
		return Config{}, errors.New("CONSOLE_SESSION_KEY must be base64 for exactly 32 bytes")
	}
	config := Config{
		ListenAddr: os.Getenv("LISTEN_ADDR"), PlatformCoreURL: strings.TrimRight(os.Getenv("PLATFORM_CORE_URL"), "/"),
		PlatformAuthorize: strings.TrimRight(os.Getenv("PLATFORM_ACCOUNT_ORIGIN"), "/"), ClientID: os.Getenv("PLATFORM_CLIENT_ID"),
		ClientSecret: os.Getenv("PLATFORM_CLIENT_SECRET"), KeyID: os.Getenv("PLATFORM_KEY_ID"), RedirectURI: os.Getenv("CONSOLE_REDIRECT_URI"),
		RedisAddr: os.Getenv("REDIS_ADDR"), SessionKey: key,
	}
	if config.ListenAddr == "" {
		config.ListenAddr = ":8082"
	}
	if config.PlatformCoreURL == "" || config.PlatformAuthorize == "" || config.ClientID == "" || config.ClientSecret == "" || config.KeyID == "" || config.RedirectURI == "" || config.RedisAddr == "" {
		return Config{}, errors.New("console gateway configuration is incomplete")
	}
	return config, nil
}
