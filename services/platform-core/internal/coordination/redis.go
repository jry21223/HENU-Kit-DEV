package coordination

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrBusy = errors.New("coordination lock is busy")

type Redis struct {
	client *redis.Client
}

func NewRedis(client *redis.Client) *Redis {
	return &Redis{client: client}
}

func (r *Redis) UseOnce(ctx context.Context, key string, ttl time.Duration) error {
	stored, err := r.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return err
	}
	if !stored {
		return ErrBusy
	}
	return nil
}

func (r *Redis) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	if key == "" || limit <= 0 || window <= 0 {
		return false, errors.New("rate limit key, limit, and window are required")
	}
	const script = `
local count = redis.call("INCR", KEYS[1])
if count == 1 then redis.call("PEXPIRE", KEYS[1], ARGV[1]) end
if count > tonumber(ARGV[2]) then return 0 end
return 1`
	result, err := r.client.Eval(ctx, script, []string{key}, window.Milliseconds(), limit).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (r *Redis) FailureCount(ctx context.Context, key string) (int64, error) {
	value, err := r.client.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return value, err
}

func (r *Redis) RecordFailure(ctx context.Context, key string, window time.Duration) (int64, error) {
	if key == "" || window <= 0 {
		return 0, errors.New("failure key and window are required")
	}
	const script = `
local count = redis.call("INCR", KEYS[1])
if count == 1 then redis.call("PEXPIRE", KEYS[1], ARGV[1]) end
return count`
	return r.client.Eval(ctx, script, []string{key}, window.Milliseconds()).Int64()
}

func (r *Redis) Clear(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.client.Del(ctx, keys...).Err()
}

func (r *Redis) Acquire(ctx context.Context, key string, ttl time.Duration) (func(context.Context) error, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	acquired, err := r.client.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, ErrBusy
	}
	return func(releaseContext context.Context) error {
		const releaseScript = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
		return r.client.Eval(releaseContext, releaseScript, []string{key}, token).Err()
	}, nil
}
