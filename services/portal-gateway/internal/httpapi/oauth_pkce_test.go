package httpapi

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/contract"
)

func TestOAuthPKCEChallengeMatchesVerifierAndSetsSession(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	var tokenCalls atomic.Int32
	var capturedVerifier string
	var capturedChallenge string

	expiresAt := time.Now().UTC().Add(8 * time.Hour).Truncate(time.Second)
	platformCore := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/oauth/token" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		tokenCalls.Add(1)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read token body: %v", err)
		}
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("token body json: %v", err)
		}
		capturedVerifier = payload["code_verifier"]
		if capturedVerifier == "" || payload["client_id"] != "portal-gateway" || payload["code"] == "" {
			t.Fatalf("incomplete token payload: %#v", payload)
		}
		// RFC 7636 / Platform Core: challenge must equal BASE64URL(SHA256(ASCII(verifier))).
		sum := sha256.Sum256([]byte(capturedVerifier))
		actual := base64.RawURLEncoding.EncodeToString(sum[:])
		if actual != capturedChallenge {
			t.Fatalf("PKCE mismatch: challenge=%q actual_from_verifier=%q", capturedChallenge, actual)
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"user": map[string]string{
					"user_id":      "171f1c6f-7b10-4c92-91a2-b39bf5af5302",
					"display_name": "小河同学",
				},
				"session_exchange_token": "exchange_token_with_at_least_32_characters",
				"expires_at":             expiresAt,
			},
			"request_id": "req_platform_ok",
		})
	}))
	t.Cleanup(platformCore.Close)

	handler, err := New(config.Config{
		PlatformCoreURL:       platformCore.URL,
		PlatformCorePublicURL: "https://accounts.example",
		PlatformClientID:      "portal-gateway",
		PlatformSecret:        "portal-client-secret-with-enough-entropy",
		PlatformKeyID:         "active-key",
		PortalRedirectURI:     "https://portal.example/api/v1/auth/callback",
		PortalOrigin:          "https://portal.example",
		SessionKey:            []byte("0123456789abcdef0123456789abcdef"),
	}, redisClient)
	if err != nil {
		t.Fatal(err)
	}

	loginRequest := httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/auth/login?return_to=/account", nil)
	loginRequest.TLS = &tls.ConnectionState{}
	loginRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(loginRecorder, loginRequest)

	loginResponse := loginRecorder.Result()
	if loginResponse.StatusCode != http.StatusFound {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}
	location, err := url.Parse(loginResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q", location.Query().Get("code_challenge_method"))
	}
	capturedChallenge = location.Query().Get("code_challenge")
	state := location.Query().Get("state")
	if capturedChallenge == "" || state == "" {
		t.Fatal("login redirect omitted PKCE challenge or state")
	}
	var oauthCookie *http.Cookie
	for _, cookie := range loginResponse.Cookies() {
		if cookie.Name == "__Host-henukit_portal_oauth" {
			oauthCookie = cookie
			break
		}
	}
	if oauthCookie == nil {
		t.Fatal("login response omitted OAuth cookie")
	}

	callbackURL := "https://portal.example/api/v1/auth/callback?code=test-authorization-code&state=" + url.QueryEscape(state)
	callbackRequest := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	callbackRequest.TLS = &tls.ConnectionState{}
	callbackRequest.AddCookie(oauthCookie)
	callbackRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(callbackRecorder, callbackRequest)

	if callbackRecorder.Code != http.StatusFound {
		t.Fatalf("callback status = %d body=%s", callbackRecorder.Code, strings.TrimSpace(callbackRecorder.Body.String()))
	}
	if got := callbackRecorder.Header().Get("Location"); got != "https://portal.example/account" {
		t.Fatalf("callback Location = %q", got)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range callbackRecorder.Result().Cookies() {
		if cookie.Name == "__Host-henukit_portal_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" || !sessionCookie.Secure || !sessionCookie.HttpOnly {
		t.Fatalf("missing Secure HttpOnly portal session cookie: %+v", sessionCookie)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", tokenCalls.Load())
	}

	sessionRequest := httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/session", nil)
	sessionRequest.TLS = &tls.ConnectionState{}
	sessionRequest.AddCookie(sessionCookie)
	sessionRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(sessionRecorder, sessionRequest)
	if sessionRecorder.Code != http.StatusOK {
		t.Fatalf("session status = %d body=%s", sessionRecorder.Code, sessionRecorder.Body.String())
	}
	var portalSession contract.PortalSession
	if err := json.Unmarshal(sessionRecorder.Body.Bytes(), &portalSession); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if portalSession.DisplayName != "小河同学" {
		t.Fatalf("session display_name = %q, want exchange display name", portalSession.DisplayName)
	}

	// Replay the same state/cookie: Redis state was GetDel'd — must fail closed without a second token call.
	replay := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	replay.TLS = &tls.ConnectionState{}
	replay.AddCookie(oauthCookie)
	replayRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(replayRecorder, replay)
	if replayRecorder.Code == http.StatusFound {
		t.Fatal("replayed OAuth state must not succeed")
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(replayRecorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("replay body: %v", err)
	}
	if envelope.RequestID == "" {
		t.Fatal("replay error must include non-empty request_id")
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint calls after replay = %d, want still 1", tokenCalls.Load())
	}
}

func TestOAuthCallbackErrorsIncludeRequestID(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	handler, err := New(config.Config{
		PlatformCoreURL:   "http://127.0.0.1:9",
		PlatformClientID:  "portal-gateway",
		PlatformSecret:    "portal-client-secret-with-enough-entropy",
		PlatformKeyID:     "active-key",
		PortalRedirectURI: "https://portal.example/api/v1/auth/callback",
		PortalOrigin:      "https://portal.example",
		SessionKey:        []byte("0123456789abcdef0123456789abcdef"),
	}, redisClient)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/auth/callback", nil)
	req.Header.Set("X-Request-Id", "req_callback_missing")
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	handler.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error != "missing code or state" {
		t.Fatalf("error = %q", envelope.Error)
	}
	if envelope.RequestID != "req_callback_missing" {
		t.Fatalf("request_id = %q, want req_callback_missing", envelope.RequestID)
	}
	if strings.Contains(rec.Body.String(), "code_verifier") || strings.Contains(rec.Body.String(), "authorization") {
		t.Fatalf("error body must not leak OAuth secrets: %s", rec.Body.String())
	}
}

func TestS256ChallengeHashesVerifierStringNotRawBytes(t *testing.T) {
	raw := []byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32,
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	// Correct (RFC): hash ASCII of encoded verifier.
	wantSum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(wantSum[:])
	if got := s256Challenge(verifier); got != want {
		t.Fatalf("s256Challenge = %q want %q", got, want)
	}
	// Incorrect legacy behavior hashed raw bytes — must differ for this input.
	legacySum := sha256.Sum256(raw)
	legacy := base64.RawURLEncoding.EncodeToString(legacySum[:])
	if want == legacy {
		t.Fatal("test vector collapsed; choose raw bytes that differ when hashed vs string-hashed")
	}
}
