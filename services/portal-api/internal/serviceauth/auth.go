package serviceauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MaxSignedBodyBytes = 512 << 10
	TimestampWindowSec = 300
)

type Actor struct {
	UserID      string
	DisplayName string
	Permission  string
	ProductCode string
	ScopeKind   string
}

type contextKey string

const actorContextKey contextKey = "campus-service-actor"

func ActorFromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorContextKey).(Actor)
	return a, ok
}

type Verifier struct {
	ClientID   string
	KeyID      string
	Secret     string
	Now        func() time.Time
	mu         sync.Mutex
	seenNonces map[string]time.Time
}

func NewVerifier(clientID, keyID, secret string) *Verifier {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(keyID) == "" || len(secret) < 16 {
		return nil
	}
	return &Verifier{
		ClientID:   strings.TrimSpace(clientID),
		KeyID:      strings.TrimSpace(keyID),
		Secret:     secret,
		Now:        time.Now,
		seenNonces: make(map[string]time.Time),
	}
}

func (v *Verifier) recordNonce(nonce string, now time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.seenNonces) > 2048 {
		for k, exp := range v.seenNonces {
			if now.After(exp) {
				delete(v.seenNonces, k)
			}
		}
	}
	if exp, exists := v.seenNonces[nonce]; exists && now.Before(exp) {
		return false
	}
	v.seenNonces[nonce] = now.Add(10 * time.Minute)
	return true
}

func (v *Verifier) Middleware(requireActor bool, requireConsoleScope bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if v == nil {
				writeAuthError(w, r, http.StatusServiceUnavailable, "SERVICE_AUTH_UNCONFIGURED", "campus service credentials are not configured")
				return
			}
			clientID, basicSecret, basic := r.BasicAuth()
			if !basic || clientID != v.ClientID || r.Header.Get("X-Service-Id") != v.ClientID ||
				r.Header.Get("X-Key-Id") != v.KeyID || !hmac.Equal([]byte(v.Secret), []byte(basicSecret)) {
				writeAuthError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service credentials are invalid")
				return
			}
			now := v.Now()
			ts, err := strconv.ParseInt(r.Header.Get("X-Timestamp"), 10, 64)
			if err != nil || ts < now.Unix()-TimestampWindowSec || ts > now.Unix()+TimestampWindowSec {
				writeAuthError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service timestamp is invalid")
				return
			}
			nonce := r.Header.Get("X-Nonce")
			decoded, err := base64.RawURLEncoding.DecodeString(nonce)
			if err != nil || len(decoded) != 24 {
				writeAuthError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service nonce is invalid")
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxSignedBodyBytes))
			if err != nil {
				writeAuthError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "request body is too large")
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			digest := sha256.Sum256(body)
			canonical := strings.Join([]string{r.Method, r.URL.RequestURI(), r.Header.Get("X-Timestamp"), nonce, hex.EncodeToString(digest[:])}, "\n")
			mac := hmac.New(sha256.New, []byte(v.Secret))
			_, _ = mac.Write([]byte(canonical))
			expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
			if !hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(expected)) {
				writeAuthError(w, r, http.StatusUnauthorized, "INVALID_SERVICE_AUTH", "service signature is invalid")
				return
			}
			if !v.recordNonce(nonce, now) {
				writeAuthError(w, r, http.StatusConflict, "REPLAY_DETECTED", "service nonce was already used")
				return
			}
			actor := Actor{
				UserID:      strings.TrimSpace(r.Header.Get("X-Actor-User-Id")),
				DisplayName: strings.TrimSpace(r.Header.Get("X-Actor-Display-Name")),
				Permission:  strings.TrimSpace(r.Header.Get("X-Permission-Code")),
				ProductCode: strings.TrimSpace(r.Header.Get("X-Product-Code")),
				ScopeKind:   strings.TrimSpace(r.Header.Get("X-Scope-Kind")),
			}
			if requireActor && !looksLikeUUID(actor.UserID) {
				writeAuthError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "actor user id is required")
				return
			}
			if requireConsoleScope && (actor.ScopeKind != "product" || actor.ProductCode != "campus") {
				writeAuthError(w, r, http.StatusForbidden, "SCOPE_DENIED", "campus product scope is required")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorContextKey, actor)))
		})
	}
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func writeAuthError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      map[string]string{"code": code, "message": message},
		"request_id": r.Header.Get("X-Request-Id"),
	})
}
