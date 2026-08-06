package httpapi

import (
	"crypto/tls"
	"encoding/json"
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

// The OAuth flow window (Redis state TTL and oauth cookie MaxAge) must stay in
// lockstep: both are oauthFlowTTL inside login(). This test pins the browser
// cookie attributes so a regression that widens or shrinks one side without
// the other fails here. 1800 = 30 minutes (#264: the email-code login flow
// outlived the old 5-minute window and the callback failed with missing oauth
// cookie on henukit.cn).
const wantOAuthCookieMaxAge = 1800

func TestLoginOAuthCookieAttributesMatchFlowWindow(t *testing.T) {
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

	httpsRequest := httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/auth/login", nil)
	httpsRequest.TLS = &tls.ConnectionState{}
	httpsRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(httpsRecorder, httpsRequest)

	cookies := httpsRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login set %d cookies, want 1", len(cookies))
	}
	oauth := cookies[0]
	if oauth.Name != "__Host-henukit_portal_oauth" {
		t.Fatalf("HTTPS oauth cookie name = %q, want __Host-henukit_portal_oauth", oauth.Name)
	}
	assertOAuthCookieAttributes(t, oauth, true)

	// The local HTTP profile must keep its own non-__Host- name and drop
	// Secure, but the flow window (MaxAge) must stay identical.
	localRequest := httptest.NewRequest(http.MethodGet, "http://portal.example/api/v1/auth/login", nil)
	localRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(localRecorder, localRequest)
	localCookies := localRecorder.Result().Cookies()
	if len(localCookies) != 1 || localCookies[0].Name != "henukit_portal_oauth_local" {
		t.Fatalf("local oauth cookie = %+v, want henukit_portal_oauth_local", localCookies)
	}
	assertOAuthCookieAttributes(t, localCookies[0], false)
}

func assertOAuthCookieAttributes(t *testing.T, cookie *http.Cookie, secure bool) {
	t.Helper()
	if cookie.Path != "/" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" {
		t.Fatalf("oauth cookie attributes = %+v, want Path=/ HttpOnly SameSite=Lax host-only", cookie)
	}
	if cookie.Secure != secure {
		t.Fatalf("oauth cookie Secure = %v, want %v", cookie.Secure, secure)
	}
	if cookie.MaxAge != wantOAuthCookieMaxAge {
		t.Fatalf("oauth cookie MaxAge = %d, want %d (matches oauthFlowTTL so the Redis state and cookie expire together)", cookie.MaxAge, wantOAuthCookieMaxAge)
	}
}

func TestOAuthCallbackMissingOAuthCookieFailsCleanly(t *testing.T) {
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

	// The production symptom of #264: callback arrives with code+state but the
	// oauth cookie is gone (the browser deleted it after MaxAge expired while
	// the user was reading the mail code). It must fail as a clean 400 with the
	// honest error, echo the request id, and never reach the exchange endpoint.
	request := httptest.NewRequest(http.MethodGet,
		"https://portal.example/api/v1/auth/callback?code=test-code&state="+url.QueryEscape("c2hhMTI4Ynl0ZXNvZnN0YXRlbGVuZ3RodGhpcnR5dHdv"),
		nil)
	request.TLS = &tls.ConnectionState{}
	request.Header.Set("X-Request-Id", "req_oauth_cookie_missing")
	recorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d body=%s, want 400", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error body: %v body=%s", err, recorder.Body.String())
	}
	if envelope.Error != "missing oauth cookie" {
		t.Fatalf("error = %q, want missing oauth cookie", envelope.Error)
	}
	if envelope.RequestID != "req_oauth_cookie_missing" {
		t.Fatalf("request_id = %q, want req_oauth_cookie_missing", envelope.RequestID)
	}
}

