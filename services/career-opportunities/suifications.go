package career

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

const (
	defaultSuifyRateLimit = 5
	suificationReplayTTL  = 10 * time.Minute
	// The lock outlives the 60s provider, 65s Gateway, and 70s server budgets,
	// leaving room for replay persistence before another caller can acquire it.
	suificationLockTTL = 2 * time.Minute
)

var suificationIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,200}$`)

type suificationInput struct {
	ResumeText string `json:"resume_text"`
}

type suificationRecord struct {
	RequestHash string `json:"request_hash"`
	Draft       string `json:"draft"`
}

type suificationLock struct {
	RequestHash string `json:"request_hash"`
	Token       string `json:"token"`
}

func (h *service) createSuification(w http.ResponseWriter, r *http.Request) {
	value, ok := h.requireActor(w, r)
	if !ok {
		return
	}
	var input suificationInput
	if _, ok := decode(w, r, &input); !ok {
		return
	}
	if strings.TrimSpace(input.ResumeText) == "" || utf8.RuneCountInString(input.ResumeText) > profileResumeTextLimit {
		writeError(w, r, http.StatusBadRequest, "INVALID_RESUME_TEXT", "resume text is invalid")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if !suificationIdempotencyKeyPattern.MatchString(idempotencyKey) {
		writeError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required")
		return
	}
	requestDigest := sha256.Sum256([]byte(input.ResumeText))
	requestHash := hex.EncodeToString(requestDigest[:])
	replayKey, lockKey := suificationRedisKeys(value.userID, idempotencyKey)
	if record, found, err := h.loadSuificationRecord(r, replayKey); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career suification is unavailable")
		return
	} else if found {
		h.writeSuificationReplay(w, r, record, requestHash)
		return
	}
	if h.suify == nil {
		writeError(w, r, http.StatusServiceUnavailable, "AI_UNCONFIGURED", "resume suification is not configured")
		return
	}
	lockValue, err := json.Marshal(suificationLock{RequestHash: requestHash, Token: requestID(r)})
	if err != nil {
		log.Printf("career: encode suification lock failed: %v", err)
		writeError(w, r, http.StatusServiceUnavailable, "SUIFY_FAILED", "resume suification failed")
		return
	}
	locked, err := h.redis.SetNX(r.Context(), lockKey, lockValue, suificationLockTTL).Result()
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career suification is unavailable")
		return
	}
	if !locked {
		h.writeSuificationLocked(w, r, lockKey, requestHash)
		return
	}
	defer h.releaseSuificationLock(r, lockKey, string(lockValue))

	// Close the race between the first replay read and lock acquisition.
	if record, found, err := h.loadSuificationRecord(r, replayKey); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career suification is unavailable")
		return
	} else if found {
		h.writeSuificationReplay(w, r, record, requestHash)
		return
	}
	allowed, err := h.allowSuification(r, value.userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career suification is unavailable")
		return
	}
	if !allowed {
		writeError(w, r, http.StatusTooManyRequests, "SUIFY_RATE_LIMITED", "resume suification rate limit exceeded")
		return
	}
	draft, err := h.suify(r.Context(), input.ResumeText)
	if err != nil {
		log.Printf("career: resume suification failed: %v", err)
		if errors.Is(err, ErrAIUnconfigured) {
			writeError(w, r, http.StatusServiceUnavailable, "AI_UNCONFIGURED", "resume suification is not configured")
			return
		}
		writeError(w, r, http.StatusServiceUnavailable, "SUIFY_FAILED", "resume suification failed")
		return
	}
	draft = truncateSuificationText(draft, profileResumeTextLimit)
	if draft == "" {
		writeError(w, r, http.StatusServiceUnavailable, "SUIFY_FAILED", "resume suification failed")
		return
	}
	record := suificationRecord{RequestHash: requestHash, Draft: draft}
	encoded, err := json.Marshal(record)
	if err != nil {
		log.Printf("career: encode suification replay failed: %v", err)
		writeError(w, r, http.StatusServiceUnavailable, "SUIFY_FAILED", "resume suification failed")
		return
	}
	storeContext, cancelStore := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelStore()
	if err := h.redis.Set(storeContext, replayKey, encoded, suificationReplayTTL).Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career suification is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"draft": map[string]string{"resume_text": draft}})
}

func suificationRedisKeys(userID, idempotencyKey string) (string, string) {
	digest := sha256.Sum256([]byte(userID + "\n" + idempotencyKey))
	identity := hex.EncodeToString(digest[:])
	return "career:suify:result:" + identity, "career:suify:lock:" + identity
}

func (h *service) loadSuificationRecord(r *http.Request, key string) (suificationRecord, bool, error) {
	raw, err := h.redis.Get(r.Context(), key).Bytes()
	if errors.Is(err, redis.Nil) {
		return suificationRecord{}, false, nil
	}
	if err != nil {
		return suificationRecord{}, false, err
	}
	var record suificationRecord
	if err := json.Unmarshal(raw, &record); err != nil || record.RequestHash == "" || record.Draft == "" {
		return suificationRecord{}, false, errors.New("invalid suification replay record")
	}
	return record, true, nil
}

func (h *service) writeSuificationReplay(w http.ResponseWriter, r *http.Request, record suificationRecord, requestHash string) {
	if record.RequestHash != requestHash {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"draft": map[string]string{"resume_text": record.Draft}})
}

func (h *service) writeSuificationLocked(w http.ResponseWriter, r *http.Request, lockKey, requestHash string) {
	raw, err := h.redis.Get(r.Context(), lockKey).Bytes()
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career suification is unavailable")
		return
	}
	var lock suificationLock
	if err := json.Unmarshal(raw, &lock); err != nil || lock.RequestHash == "" {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "career suification is unavailable")
		return
	}
	if lock.RequestHash != requestHash {
		writeError(w, r, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was used for another request")
		return
	}
	w.Header().Set("Retry-After", "2")
	writeError(w, r, http.StatusConflict, "SUIFY_ALREADY_ACTIVE", "resume suification is already running")
}

const releaseSuificationLockScript = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`

func (h *service) releaseSuificationLock(_ *http.Request, lockKey, lockValue string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := h.redis.Eval(ctx, releaseSuificationLockScript, []string{lockKey}, lockValue).Err(); err != nil {
		log.Printf("career: release suification lock failed: %v", err)
	}
}

func (h *service) allowSuification(r *http.Request, userID string) (bool, error) {
	key := "career:suify:rl:" + userID + ":" + h.now().UTC().Format("2006010215")
	count, err := h.redis.Incr(r.Context(), key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		if err := h.redis.Expire(r.Context(), key, time.Hour).Err(); err != nil {
			return false, err
		}
	}
	return count <= int64(h.suifyRateLimit), nil
}
