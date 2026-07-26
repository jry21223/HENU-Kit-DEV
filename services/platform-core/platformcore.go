package platformcore

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/platform-core/internal/coordination"
	"henukit.dev/platform-core/internal/httpapi"
	"henukit.dev/platform-core/internal/identity"
	"henukit.dev/platform-core/internal/operationsinbox"
	"henukit.dev/platform-core/internal/password"
	"henukit.dev/platform-core/internal/platformoperations"
	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verification"
)

type Config struct {
	Database                  *pgxpool.Pool
	Redis                     *redis.Client
	CoreCookieName            string
	LocalCoreCookieName       string
	CoreSessionTTL            time.Duration
	AuthorizationTTL          time.Duration
	ExchangeSessionTTL        time.Duration
	IdempotencyEncryptionKey  []byte
	IdempotencyTTL            time.Duration
	Logger                    *slog.Logger
	VerificationEncryptionKey []byte
	StudentEmailDomains       []string
	VerificationCodeTTL       time.Duration
	VerificationResendDelay   time.Duration
	MailDeliveryWebhookToken  string
	MailDeliveryActiveKeyID   string
	MailDeliveryRetiringToken string
	MailDeliveryRetiringKeyID string
	TrustedProxyCIDRs         []string
	PasswordMemoryKiB         uint32
	PasswordIterations        uint32
	PasswordParallelism       uint8
	PasswordHashConcurrency   int
}

func New(config Config) (http.Handler, error) {
	if config.Database == nil || config.Redis == nil {
		return nil, errors.New("postgresql and Redis clients are required")
	}
	if config.CoreCookieName == "" {
		config.CoreCookieName = "__Host-henukit_core_session"
	}
	if !strings.HasPrefix(config.CoreCookieName, "__Host-") {
		return nil, errors.New("core session cookie name must use the __Host- prefix")
	}
	if config.LocalCoreCookieName == "" {
		config.LocalCoreCookieName = "henukit_core_session_local"
	}
	if strings.HasPrefix(config.LocalCoreCookieName, "__Host-") ||
		(&http.Cookie{Name: config.LocalCoreCookieName, Value: "valid"}).Valid() != nil {
		return nil, errors.New("local core session cookie name must be a valid non-__Host- name")
	}
	if config.CoreSessionTTL <= 0 {
		config.CoreSessionTTL = 15 * 24 * time.Hour
	}
	if config.CoreSessionTTL != 15*24*time.Hour {
		return nil, errors.New("core Session TTL must be 15 days")
	}
	if config.AuthorizationTTL <= 0 {
		config.AuthorizationTTL = 90 * time.Second
	}
	if config.AuthorizationTTL < 60*time.Second || config.AuthorizationTTL > 120*time.Second {
		return nil, errors.New("authorization code TTL must be between 60s and 120s")
	}
	if config.ExchangeSessionTTL <= 0 {
		config.ExchangeSessionTTL = 8 * time.Hour
	}
	if config.ExchangeSessionTTL > 8*time.Hour {
		return nil, errors.New("exchange Session TTL must not exceed 8h")
	}
	if len(config.IdempotencyEncryptionKey) != 32 {
		return nil, errors.New("idempotency encryption key must be 32 bytes")
	}
	if config.IdempotencyTTL <= 0 {
		config.IdempotencyTTL = 24 * time.Hour
	}
	if config.IdempotencyTTL < 24*time.Hour {
		return nil, errors.New("idempotency TTL must be at least 24h")
	}
	if config.Logger == nil {
		config.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if len(config.VerificationEncryptionKey) == 0 {
		mac := hmac.New(sha256.New, config.IdempotencyEncryptionKey)
		_, _ = mac.Write([]byte("henukit-verification-master"))
		config.VerificationEncryptionKey = mac.Sum(nil)
	}
	if len(config.StudentEmailDomains) == 0 {
		config.StudentEmailDomains = []string{"henu.edu.cn"}
	}
	if config.VerificationCodeTTL <= 0 {
		config.VerificationCodeTTL = 10 * time.Minute
	}
	if config.VerificationResendDelay <= 0 {
		config.VerificationResendDelay = 60 * time.Second
	}
	if config.MailDeliveryWebhookToken == "" {
		mac := hmac.New(sha256.New, config.IdempotencyEncryptionKey)
		_, _ = mac.Write([]byte("henukit-mail-delivery-webhook"))
		config.MailDeliveryWebhookToken = fmt.Sprintf("%x", mac.Sum(nil))
	}
	if len(config.MailDeliveryWebhookToken) < 32 {
		return nil, errors.New("mail delivery webhook token must contain at least 32 characters")
	}
	if config.MailDeliveryActiveKeyID == "" {
		config.MailDeliveryActiveKeyID = "mail-provider-active"
	}
	if (config.MailDeliveryRetiringToken == "") != (config.MailDeliveryRetiringKeyID == "") {
		return nil, errors.New("retiring mail delivery key id and token must be configured together")
	}
	if config.MailDeliveryRetiringToken != "" && (len(config.MailDeliveryRetiringToken) < 32 || config.MailDeliveryRetiringKeyID == config.MailDeliveryActiveKeyID) {
		return nil, errors.New("retiring mail delivery key must be distinct and contain at least 32 characters")
	}
	queries := store.New(config.Database)
	trustedProxies := make([]*net.IPNet, 0, len(config.TrustedProxyCIDRs))
	for _, value := range config.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q", value)
		}
		trustedProxies = append(trustedProxies, network)
	}
	deviceMAC := hmac.New(sha256.New, config.VerificationEncryptionKey)
	_, _ = deviceMAC.Write([]byte("henukit-device-cookie"))
	deviceKey := deviceMAC.Sum(nil)
	coordinator := coordination.NewRedis(config.Redis)
	passwordParameters := password.DefaultParameters()
	if config.PasswordMemoryKiB > 0 {
		passwordParameters.MemoryKiB = config.PasswordMemoryKiB
	}
	if config.PasswordIterations > 0 {
		passwordParameters.Iterations = config.PasswordIterations
	}
	if config.PasswordParallelism > 0 {
		passwordParameters.Parallelism = config.PasswordParallelism
	}
	if config.PasswordHashConcurrency <= 0 {
		config.PasswordHashConcurrency = 2
	}
	passwordManager, err := password.New(passwordParameters, config.PasswordHashConcurrency)
	if err != nil {
		return nil, err
	}
	flow := identity.New(queries, config.Database, coordinator, config.AuthorizationTTL, config.ExchangeSessionTTL, config.IdempotencyTTL)
	inbox := operationsinbox.New(queries, config.Database)
	platformOperations := platformoperations.New(queries, config.Database, config.Redis)
	verificationFlow, err := verification.New(queries, config.Database, coordinator, passwordManager, config.VerificationEncryptionKey, config.StudentEmailDomains, config.VerificationCodeTTL, config.VerificationResendDelay, config.CoreSessionTTL)
	if err != nil {
		return nil, err
	}
	deliveryKeys := map[string][]byte{config.MailDeliveryActiveKeyID: []byte(config.MailDeliveryWebhookToken)}
	if config.MailDeliveryRetiringToken != "" {
		deliveryKeys[config.MailDeliveryRetiringKeyID] = []byte(config.MailDeliveryRetiringToken)
	}
	return httpapi.New(flow, verificationFlow, inbox, platformOperations, queries, config.Database, config.Redis, config.CoreCookieName, config.LocalCoreCookieName, deliveryKeys, deviceKey, trustedProxies, config.Logger), nil
}
