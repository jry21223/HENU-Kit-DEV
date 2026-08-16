package httpapi

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/contract"
)

// The OAuth callback is a browser navigation, never an API surface: when the
// flow fails (missing/invalid oauth cookie, missing/invalid/expired/replayed
// state), the browser must be redirected back into the login entry so a fresh
// flow can recover — never shown a raw JSON error page. The recovery target
// is the login entry with the safe default return path.
const wantLoginRecoveryLocation = "/api/v1/auth/login?return_to=%2F"

// validState is a 32-byte base64url state so tests reach the cookie checks.
const validState = "c2hhMTI4Ynl0ZXNvZnN0YXRlbGVuZ3RodGhpcnR5dHdv"

func TestOAuthCallbackMissingOAuthCookieRedirectsToLogin(t *testing.T) {
	handler, _ := newTestHandler(t, "http://127.0.0.1:9")

	// The #264 production symptom: callback arrives with code+state but the
	// oauth cookie is gone (browser deleted it after MaxAge while the user read
	// the mail code). The browser must recover through a fresh login flow.
	request := httptest.NewRequest(http.MethodGet,
		"https://portal.example/api/v1/auth/callback?code=test-code&state="+url.QueryEscape(validState), nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(recorder, request)

	assertLoginRecoveryRedirect(t, recorder)
}

func TestOAuthCallbackInvalidOAuthCookieRedirectsToLogin(t *testing.T) {
	handler, _ := newTestHandler(t, "http://127.0.0.1:9")

	request := httptest.NewRequest(http.MethodGet,
		"https://portal.example/api/v1/auth/callback?code=test-code&state="+url.QueryEscape(validState), nil)
	request.TLS = &tls.ConnectionState{}
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_oauth", Value: "not-base64url!!"})
	recorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(recorder, request)

	assertLoginRecoveryRedirect(t, recorder)
}

func TestOAuthCallbackExpiredFlowRedirectsToLogin(t *testing.T) {
	var tokenCalls atomic.Int32
	platformCore := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		tokenCalls.Add(1)
		t.Fatalf("expired OAuth flow must never reach the token endpoint, path=%q", request.URL.Path)
	}))
	t.Cleanup(platformCore.Close)

	handler, redisServer := newTestHandler(t, platformCore.URL)

	var logBuffer strings.Builder
	previousOutput := log.Writer()
	log.SetOutput(&logBuffer)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	state, oauthCookie := startOAuthFlow(t, handler, "https://portal.example/api/v1/auth/login")

	// The Redis state and the cookie share the same 30-minute window. Simulate
	// a slow email-code login that outlives it: the state expires server-side,
	// so the callback must redirect into a fresh flow even if the browser still
	// holds the cookie, and the log must classify it as expiry, not replay.
	redisServer.FastForward(31 * time.Minute)

	assertLoginRecoveryRedirect(t, completeOAuthCallback(t, handler, state, oauthCookie))
	if tokenCalls.Load() != 0 {
		t.Fatalf("token endpoint calls = %d, want 0", tokenCalls.Load())
	}
	if logged := logBuffer.String(); !strings.Contains(logged, "category=expired_state") {
		t.Fatalf("log must classify the failure as expired_state, got: %s", logged)
	}
}

func TestOAuthCallbackReplayRedirectsToLoginAndDoesNotReExchange(t *testing.T) {
	var tokenCalls atomic.Int32
	platformCore := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/oauth/token" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		tokenCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"user": map[string]string{
					"user_id":      "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
					"display_name": "小河同学",
				},
				"session_exchange_token": "exchange_token_with_at_least_32_characters",
				"expires_at":             time.Now().UTC().Add(8 * time.Hour),
			},
			"request_id": "req_platform_ok",
		})
	}))
	t.Cleanup(platformCore.Close)

	handler, _ := newTestHandler(t, platformCore.URL)

	var logBuffer strings.Builder
	previousOutput := log.Writer()
	log.SetOutput(&logBuffer)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	state, oauthCookie := startOAuthFlow(t, handler, "https://portal.example/api/v1/auth/login")
	first := completeOAuthCallback(t, handler, state, oauthCookie)
	if first.Code != http.StatusFound {
		t.Fatalf("first callback status = %d body=%s, want 302", first.Code, strings.TrimSpace(first.Body.String()))
	}

	// Replay the same state/cookie: the Redis state was GetDel'd on first use,
	// so the replay must redirect into a fresh login flow without exchanging,
	// and the log must classify it as a replay, not expiry.
	replay := completeOAuthCallback(t, handler, state, oauthCookie)
	assertLoginRecoveryRedirect(t, replay)
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint calls after replay = %d, want still 1", tokenCalls.Load())
	}
	if logged := logBuffer.String(); !strings.Contains(logged, "category=replayed_callback") {
		t.Fatalf("log must classify the failure as replayed_callback, got: %s", logged)
	}
}

func TestOAuthCallbackStateDependencyFailureReturnsServiceUnavailable(t *testing.T) {
	handler, redisServer := newTestHandler(t, "http://127.0.0.1:9")

	state, oauthCookie := startOAuthFlow(t, handler, "https://portal.example/api/v1/auth/login")

	// Redis going down is a dependency failure, not an expired or replayed
	// flow: the callback must fail closed with the honest service error and
	// never mislead ops by classifying an outage as flow expiry.
	redisServer.Close()

	recorder := completeOAuthCallback(t, handler, state, oauthCookie)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("callback status = %d body=%s, want 503", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error body: %v body=%s", err, recorder.Body.String())
	}
	if envelope.RequestID == "" {
		t.Fatal("dependency failure must include non-empty request_id")
	}
	if strings.Contains(recorder.Body.String(), "expired") || strings.Contains(recorder.Body.String(), "replay") {
		t.Fatalf("outage must not be reported as flow failure: %s", recorder.Body.String())
	}
}

func TestOAuthCallbackFailureLogsRedactedCategory(t *testing.T) {
	handler, _ := newTestHandler(t, "http://127.0.0.1:9")

	var logBuffer strings.Builder
	previousOutput := log.Writer()
	log.SetOutput(&logBuffer)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	request := httptest.NewRequest(http.MethodGet,
		"https://portal.example/api/v1/auth/callback?code=super-secret-code&state="+url.QueryEscape(validState), nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(recorder, request)

	assertLoginRecoveryRedirect(t, recorder)

	logged := logBuffer.String()
	if !strings.Contains(logged, "category=missing_oauth_cookie") {
		t.Fatalf("log must classify the failure, got: %s", logged)
	}
	if !strings.Contains(logged, "request_id=") {
		t.Fatalf("log must carry the request id, got: %s", logged)
	}
	if strings.Contains(logged, "super-secret-code") || strings.Contains(logged, validState) {
		t.Fatalf("log must not leak code or state, got: %s", logged)
	}
}

func assertLoginRecoveryRedirect(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusFound {
		t.Fatalf("callback status = %d body=%s, want 302 to login entry", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	if got := recorder.Header().Get("Location"); got != wantLoginRecoveryLocation {
		t.Fatalf("callback Location = %q, want %q", got, wantLoginRecoveryLocation)
	}
	if strings.Contains(recorder.Body.String(), "error") {
		t.Fatalf("callback must not render a JSON error body, got: %s", recorder.Body.String())
	}
}
