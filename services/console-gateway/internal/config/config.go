package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

type Config struct {
	ListenAddr          string
	PlatformCoreURL     string
	PlatformAuthorize   string
	ClientID            string
	ClientSecret        string
	KeyID               string
	RedirectURI         string
	RedisAddr           string
	SessionKey          []byte
	OverviewEndpoints   map[string]string
	OverviewCredentials map[string]SummaryCredentials
	NoticeAPIURL        string
	NoticeCredentials   SummaryCredentials
	LibraryAPIURL       string
	LibraryCredentials  SummaryCredentials
	FoodAPIURL          string
	FoodCredentials     SummaryCredentials
}

type SummaryCredentials struct{ ClientID, ClientSecret, KeyID string }

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
		OverviewEndpoints: map[string]string{
			"portal": os.Getenv("PORTAL_SUMMARY_URL"), "platform": os.Getenv("PLATFORM_SUMMARY_URL"), "notice": os.Getenv("NOTICE_SUMMARY_URL"),
			"library": os.Getenv("LIBRARY_SUMMARY_URL"), "quizcraft": os.Getenv("QUIZCRAFT_SUMMARY_URL"), "food": os.Getenv("FOOD_SUMMARY_URL"),
		},
		OverviewCredentials: map[string]SummaryCredentials{
			"portal": summaryCredentials("PORTAL"), "platform": summaryCredentials("PLATFORM"), "notice": summaryCredentials("NOTICE"),
			"library": summaryCredentials("LIBRARY"), "quizcraft": summaryCredentials("QUIZCRAFT"), "food": summaryCredentials("FOOD"),
		},
		NoticeAPIURL: strings.TrimRight(os.Getenv("NOTICE_API_URL"), "/"), NoticeCredentials: summaryCredentials("NOTICE"),
		LibraryAPIURL: strings.TrimRight(os.Getenv("LIBRARY_API_URL"), "/"), LibraryCredentials: summaryCredentials("LIBRARY"),
		FoodAPIURL: strings.TrimRight(os.Getenv("FOOD_API_URL"), "/"), FoodCredentials: summaryCredentials("FOOD"),
	}
	if config.ListenAddr == "" {
		config.ListenAddr = ":8082"
	}
	if config.PlatformCoreURL == "" || config.PlatformAuthorize == "" || config.ClientID == "" || config.ClientSecret == "" || config.KeyID == "" || config.RedirectURI == "" || config.RedisAddr == "" {
		return Config{}, errors.New("console gateway configuration is incomplete")
	}
	return config, nil
}

func summaryCredentials(module string) SummaryCredentials {
	prefix := module + "_SUMMARY_"
	return SummaryCredentials{ClientID: os.Getenv(prefix + "CLIENT_ID"), ClientSecret: os.Getenv(prefix + "CLIENT_SECRET"), KeyID: os.Getenv(prefix + "KEY_ID")}
}
