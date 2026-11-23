package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func signCampusReq(req *http.Request, clientID, keyID, secret string, body string) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef01234567"))
	sum := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{req.Method, req.URL.RequestURI(), ts, nonce, hex.EncodeToString(sum[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	req.SetBasicAuth(clientID, secret)
	req.Header.Set("X-Service-Id", clientID)
	req.Header.Set("X-Key-Id", keyID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Nonce", nonce)
	req.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

func TestCampusWorkflowSecretSeparationAndFailClosed(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("STUDY_DATABASE_URL", "")
	t.Setenv("PORTAL_DATABASE_URL", "")

	// Overlapping portal & console secrets must fail closed at router construction.
	t.Setenv("CAMPUS_PORTAL_CLIENT_ID", "portal-gw")
	t.Setenv("CAMPUS_PORTAL_KEY_ID", "pk1")
	t.Setenv("CAMPUS_PORTAL_SECRET", "same-campus-secret-at-least-32-bytes")
	t.Setenv("CAMPUS_CONSOLE_CLIENT_ID", "console-gw")
	t.Setenv("CAMPUS_CONSOLE_KEY_ID", "ck1")
	t.Setenv("CAMPUS_CONSOLE_SECRET", "same-campus-secret-at-least-32-bytes")
	if _, err := NewRouter(); err == nil {
		t.Fatal("expected identical portal/console campus secret to fail")
	}

	// Unconfigured verifiers must fail closed with 503.
	t.Setenv("CAMPUS_PORTAL_SECRET", "")
	t.Setenv("CAMPUS_CONSOLE_SECRET", "")
	h, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/campus/contact-book", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured contact-book status = %d, want 503", rec.Code)
	}

	// Configured verifiers enforce HMAC + actor + scope + permission.
	const pSecret = "campus-portal-secret-at-least-32-bytes"
	const cSecret = "campus-console-secret-at-least-32-bytes"
	t.Setenv("CAMPUS_PORTAL_SECRET", pSecret)
	t.Setenv("CAMPUS_CONSOLE_SECRET", cSecret)
	h, err = NewRouter()
	if err != nil {
		t.Fatalf("NewRouter configured: %v", err)
	}

	// Raw unsigned actor header must be rejected with 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/campus/contact-book", nil)
	req.Header.Set("X-Actor-User-Id", "22222222-2222-4222-8222-222222222222")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned contact-book status = %d, want 401", rec.Code)
	}

	// Signed console summary works without actor header.
	sumReq := httptest.NewRequest(http.MethodGet, "/api/v1/campus/console-summary", nil)
	signCampusReq(sumReq, "console-gw", "ck1", cSecret, "")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, sumReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed console-summary status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Signed console workspace with wrong permission -> 403.
	wsReq := httptest.NewRequest(http.MethodGet, "/api/v1/campus/console/workspace", nil)
	wsReq.Header.Set("X-Actor-User-Id", "11111111-1111-4111-8111-111111111111")
	wsReq.Header.Set("X-Scope-Kind", "product")
	wsReq.Header.Set("X-Product-Code", "campus")
	wsReq.Header.Set("X-Permission-Code", "food.read")
	signCampusReq(wsReq, "console-gw", "ck1", cSecret, "")
	// Use distinct nonce so replay check passes
	wsReq.Header.Set("X-Nonce", base64.RawURLEncoding.EncodeToString([]byte("fedcba9876543210fedcba98")))
	sum := sha256.Sum256(nil)
	canonical := strings.Join([]string{wsReq.Method, wsReq.URL.RequestURI(), wsReq.Header.Get("X-Timestamp"), wsReq.Header.Get("X-Nonce"), hex.EncodeToString(sum[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(cSecret))
	_, _ = mac.Write([]byte(canonical))
	wsReq.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, wsReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong permission workspace status = %d, want 403", rec.Code)
	}
}
