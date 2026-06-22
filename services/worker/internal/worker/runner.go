package worker

import (
	"context"
	"log/slog"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"final-review-platform/services/worker/pkg/config"
)

type Runner struct {
	cfg config.Config
	log *slog.Logger
}

func NewRunner(cfg config.Config, log *slog.Logger) Runner {
	return Runner{cfg: cfg, log: log}
}

func (r Runner) Run(ctx context.Context) error {
	client := redislib.NewClient(&redislib.Options{
		Addr:     r.cfg.RedisAddr,
		Password: r.cfg.RedisPassword,
		DB:       r.cfg.RedisDB,
	})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		return err
	}

	r.log.Info("worker started", slog.String("environment", r.cfg.Environment), slog.String("llm_mode", r.cfg.LLMMode))
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("worker shutdown requested")
			return nil
		case <-ticker.C:
			r.log.Debug("worker heartbeat")
		}
	}
}
