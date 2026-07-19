package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/contract"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

type fakePlatform struct {
	mu                 sync.Mutex
	exchangeCalls      int
	checkCalls         int
	checkErr           error
	verifier, redirect string
	idempotencyKey     string
	exchange           platformcore.Exchange
}

func (fake *fakePlatform) ExchangeCode(_ context.Context, _, redirect, verifier, idempotencyKey string) (platformcore.Exchange, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.exchangeCalls++
	fake.redirect, fake.verifier, fake.idempotencyKey = redirect, verifier, idempotencyKey
	return fake.exchange, nil
}

func (fake *fakePlatform) CheckOverview(_ context.Context, token string) error {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.checkCalls++
	if token != fake.exchange.ExchangeToken {
		return platformcore.ErrUnauthorized
	}
	return fake.checkErr
}

func TestConsoleAuthorizationCodeFlowAndAccessContextConformsToContract(t *testing.T) {
	redisClient := testRedis(t)
	codec, err := session.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakePlatform{exchange: platformcore.Exchange{
		UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: "exchange_token_with_at_least_32_characters", ExpiresAt: time.Now().Add(5 * time.Minute),
	}}
	handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", fake, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	login, err := client.Get(server.URL + "/api/v1/auth/login?return_to=%2Foperations%3Ftab%3Dinbox")
	if err != nil {
		t.Fatal(err)
	}
	defer login.Body.Close()
	if login.StatusCode != http.StatusFound {
		t.Fatalf("login = %d, want 302", login.StatusCode)
	}
	if login.Header.Get("Cache-Control") != "no-store" || login.Header.Get("Referrer-Policy") != "no-referrer" || login.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing security headers: %v", login.Header)
	}
	var flowCookie *http.Cookie
	for _, cookie := range login.Cookies() {
		if cookie.Name == oauthFlowCookie {
			flowCookie = cookie
		}
	}
	if flowCookie == nil || !flowCookie.HttpOnly || !flowCookie.Secure || flowCookie.SameSite != http.SameSiteLaxMode || flowCookie.Path != "/" || flowCookie.MaxAge != int(stateTTL.Seconds()) {
		t.Fatalf("invalid browser-bound OAuth cookie: %+v", login.Cookies())
	}
	authorize, err := url.Parse(login.Header.Get("Location"))
	if err != nil || authorize.Host != "account.henukit.test" || authorize.Query().Get("code_challenge_method") != "S256" || len(authorize.Query().Get("code_challenge")) != 43 {
		t.Fatalf("invalid authorize redirect: %s (%v)", login.Header.Get("Location"), err)
	}
	state := authorize.Query().Get("state")
	attackerCallbackClient := &http.Client{Transport: client.Transport, CheckRedirect: client.CheckRedirect}
	unbound, err := attackerCallbackClient.Get(server.URL + "/api/v1/auth/callback?code=authorization_code_123456&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	unbound.Body.Close()
	if unbound.StatusCode != http.StatusBadRequest || fake.exchangeCalls != 0 {
		t.Fatalf("unbound callback = %d with %d exchanges", unbound.StatusCode, fake.exchangeCalls)
	}
	wrongState, err := client.Get(server.URL + "/api/v1/auth/callback?code=authorization_code_123456&state=wrong_browser_bound_state_123456789012")
	if err != nil {
		t.Fatal(err)
	}
	wrongState.Body.Close()
	if wrongState.StatusCode != http.StatusBadRequest || fake.exchangeCalls != 0 {
		t.Fatalf("wrong bound state = %d with %d exchanges", wrongState.StatusCode, fake.exchangeCalls)
	}
	callback, err := client.Get(server.URL + "/api/v1/auth/callback?code=authorization_code_123456&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Body.Close()
	if callback.StatusCode != http.StatusFound || callback.Header.Get("Location") != "/operations?tab=inbox" {
		t.Fatalf("callback = %d %q", callback.StatusCode, callback.Header.Get("Location"))
	}
	var sessionValue *http.Cookie
	for _, cookie := range callback.Cookies() {
		if cookie.Name == sessionCookie {
			sessionValue = cookie
		}
	}
	if sessionValue == nil || !sessionValue.HttpOnly || !sessionValue.Secure || sessionValue.SameSite != http.SameSiteLaxMode || sessionValue.Path != "/" {
		t.Fatalf("invalid Console Session cookie: %+v", callback.Cookies())
	}
	if fake.exchangeCalls != 1 || len(fake.verifier) != 43 || !strings.HasPrefix(fake.idempotencyKey, "idem_console_") {
		t.Fatalf("unexpected exchange call: %+v", fake)
	}

	replay, _ := client.Get(server.URL + "/api/v1/auth/callback?code=authorization_code_123456&state=" + url.QueryEscape(state))
	replay.Body.Close()
	if replay.StatusCode != http.StatusBadRequest || fake.exchangeCalls != 1 {
		t.Fatalf("replayed callback = %d with %d exchanges", replay.StatusCode, fake.exchangeCalls)
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
	request.AddCookie(sessionValue)
	sessionResponse, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer sessionResponse.Body.Close()
	payload, _ := io.ReadAll(sessionResponse.Body)
	if sessionResponse.StatusCode != http.StatusOK || strings.Contains(string(payload), fake.exchange.ExchangeToken) {
		t.Fatalf("session response = %d %s", sessionResponse.StatusCode, payload)
	}
	var envelope struct {
		Data contract.ConsoleSession `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Data.User.ID != fake.exchange.UserID || len(envelope.Data.AccessContext.Permissions) != 1 || envelope.Data.AccessContext.Scopes[0].Kind != "platform" {
		t.Fatalf("invalid access context: %s (%v)", payload, err)
	}
}

func TestConsoleSessionDefaultsToDenyAndClearsRevokedSession(t *testing.T) {
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	fake := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: "exchange_token_with_at_least_32_characters"}, checkErr: platformcore.ErrForbidden}
	handler, _ := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", fake, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	encoded, _ := codec.Encode(session.Value{UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: fake.exchange.ExchangeToken, ExpiresAt: time.Now().Add(time.Minute)})
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	response, _ := server.Client().Do(request)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("default deny = %d, want 403", response.StatusCode)
	}

	fake.checkErr = platformcore.ErrUnauthorized
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	response, _ = server.Client().Do(request)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || len(response.Cookies()) != 1 || response.Cookies()[0].MaxAge != -1 {
		t.Fatalf("revoked session = %d cookies=%+v", response.StatusCode, response.Cookies())
	}
}

func TestConsoleRejectsOpenRedirectAndExpiredCookieBeforePlatformCall(t *testing.T) {
	if !validReturnTo("/search?q=10:30") {
		t.Fatal("same-origin URI-reference with colon should be accepted")
	}
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	fake := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: "exchange_token_with_at_least_32_characters"}}
	handler, _ := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", fake, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	response, _ := server.Client().Get(server.URL + "/api/v1/auth/login?return_to=https%3A%2F%2Fevil.example")
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("open redirect = %d, want 400", response.StatusCode)
	}
	response, _ = server.Client().Get(server.URL + "/api/v1/auth/login?return_to=%2F%5C%5Cevil.example")
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("backslash open redirect = %d, want 400", response.StatusCode)
	}
	encoded, _ := codec.Encode(session.Value{UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: fake.exchange.ExchangeToken, ExpiresAt: time.Now().Add(-time.Second)})
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	response, _ = server.Client().Do(request)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || fake.checkCalls != 0 {
		t.Fatalf("expired cookie = %d with %d platform checks", response.StatusCode, fake.checkCalls)
	}
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	address := os.Getenv("CONSOLE_GATEWAY_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("CONSOLE_GATEWAY_TEST_REDIS_ADDR is required")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("flush Redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
