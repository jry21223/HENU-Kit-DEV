package portalgateway

import (
	"fmt"
	"net/http"

	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/httpapi"
)

// New creates the Portal Gateway http.Handler.
func New(cfg config.Config) (http.Handler, error) {
	rdb, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("redis.ParseURL: %w", err)
	}
	handler, err := httpapi.New(cfg, redis.NewClient(rdb))
	if err != nil {
		return nil, fmt.Errorf("httpapi.New: %w", err)
	}
	return handler.Router(), nil
}
