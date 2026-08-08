package notice

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"henukit.dev/notice/internal/contract"
)

func (h *service) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if len(id) > 120 || !requestIDPattern.MatchString(id) {
			id = "req_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func (h *service) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID, basicSecret, basic := r.BasicAuth()
		client, clientKnown := h.clients[clientID]
		secret, keyKnown := client.keys[r.Header.Get("X-Key-Id")]
		if !basic || !clientKnown || r.Header.Get("X-Service-Id") != clientID || !keyKnown || !hmac.Equal([]byte(secret), []byte(basicSecret)) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service credentials are invalid")
			return
		}
		timestamp, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
		if err != nil || abs(h.now().Unix()-timestamp) > int64(nonceTTL/time.Second) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service timestamp is invalid")
			return
		}
		nonce := r.Header.Get("X-Nonce")
		decoded, err := base64.RawURLEncoding.DecodeString(nonce)
		if err != nil || len(decoded) != 24 {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service nonce is invalid")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is too large")
			return
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if !hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service signature is invalid")
			return
		}
		accepted, err := h.redis.SetNX(r.Context(), "notice:nonce:"+clientID+":"+nonce, "1", nonceTTL).Result()
		if err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "nonce store is unavailable")
			return
		}
		if !accepted {
			writeError(w, r, http.StatusConflict, "REPLAY_DETECTED", "service nonce was already used")
			return
		}
		if client.snapshotOnly && r.URL.Path != contract.SnapshotRoute {
			writeError(w, r, http.StatusForbidden, "READ_ONLY_CLIENT", "service credential is limited to the Notice snapshot")
			return
		}
		if r.URL.Path == "/api/v1/console-summary" {
			next.ServeHTTP(w, r)
			return
		}
		userID := r.Header.Get("X-Actor-User-Id")
		if _, err := uuid.Parse(userID); err != nil {
			writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
			return
		}
		if r.Header.Get("X-Scope-Kind") != "product" || r.Header.Get("X-Product-Code") != "notice" {
			writeError(w, r, http.StatusForbidden, "SCOPE_DENIED", "Notice product Scope is required")
			return
		}
		permission := r.Header.Get("X-Permission-Code")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor{userID: userID, permission: permission})))
	})
}
