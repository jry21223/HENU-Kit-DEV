package accountportfolio

import (
	"bytes"
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
)

const nonceTTL = 5 * time.Minute

func (h *service) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID, basicSecret, basic := r.BasicAuth()
		secret, keyKnown := h.keys[r.Header.Get("X-Key-Id")]
		if !basic || clientID != h.clientID || r.Header.Get("X-Service-Id") != clientID || !keyKnown || !hmac.Equal([]byte(secret), []byte(basicSecret)) {
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

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 512<<10))
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is too large")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		digest := sha256.Sum256(body)
		canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write([]byte(canonical))
		if !hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			writeError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service signature is invalid")
			return
		}

		if _, err := h.database.Exec(r.Context(), `DELETE FROM account_portfolio_service_nonces WHERE expires_at <= $1`, h.now().UTC()); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "nonce store is unavailable")
			return
		}
		result, err := h.database.Exec(r.Context(), `
			INSERT INTO account_portfolio_service_nonces(client_id, nonce, expires_at)
			VALUES($1, $2, $3)
			ON CONFLICT (client_id, nonce) DO NOTHING
		`, clientID, nonce, h.now().UTC().Add(nonceTTL))
		if err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "nonce store is unavailable")
			return
		}
		if result.RowsAffected() != 1 {
			writeError(w, r, http.StatusConflict, "REPLAY_DETECTED", "service nonce was already used")
			return
		}

		userID := r.Header.Get("X-Actor-User-Id")
		if uuid.Validate(userID) != nil {
			writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor context is invalid")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor{userID: userID})))
	})
}

func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}
