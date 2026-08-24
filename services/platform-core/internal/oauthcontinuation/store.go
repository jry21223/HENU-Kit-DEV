package oauthcontinuation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrUnavailable = errors.New("OAuth continuation is unavailable")
	ErrConsumed    = errors.New("OAuth continuation was already consumed")
	ErrDependency  = errors.New("OAuth continuation dependency is unavailable")
)

const TTL = 30 * time.Minute

type Continuation struct {
	ClientID      string    `json:"client_id"`
	ProductName   string    `json:"product_name"`
	RedirectURI   string    `json:"redirect_uri"`
	State         string    `json:"state"`
	CodeChallenge string    `json:"code_challenge"`
	BindingHash   string    `json:"binding_hash"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type CreateInput struct {
	ClientID      string
	ProductName   string
	RedirectURI   string
	State         string
	CodeChallenge string
	BrowserID     string
}

type Store struct {
	redis *redis.Client
	now   func() time.Time
}

func New(redisClient *redis.Client) *Store {
	return &Store{redis: redisClient, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Store) ConsumeCreationQuota(ctx context.Context, clientID, browserID, clientIP string) (bool, error) {
	if s == nil || s.redis == nil || clientID == "" || browserID == "" || clientIP == "" {
		return false, ErrUnavailable
	}
	keys := []string{
		creationLimitKey("browser-hour", clientID, browserID),
		creationLimitKey("ip-hour", clientID, clientIP),
		creationLimitKey("ip-day", clientID, clientIP),
	}
	const script = `
local allowed = 1
for i = 1, #KEYS do
  local count = redis.call("INCR", KEYS[i])
  if count == 1 then redis.call("PEXPIRE", KEYS[i], ARGV[(i - 1) * 2 + 1]) end
  if count > tonumber(ARGV[(i - 1) * 2 + 2]) then allowed = 0 end
end
return allowed`
	allowed, err := s.redis.Eval(ctx, script, keys,
		time.Hour.Milliseconds(), 10,
		time.Hour.Milliseconds(), 60,
		(24 * time.Hour).Milliseconds(), 200,
	).Int64()
	if err != nil {
		return false, ErrDependency
	}
	return allowed == 1, nil
}

func (s *Store) Create(ctx context.Context, input CreateInput) (string, error) {
	if s == nil || s.redis == nil || input.ClientID == "" || len(input.ClientID) > 120 ||
		input.ProductName == "" || len(input.ProductName) > 80 ||
		input.RedirectURI == "" || len(input.RedirectURI) > 2048 ||
		len(input.State) < 8 || len(input.State) > 200 || len(input.CodeChallenge) != 43 ||
		input.BrowserID == "" || len(input.BrowserID) > 128 {
		return "", ErrUnavailable
	}
	createdAt := s.now()
	record := Continuation{
		ClientID: input.ClientID, ProductName: input.ProductName,
		RedirectURI: input.RedirectURI, State: input.State, CodeChallenge: input.CodeChallenge,
		BindingHash: bindingDigest(input.BrowserID), CreatedAt: createdAt, ExpiresAt: createdAt.Add(TTL),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", ErrUnavailable
	}
	for range 3 {
		raw := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, raw); err != nil {
			return "", err
		}
		handle := base64.RawURLEncoding.EncodeToString(raw)
		stored, err := s.redis.SetNX(ctx, lookupKey(handle, input.BrowserID), payload, TTL).Result()
		if err != nil {
			return "", ErrDependency
		}
		if stored {
			return handle, nil
		}
	}
	return "", ErrUnavailable
}

func (s *Store) Peek(ctx context.Context, handle, browserID string) (Continuation, error) {
	if !validHandle(handle) || browserID == "" {
		return Continuation{}, ErrUnavailable
	}
	payload, err := s.redis.Get(ctx, lookupKey(handle, browserID)).Bytes()
	return s.decode(payload, err, browserID)
}

func (s *Store) Consume(ctx context.Context, handle, browserID string) (Continuation, error) {
	if !validHandle(handle) || browserID == "" {
		return Continuation{}, ErrUnavailable
	}
	const script = `
local payload = redis.call("GET", KEYS[1])
if payload then
  local ttl = redis.call("PTTL", KEYS[1])
  redis.call("DEL", KEYS[1])
  if ttl > 0 then redis.call("SET", KEYS[2], "1", "PX", ttl) end
  return {"ok", payload}
end
if redis.call("EXISTS", KEYS[2]) == 1 then return {"consumed", ""} end
return {"missing", ""}`
	result, err := s.redis.Eval(ctx, script, []string{
		lookupKey(handle, browserID), consumedKey(handle, browserID),
	}).Slice()
	if err != nil || len(result) != 2 {
		return Continuation{}, ErrDependency
	}
	status, statusOK := result[0].(string)
	payload, payloadOK := result[1].(string)
	if !statusOK || !payloadOK {
		return Continuation{}, ErrDependency
	}
	switch status {
	case "ok":
		return s.decode([]byte(payload), nil, browserID)
	case "consumed":
		return Continuation{}, ErrConsumed
	default:
		return Continuation{}, ErrUnavailable
	}
}

func (s *Store) decode(payload []byte, err error, browserID string) (Continuation, error) {
	if errors.Is(err, redis.Nil) {
		return Continuation{}, ErrUnavailable
	}
	if err != nil {
		return Continuation{}, ErrDependency
	}
	var record Continuation
	if json.Unmarshal(payload, &record) != nil || record.ClientID == "" || record.ProductName == "" || record.RedirectURI == "" || record.State == "" || record.CodeChallenge == "" || record.BindingHash != bindingDigest(browserID) || !s.now().Before(record.ExpiresAt) {
		return Continuation{}, ErrUnavailable
	}
	return record, nil
}

func lookupKey(handle, browserID string) string {
	handleHash := sha256.Sum256([]byte(handle))
	return "platform-core:oauth-continuation:" + hex.EncodeToString(handleHash[:]) + ":" + bindingDigest(browserID)
}

func consumedKey(handle, browserID string) string {
	handleHash := sha256.Sum256([]byte(handle))
	return "platform-core:oauth-continuation-consumed:" + hex.EncodeToString(handleHash[:]) + ":" + bindingDigest(browserID)
}

func bindingDigest(browserID string) string {
	digest := sha256.Sum256([]byte(browserID))
	return hex.EncodeToString(digest[:])
}

func creationLimitKey(scope, clientID, discriminator string) string {
	digest := sha256.Sum256([]byte(clientID + "\x00" + discriminator))
	return "platform-core:oauth-continuation-limit:" + scope + ":" + hex.EncodeToString(digest[:])
}

func validHandle(handle string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(handle)
	return err == nil && len(raw) == 32
}
