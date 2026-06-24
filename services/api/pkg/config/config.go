package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var (
	ErrCORSWildcardWithCredentials = errors.New("cors wildcard origin is not allowed with credentials")
	ErrProductionCORSRequired      = errors.New("production CORS_ALLOWED_ORIGINS must not be empty")
	ErrProductionCORSHTTPSRequired = errors.New("production CORS_ALLOWED_ORIGINS must use https origins")
	ErrInvalidCORSOrigin           = errors.New("invalid CORS origin")
)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type Config struct {
	Environment               string
	Port                      string
	Version                   string
	DatabaseURL               string
	Redis                     RedisConfig
	CORSAllowedOrigins        []string
	RateLimitRPS              float64
	RateLimitBurst            int
	AutoMigrate               bool
	DevFixedCode              string
	LocalUploadDir            string
	AITaskStream              string
	OperationLogRetentionDays int
	OperationLogExportLimit   int
	JWT                       JWTConfig
	WeChatPay                 WeChatPayConfig
	PaymentIncidentAlerts     PaymentIncidentAlertConfig
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

type WeChatPayConfig struct {
	Mode                   string
	APIBaseURL             string
	AppID                  string
	MchID                  string
	APIV3Key               string
	MerchantSerialNo       string
	MerchantPrivateKey     string
	MerchantPrivateKeyPath string
	PlatformCertsDir       string
	NotifyURL              string
	NativeExpireMinutes    int
}

type PaymentIncidentAlertConfig struct {
	WebhookURL     string
	WebhookSecret  string
	TimeoutSeconds int
}

func Load() Config {
	environment := env("APP_ENV", "development")
	return Config{
		Environment:               environment,
		Port:                      env("API_PORT", "8080"),
		Version:                   env("APP_VERSION", "0.1.0"),
		DatabaseURL:               env("DATABASE_URL", "postgres://final_review:final_review_dev@localhost:5432/final_review_v2?sslmode=disable"),
		Redis:                     loadRedisConfig(),
		CORSAllowedOrigins:        csvEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173"),
		RateLimitRPS:              floatEnv("RATE_LIMIT_RPS", 20),
		RateLimitBurst:            intEnv("RATE_LIMIT_BURST", 40),
		AutoMigrate:               boolEnv("AUTO_MIGRATE", true),
		DevFixedCode:              devFixedCode(environment),
		LocalUploadDir:            env("LOCAL_UPLOAD_DIR", "uploads"),
		AITaskStream:              env("AI_TASK_STREAM", "ai_tasks"),
		OperationLogRetentionDays: intEnv("OPERATION_LOG_RETENTION_DAYS", 180),
		OperationLogExportLimit:   intEnv("OPERATION_LOG_EXPORT_LIMIT", 5000),
		JWT: JWTConfig{
			Issuer:           env("JWT_ISSUER", "final-review-platform"),
			AccessTTLMinutes: intEnv("JWT_ACCESS_TTL_MINUTES", 15),
			RefreshTTLHours:  intEnv("JWT_REFRESH_TTL_HOURS", 168),
			PrivateKeyPEM:    env("JWT_PRIVATE_KEY", ""),
			PrivateKeyPath:   env("JWT_PRIVATE_KEY_PATH", ""),
			PublicKeyPEM:     env("JWT_PUBLIC_KEY", ""),
			PublicKeyPath:    env("JWT_PUBLIC_KEY_PATH", ""),
		},
		WeChatPay: WeChatPayConfig{
			Mode:                   env("WECHAT_PAY_MODE", "mock"),
			APIBaseURL:             env("WECHAT_PAY_API_BASE_URL", "https://api.mch.weixin.qq.com"),
			AppID:                  env("WECHAT_PAY_APPID", ""),
			MchID:                  env("WECHAT_PAY_MCH_ID", ""),
			APIV3Key:               env("WECHAT_PAY_API_V3_KEY", ""),
			MerchantSerialNo:       env("WECHAT_PAY_MERCHANT_SERIAL_NO", ""),
			MerchantPrivateKey:     env("WECHAT_PAY_MERCHANT_PRIVATE_KEY", ""),
			MerchantPrivateKeyPath: env("WECHAT_PAY_MERCHANT_PRIVATE_KEY_PATH", ""),
			PlatformCertsDir:       env("WECHAT_PAY_PLATFORM_CERTS_DIR", ""),
			NotifyURL:              env("WECHAT_PAY_NOTIFY_URL", "http://localhost:8080/api/v1/payments/wechat/notify"),
			NativeExpireMinutes:    intEnv("WECHAT_PAY_NATIVE_EXPIRE_MINUTES", 15),
		},
		PaymentIncidentAlerts: PaymentIncidentAlertConfig{
			WebhookURL:     env("PAYMENT_INCIDENT_WEBHOOK_URL", ""),
			WebhookSecret:  env("PAYMENT_INCIDENT_WEBHOOK_SECRET", ""),
			TimeoutSeconds: intEnv("PAYMENT_INCIDENT_WEBHOOK_TIMEOUT_SECONDS", 3),
		},
	}
}

func ValidateHTTPConfig(cfg Config) error {
	validOrigins := 0
	for _, origin := range cfg.CORSAllowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			return ErrCORSWildcardWithCredentials
		}
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return ErrInvalidCORSOrigin
		}
		if strings.EqualFold(cfg.Environment, "production") && parsed.Scheme != "https" {
			return ErrProductionCORSHTTPSRequired
		}
		validOrigins++
	}
	if validOrigins == 0 && strings.EqualFold(cfg.Environment, "production") {
		return ErrProductionCORSRequired
	}
	return nil
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

func envAllowEmpty(key string, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func devFixedCode(environment string) string {
	fallback := "123456"
	if strings.EqualFold(strings.TrimSpace(environment), "production") {
		fallback = ""
	}
	return envAllowEmpty("DEV_FIXED_VERIFICATION_CODE", fallback)
}

func csvEnv(key string, fallback string) []string {
	raw := env(key, fallback)
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
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
