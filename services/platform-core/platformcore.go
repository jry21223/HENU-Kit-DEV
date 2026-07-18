package platformcore

import (
	"errors"
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
	Database           *pgxpool.Pool
	Redis              *redis.Client
	CoreCookieName     string
	AuthorizationTTL   time.Duration
	ExchangeSessionTTL time.Duration
}

func New(config Config) (http.Handler, error) {
	if config.Database == nil || config.Redis == nil {
		return nil, errors.New("PostgreSQL and Redis are required")
	}
	if config.CoreCookieName == "" {
		config.CoreCookieName = "__Host-henukit_core_session"
	}
	if !strings.HasPrefix(config.CoreCookieName, "__Host-") {
		return nil, errors.New("Core Session cookie name must use the __Host- prefix")
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
	queries := store.New(config.Database)
	coordinator := coordination.NewRedis(config.Redis)
	flow := identity.New(queries, config.Database, coordinator, config.AuthorizationTTL, config.ExchangeSessionTTL)
	return httpapi.New(flow, config.Database, config.Redis, config.CoreCookieName), nil
}
