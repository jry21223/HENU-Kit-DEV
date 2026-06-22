package redis

import (
	"context"

	redislib "github.com/redis/go-redis/v9"

	"final-review-platform/services/api/pkg/config"
)

func Connect(ctx context.Context, cfg config.RedisConfig) (*redislib.Client, error) {
	client := redislib.NewClient(&redislib.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return client, nil
}
