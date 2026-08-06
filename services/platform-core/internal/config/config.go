package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address                     string
	DatabaseURL                 string
	RedisURL                    string
	CoreCookieName              string
	LocalCoreCookieName         string
	CoreSessionTTL              time.Duration
	AuthorizationTTL            time.Duration
	ExchangeSessionTTL          time.Duration
	ExchangeSessionTTLOverrides map[string]time.Duration
	IdempotencyEncryptionKey    []byte
	IdempotencyTTL              time.Duration
	VerificationKey             []byte
	StudentEmailDomains         []string
	VerificationCodeTTL         time.Duration
	VerificationResendDelay     time.Duration
	MailDeliveryWebhookToken    string
	MailDeliveryActiveKeyID     string
	MailDeliveryRetiringToken   string
	MailDeliveryRetiringKeyID   string
	TrustedProxyCIDRs           []string
	PasswordMemoryKiB           uint32
	PasswordIterations          uint32
	PasswordParallelism         uint8
	PasswordHashConcurrency     int
}

func Load() (Config, error) {
	passwordMemoryKiB := intEnv("PLATFORM_CORE_PASSWORD_MEMORY_KIB", 64*1024)
	passwordIterations := intEnv("PLATFORM_CORE_PASSWORD_ITERATIONS", 3)
	passwordParallelism := intEnv("PLATFORM_CORE_PASSWORD_PARALLELISM", 1)
	config := Config{
		Address:                     env("PLATFORM_CORE_ADDRESS", ":8081"),
		DatabaseURL:                 os.Getenv("PLATFORM_CORE_DATABASE_URL"),
		RedisURL:                    env("PLATFORM_CORE_REDIS_URL", "redis://localhost:6379/0"),
		CoreCookieName:              env("PLATFORM_CORE_COOKIE_NAME", "__Host-henukit_core_session"),
		LocalCoreCookieName:         env("PLATFORM_CORE_LOCAL_COOKIE_NAME", "henukit_core_session_local"),
		CoreSessionTTL:              durationEnv("PLATFORM_CORE_CORE_SESSION_TTL", 30*24*time.Hour),
		AuthorizationTTL:            durationEnv("PLATFORM_CORE_AUTHORIZATION_TTL", 90*time.Second),
		ExchangeSessionTTL:          durationEnv("PLATFORM_CORE_EXCHANGE_SESSION_TTL", 8*time.Hour),
		ExchangeSessionTTLOverrides: make(map[string]time.Duration),
		IdempotencyTTL:              durationEnv("PLATFORM_CORE_IDEMPOTENCY_TTL", 24*time.Hour),
		StudentEmailDomains:         strings.Split(env("PLATFORM_CORE_STUDENT_EMAIL_DOMAINS", "henu.edu.cn"), ","),
		VerificationCodeTTL:         durationEnv("PLATFORM_CORE_VERIFICATION_CODE_TTL", 10*time.Minute),
		VerificationResendDelay:     durationEnv("PLATFORM_CORE_VERIFICATION_RESEND_DELAY", 60*time.Second),
		MailDeliveryWebhookToken:    os.Getenv("PLATFORM_CORE_MAIL_DELIVERY_TOKEN"),
		MailDeliveryActiveKeyID:     env("PLATFORM_CORE_MAIL_DELIVERY_KEY_ID", "mail-provider-active"),
		MailDeliveryRetiringToken:   os.Getenv("PLATFORM_CORE_MAIL_DELIVERY_RETIRING_TOKEN"),
		MailDeliveryRetiringKeyID:   os.Getenv("PLATFORM_CORE_MAIL_DELIVERY_RETIRING_KEY_ID"),
		TrustedProxyCIDRs:           splitNonEmpty(os.Getenv("PLATFORM_CORE_TRUSTED_PROXY_CIDRS")),
		PasswordMemoryKiB:           uint32(passwordMemoryKiB),
		PasswordIterations:          uint32(passwordIterations),
		PasswordParallelism:         uint8(passwordParallelism),
		PasswordHashConcurrency:     intEnv("PLATFORM_CORE_PASSWORD_HASH_CONCURRENCY", 2),
	}
	if config.DatabaseURL == "" {
		return Config{}, errors.New("PLATFORM_CORE_DATABASE_URL is required")
	}
	overrides, err := exchangeSessionTTLOverrides()
	if err != nil {
		return Config{}, err
	}
	config.ExchangeSessionTTLOverrides = overrides
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
	// 30 days is a deliberate long-lived session: students get a stay-signed-in
	// Portal without re-entering the email code, at the cost of a longer theft
	// window if a cookie is stolen. The cookie stays HttpOnly+Secure and every
	// permission check still validates the server-side Core Session, so a
	// revocation on the account origin kills the Portal session too.
	if config.CoreSessionTTL != 30*24*time.Hour {
		return Config{}, errors.New("PLATFORM_CORE_CORE_SESSION_TTL must be 720h")
	}
	if config.ExchangeSessionTTL <= 0 || config.ExchangeSessionTTL > 8*time.Hour {
		return Config{}, errors.New("PLATFORM_CORE_EXCHANGE_SESSION_TTL must be greater than zero and at most 8h")
	}
	if err := ValidateExchangeSessionTTLOverrides(config.ExchangeSessionTTLOverrides); err != nil {
		return Config{}, fmt.Errorf("PLATFORM_CORE_EXCHANGE_SESSION_TTL_OVERRIDES: %w", err)
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
	if len(config.StudentEmailDomains) != 1 || strings.ToLower(strings.TrimSpace(config.StudentEmailDomains[0])) != "henu.edu.cn" {
		return Config{}, errors.New("PLATFORM_CORE_STUDENT_EMAIL_DOMAINS must be exactly henu.edu.cn")
	}
	if len(config.MailDeliveryWebhookToken) < 32 {
		return Config{}, errors.New("PLATFORM_CORE_MAIL_DELIVERY_TOKEN must contain at least 32 characters")
	}
	if (config.MailDeliveryRetiringToken == "") != (config.MailDeliveryRetiringKeyID == "") || (config.MailDeliveryRetiringToken != "" && len(config.MailDeliveryRetiringToken) < 32) {
		return Config{}, errors.New("retiring mail delivery key id and 32-character token must be configured together")
	}
	if passwordMemoryKiB < 32*1024 || passwordMemoryKiB > 1024*1024 ||
		passwordIterations < 1 || passwordIterations > 10 ||
		passwordParallelism < 1 || passwordParallelism > 16 ||
		config.PasswordHashConcurrency < 1 || config.PasswordHashConcurrency > 32 {
		return Config{}, errors.New("password Argon2id parameters are outside the accepted security bounds")
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

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
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

// ValidateExchangeSessionTTLOverrides rejects empty client ids and non-positive
// or longer-than-30-day durations. Both config.Load and platformcore.New enforce
// the same contract, so the check lives here once.
func ValidateExchangeSessionTTLOverrides(overrides map[string]time.Duration) error {
	for clientID, ttl := range overrides {
		if clientID == "" || ttl <= 0 || ttl > 30*24*time.Hour {
			return errors.New("exchange Session TTL overrides must map client ids to positive durations of at most 720h")
		}
	}
	return nil
}

// exchangeSessionTTLOverrides parses PLATFORM_CORE_EXCHANGE_SESSION_TTL_OVERRIDES,
// a comma-separated list of client_id=ttl pairs (e.g.
// "portal-gateway=720h"). The default 8-hour exchange Session stays the
// baseline for every OAuth client; the Portal client overrides it to 30 days
// so the Portal Session cookie and its permission checks survive for the whole
// Core Session window. Console keeps its short high-privilege sessions.
func exchangeSessionTTLOverrides() (map[string]time.Duration, error) {
	overrides := make(map[string]time.Duration)
	for _, entry := range splitNonEmpty(os.Getenv("PLATFORM_CORE_EXCHANGE_SESSION_TTL_OVERRIDES")) {
		clientID, rawTTL, ok := strings.Cut(entry, "=")
		clientID = strings.TrimSpace(clientID)
		if !ok || clientID == "" {
			return nil, errors.New("PLATFORM_CORE_EXCHANGE_SESSION_TTL_OVERRIDES entries must be client_id=ttl pairs")
		}
		ttl, err := time.ParseDuration(strings.TrimSpace(rawTTL))
		if err != nil {
			return nil, errors.New("PLATFORM_CORE_EXCHANGE_SESSION_TTL_OVERRIDES contains an invalid duration")
		}
		overrides[clientID] = ttl
	}
	return overrides, nil
}
