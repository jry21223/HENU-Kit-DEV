package platformcore

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"henukit.dev/platform-core/internal/coordination"
	"henukit.dev/platform-core/internal/httpapi"
	"henukit.dev/platform-core/internal/identity"
	"henukit.dev/platform-core/internal/store"
)

type Config struct {
	Database                 *pgxpool.Pool
	Redis                    *redis.Client
	CoreCookieName           string
	AuthorizationTTL         time.Duration
	ExchangeSessionTTL       time.Duration
	IdempotencyEncryptionKey []byte
	IdempotencyTTL           time.Duration
	Logger                   *slog.Logger
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
	if config.AuthorizationTTL <= 0 {
		config.AuthorizationTTL = 90 * time.Second
	}
	if config.AuthorizationTTL < 60*time.Second || config.AuthorizationTTL > 120*time.Second {
		return nil, errors.New("authorization code TTL must be between 60s and 120s")
	}
	if config.ExchangeSessionTTL <= 0 {
		config.ExchangeSessionTTL = 5 * time.Minute
	}
	if config.ExchangeSessionTTL > 15*time.Minute {
		return nil, errors.New("exchange Session TTL must not exceed 15m")
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
	queries := store.New(config.Database)
	coordinator := coordination.NewRedis(config.Redis)
	flow := identity.New(queries, config.Database, coordinator, config.AuthorizationTTL, config.ExchangeSessionTTL, config.IdempotencyTTL, config.IdempotencyEncryptionKey)
	return httpapi.New(flow, config.Database, config.Redis, config.CoreCookieName, config.Logger), nil
}