func TestOAuthFlowWindowExpiryFailsCleanly(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	var tokenCalls atomic.Int32
	platformCore := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		tokenCalls.Add(1)
		t.Fatalf("expired OAuth flow must never reach the token endpoint, path=%q", request.URL.Path)
	}))
	t.Cleanup(platformCore.Close)

	handler, err := New(config.Config{
		PlatformCoreURL:   platformCore.URL,
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

	loginRequest := httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/auth/login", nil)
	loginRequest.TLS = &tls.ConnectionState{}
	loginRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(loginRecorder, loginRequest)
	location, err := url.Parse(loginRecorder.Result().Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := location.Query().Get("state")
	var oauthCookie *http.Cookie
	for _, cookie := range loginRecorder.Result().Cookies() {
		if cookie.Name == "__Host-henukit_portal_oauth" {
			oauthCookie = cookie
			break
		}
	}
	if state == "" || oauthCookie == nil {
		t.Fatal("login redirect omitted state or oauth cookie")
	}

	// The Redis state and the cookie share the same 30-minute window. Simulate
	// a slow email-code login that outlives it: the state expires server-side,
	// so the callback must fail closed even if the browser still holds the
	// cookie.
	redisServer.FastForward(31 * time.Minute)

	callbackRequest := httptest.NewRequest(http.MethodGet,
		"https://portal.example/api/v1/auth/callback?code=test-code&state="+url.QueryEscape(state), nil)
	callbackRequest.TLS = &tls.ConnectionState{}
	callbackRequest.AddCookie(oauthCookie)
	callbackRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(callbackRecorder, callbackRequest)

	if callbackRecorder.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d body=%s, want clean 400", callbackRecorder.Code, strings.TrimSpace(callbackRecorder.Body.String()))
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(callbackRecorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if envelope.Error != "invalid or expired state" {
		t.Fatalf("error = %q, want invalid or expired state", envelope.Error)
	}
	if tokenCalls.Load() != 0 {
		t.Fatalf("token endpoint calls = %d, want 0", tokenCalls.Load())
	}
}

func TestOAuthCallbackRedirectUsesSafeStoredReturnTo(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	platformCore := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/oauth/token" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
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
		PlatformCoreURL:   platformCore.URL,
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

	completeFlow := func(returnTo string) string {
		t.Helper()
		loginURL := "https://portal.example/api/v1/auth/login"
		if returnTo != "" {
			loginURL += "?return_to=" + url.QueryEscape(returnTo)
		}
		loginRequest := httptest.NewRequest(http.MethodGet, loginURL, nil)
		loginRequest.TLS = &tls.ConnectionState{}
		loginRecorder := httptest.NewRecorder()
		handler.Router().ServeHTTP(loginRecorder, loginRequest)
		location, err := url.Parse(loginRecorder.Result().Header.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		state := location.Query().Get("state")
		var oauthCookie *http.Cookie
		for _, cookie := range loginRecorder.Result().Cookies() {
			if cookie.Name == "__Host-henukit_portal_oauth" {
				oauthCookie = cookie
				break
			}
		}
		callbackRequest := httptest.NewRequest(http.MethodGet,
			"https://portal.example/api/v1/auth/callback?code=test-code&state="+url.QueryEscape(state), nil)
		callbackRequest.TLS = &tls.ConnectionState{}
		callbackRequest.AddCookie(oauthCookie)
		callbackRecorder := httptest.NewRecorder()
		handler.Router().ServeHTTP(callbackRecorder, callbackRequest)
		if callbackRecorder.Code != http.StatusFound {
			t.Fatalf("callback status = %d body=%s, want 302", callbackRecorder.Code, strings.TrimSpace(callbackRecorder.Body.String()))
		}
		return callbackRecorder.Header().Get("Location")
	}

	if got := completeFlow(""); got != "https://portal.example/" {
		t.Fatalf("default return_to Location = %q, want origin root", got)
	}
	if got := completeFlow("/account"); got != "https://portal.example/account" {
		t.Fatalf("relative return_to Location = %q, want https://portal.example/account", got)
	}
	// Absolute URLs are not trusted: the callback must never redirect off the
	// portal origin, so login() falls back to "/" and the flow lands on the root.
	if got := completeFlow("https://evil.example/phish"); got != "https://portal.example/" {
		t.Fatalf("absolute return_to Location = %q, want https://portal.example/ (open-redirect protection)", got)
	}
}

func TestOAuthCallbackExpiredSessionCookieValueFailsCleanly(t *testing.T) {
	// A cookie whose MaxAge has passed behaves exactly like a missing cookie at
	// the callback: r.Cookie returns nothing useful and the flow must fail
	// closed with the production error rather than panicking.
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

	request := httptest.NewRequest(http.MethodGet,
		"https://portal.example/api/v1/auth/callback?code=test-code&state=not-a-real-state", nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d body=%s, want 400", recorder.Code, strings.TrimSpace(recorder.Body.String()))
	}
	if !strings.Contains(recorder.Body.String(), "missing oauth cookie") {
		t.Fatalf("error body = %s, want missing oauth cookie", recorder.Body.String())
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.RequestID == "" {
		t.Fatal("error must include a request_id")
	}
}
