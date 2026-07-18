package serviceauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type memoryNonceStore struct{ claimed map[string]bool }

func (store *memoryNonceStore) Claim(_ context.Context, key string, _ time.Duration) (bool, error) {
	if store.claimed[key] {
		return false, nil
	}
	store.claimed[key] = true
	return true, nil
}

func sign(method, path, timestamp, nonce, body, secret string) string {
	bodyHash := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(bodyHash[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func request(verifier Verifier, timestamp, nonce, signature string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(verifier.Require())
	router.POST("/api/v1/internal/mail-deliveries", func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/mail-deliveries", strings.NewReader(`{"category":"critical"}`))
	req.Header.Set("X-Service-Id", "notice")
	req.Header.Set("X-Key-Id", "active")
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", signature)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func TestVerifierAcceptsValidSignatureAndRejectsNonceReplay(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	verifier := Verifier{store: &memoryNonceStore{claimed: map[string]bool{}}, keys: map[string]string{"notice:active": "secret"}, now: func() time.Time { return now }}
	timestamp := now.Format(time.RFC3339)
	nonce := "nonce-123456"
	signature := sign(http.MethodPost, "/api/v1/internal/mail-deliveries", timestamp, nonce, `{"category":"critical"}`, "secret")
	if got := request(verifier, timestamp, nonce, signature).Code; got != http.StatusNoContent {
		t.Fatalf("valid request status = %d", got)
	}
	if got := request(verifier, timestamp, nonce, signature).Code; got != http.StatusUnauthorized {
		t.Fatalf("replayed request status = %d", got)
	}
}

func TestVerifierRejectsStaleAndInvalidSignatures(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	verifier := Verifier{store: &memoryNonceStore{claimed: map[string]bool{}}, keys: map[string]string{"notice:active": "secret"}, now: func() time.Time { return now }}
	stale := now.Add(-6 * time.Minute).Format(time.RFC3339)
	if got := request(verifier, stale, "nonce-stale", sign(http.MethodPost, "/api/v1/internal/mail-deliveries", stale, "nonce-stale", `{"category":"critical"}`, "secret")).Code; got != http.StatusUnauthorized {
		t.Fatalf("stale request status = %d", got)
	}
	if got := request(verifier, now.Format(time.RFC3339), "nonce-invalid", "bad").Code; got != http.StatusUnauthorized {
		t.Fatalf("invalid signature status = %d", got)
	}
}
