package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	cfg   config.Config
	db    *gorm.DB
	cache *redislib.Client
}

func NewHandler(cfg config.Config, db *gorm.DB, cache *redislib.Client) Handler {
	return Handler{cfg: cfg, db: db, cache: cache}
}

func (h Handler) Healthz(ctx *gin.Context) {
	response.OK(ctx, gin.H{
		"service":      "final-review-api",
		"version":      h.cfg.Version,
		"environment":  h.cfg.Environment,
		"dependencies": h.dependencies(ctx.Request.Context()),
	})
}

func (h Handler) Readyz(ctx *gin.Context) {
	dependencies := h.dependencies(ctx.Request.Context())
	ready := dependencies["postgres"] == "ok" && dependencies["redis"] == "ok"
	payload := gin.H{
		"service":      "final-review-api",
		"version":      h.cfg.Version,
		"environment":  h.cfg.Environment,
		"ready":        ready,
		"dependencies": dependencies,
	}
	if ready {
		response.OK(ctx, payload)
		return
	}
	response.Error(ctx, http.StatusServiceUnavailable, response.CodeInternalServer, "not_ready", payload)
}

func (h Handler) dependencies(ctx context.Context) map[string]string {
	return map[string]string{
		"postgres": h.postgresStatus(ctx),
		"redis":    h.redisStatus(ctx),
	}
}

func (h Handler) postgresStatus(parent context.Context) string {
	if h.db == nil {
		return "not_configured"
	}
	sqlDB, err := h.db.DB()
	if err != nil {
		return "error"
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return "down"
	}
	return "ok"
}

func (h Handler) redisStatus(parent context.Context) string {
	if h.cache == nil {
		return "not_configured"
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := h.cache.Ping(ctx).Err(); err != nil {
		return "down"
	}
	return "ok"
}
