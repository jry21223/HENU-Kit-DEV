package server

import (
	"log/slog"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/health"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/middleware"
	"final-review-platform/services/api/pkg/response"
)

func NewRouter(cfg config.Config, log *slog.Logger, db *gorm.DB, cache *redislib.Client) *gin.Engine {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(middleware.RequestLogger(log))
	router.Use(middleware.Recover(log))
	router.Use(middleware.RateLimit(cfg.RateLimitRPS, cfg.RateLimitBurst))
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSAllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	healthHandler := health.NewHandler(cfg, db, cache)
	router.GET("/healthz", healthHandler.Healthz)

	v1 := router.Group("/api/v1")
	v1.GET("/healthz", healthHandler.Healthz)
	v1.GET("/version", func(ctx *gin.Context) {
		response.OK(ctx, gin.H{
			"service":     "final-review-api",
			"version":     cfg.Version,
			"environment": cfg.Environment,
		})
	})

	return router
}
