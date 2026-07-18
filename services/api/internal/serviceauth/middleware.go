package serviceauth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"final-review-platform/services/api/pkg/response"
)

const maxClockSkew = 5 * time.Minute

type Verifier struct {
	store nonceStore
	keys  map[string]string
	now   func() time.Time
}

type nonceStore interface {
	Claim(context.Context, string, time.Duration) (bool, error)
}

type redisNonceStore struct{ cache *redis.Client }

func (store redisNonceStore) Claim(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return store.cache.SetNX(ctx, key, "1", ttl).Result()
}

func New(cache *redis.Client, keys map[string]string) Verifier {
	var store nonceStore
	if cache != nil {
		store = redisNonceStore{cache: cache}
	}
	return Verifier{store: store, keys: keys, now: time.Now}
}

func (v Verifier) Require() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		serviceID := strings.TrimSpace(ctx.GetHeader("X-Service-Id"))
		keyID := strings.TrimSpace(ctx.GetHeader("X-Key-Id"))
		timestampText := strings.TrimSpace(ctx.GetHeader("X-Timestamp"))
		nonce := strings.TrimSpace(ctx.GetHeader("X-Nonce"))
		signature := strings.ToLower(strings.TrimSpace(ctx.GetHeader("X-Signature")))
		secret, exists := v.keys[serviceID+":"+keyID]
		if !exists || len(nonce) < 8 || signature == "" {
			v.deny(ctx, "invalid_service_signature")
			return
		}
		timestamp, err := time.Parse(time.RFC3339, timestampText)
		if err != nil || v.now().UTC().Sub(timestamp.UTC()).Abs() > maxClockSkew {
			v.deny(ctx, "service_timestamp_out_of_range")
			return
		}
		body, err := io.ReadAll(ctx.Request.Body)
		if err != nil {
			v.deny(ctx, "service_request_unreadable")
			return
		}
		ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
		bodyHash := sha256.Sum256(body)
		canonical := strings.Join([]string{ctx.Request.Method, ctx.Request.URL.RequestURI(), timestampText, nonce, hex.EncodeToString(bodyHash[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		expected := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(signature)) {
			v.deny(ctx, "invalid_service_signature")
			return
		}
		if v.store == nil {
			response.Error(ctx, http.StatusServiceUnavailable, response.CodeInternalServer, "service_auth_replay_store_unavailable", nil)
			ctx.Abort()
			return
		}
		accepted, err := v.store.Claim(ctx, "service_nonce:"+serviceID+":"+keyID+":"+nonce, maxClockSkew)
		if err != nil {
			response.Error(ctx, http.StatusServiceUnavailable, response.CodeInternalServer, "service_auth_replay_store_unavailable", nil)
			ctx.Abort()
			return
		}
		if !accepted {
			v.deny(ctx, "service_nonce_replayed")
			return
		}
		ctx.Set("service_id", serviceID)
		ctx.Set("service_key_id", keyID)
		ctx.Next()
	}
}

func (v Verifier) deny(ctx *gin.Context, message string) {
	response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, message, nil)
	ctx.Abort()
}
