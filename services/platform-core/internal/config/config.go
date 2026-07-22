package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Address                   string
	DatabaseURL               string
	RedisURL                  string
	CoreCookieName            string
	CoreSessionTTL            time.Duration
	AuthorizationTTL          time.Duration
	ExchangeSessionTTL        time.Duration
	IdempotencyEncryptionKey  []byte
	IdempotencyTTL            time.Duration
	VerificationKey           []byte
	StudentEmailDomains       []string
	VerificationCodeTTL       time.Duration
	VerificationResendDelay   time.Duration
	MailDeliveryWebhookToken  string
	MailDeliveryActiveKeyID   string
	MailDeliveryRetiringToken string
	MailDeliveryRetiringKeyID string
	TrustedProxyCIDRs         []string
}

func Load() (Config, error) {
	config := Config{
		Address:                   env("PLATFORM_CORE_ADDRESS", ":8081"),
		DatabaseURL:               os.Getenv("PLATFORM_CORE_DATABASE_URL"),
		RedisURL:                  env("PLATFORM_CORE_REDIS_URL", "redis://localhost:6379/0"),
		CoreCookieName:            env("PLATFORM_CORE_COOKIE_NAME", "__Host-henukit_core_session"),
		CoreSessionTTL:            durationEnv("PLATFORM_CORE_CORE_SESSION_TTL", 15*24*time.Hour),
		AuthorizationTTL:          durationEnv("PLATFORM_CORE_AUTHORIZATION_TTL", 90*time.Second),
		ExchangeSessionTTL:        durationEnv("PLATFORM_CORE_EXCHANGE_SESSION_TTL", 5*time.Minute),
		IdempotencyTTL:            durationEnv("PLATFORM_CORE_IDEMPOTENCY_TTL", 24*time.Hour),
		StudentEmailDomains:       strings.Split(env("PLATFORM_CORE_STUDENT_EMAIL_DOMAINS", "henu.edu.cn"), ","),
		VerificationCodeTTL:       durationEnv("PLATFORM_CORE_VERIFICATION_CODE_TTL", 10*time.Minute),
		VerificationResendDelay:   durationEnv("PLATFORM_CORE_VERIFICATION_RESEND_DELAY", 60*time.Second),
		MailDeliveryWebhookToken:  os.Getenv("PLATFORM_CORE_MAIL_DELIVERY_TOKEN"),
		MailDeliveryActiveKeyID:   env("PLATFORM_CORE_MAIL_DELIVERY_KEY_ID", "mail-provider-active"),
		MailDeliveryRetiringToken: os.Getenv("PLATFORM_CORE_MAIL_DELIVERY_RETIRING_TOKEN"),
		MailDeliveryRetiringKeyID: os.Getenv("PLATFORM_CORE_MAIL_DELIVERY_RETIRING_KEY_ID"),
		TrustedProxyCIDRs:         splitNonEmpty(os.Getenv("PLATFORM_CORE_TRUSTED_PROXY_CIDRS")),
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
	verificationKey, err := base64.StdEncoding.DecodeString(os.Getenv("PLATFORM_CORE_VERIFICATION_KEY"))
	if err != nil || len(verificationKey) != 32 {
		return Config{}, errors.New("PLATFORM_CORE_VERIFICATION_KEY must be base64 for exactly 32 bytes")
	}
	config.VerificationKey = verificationKey
	if config.AuthorizationTTL < 60*time.Second || config.AuthorizationTTL > 120*time.Second {
		return Config{}, errors.New("PLATFORM_CORE_AUTHORIZATION_TTL must be between 60s and 120s")
	}
	if config.CoreSessionTTL != 15*24*time.Hour {
		return Config{}, errors.New("PLATFORM_CORE_CORE_SESSION_TTL must be 360h")
	}
	if config.ExchangeSessionTTL <= 0 || config.ExchangeSessionTTL > 15*time.Minute {
		return Config{}, errors.New("PLATFORM_CORE_EXCHANGE_SESSION_TTL must be greater than zero and at most 15m")
	}
	if config.IdempotencyTTL < 24*time.Hour {
		return Config{}, errors.New("PLATFORM_CORE_IDEMPOTENCY_TTL must be at least 24h")
	}
	if config.VerificationCodeTTL < 5*time.Minute || config.VerificationCodeTTL > 10*time.Minute {
		return Config{}, errors.New("PLATFORM_CORE_VERIFICATION_CODE_TTL must be between 5m and 10m")
	}
	if config.VerificationResendDelay < 60*time.Second {
		return Config{}, errors.New("PLATFORM_CORE_VERIFICATION_RESEND_DELAY must be at least 60s")
	}
	if len(config.MailDeliveryWebhookToken) < 32 {
		return Config{}, errors.New("PLATFORM_CORE_MAIL_DELIVERY_TOKEN must contain at least 32 characters")
	}
	if (config.MailDeliveryRetiringToken == "") != (config.MailDeliveryRetiringKeyID == "") || (config.MailDeliveryRetiringToken != "" && len(config.MailDeliveryRetiringToken) < 32) {
		return Config{}, errors.New("retiring mail delivery key id and 32-character token must be configured together")
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

func splitNonEmpty(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}
