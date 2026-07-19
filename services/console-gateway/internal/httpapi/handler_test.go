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
	"regexp"
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
	operations         json.RawMessage
	operationResult    json.RawMessage
	operationToken     string
	operationKey       string
	libraryPermissions []string
}

type fakeOverview struct{}

type fakeNotice struct {
	actor, key       string
	snapshot, result json.RawMessage
}

type fakeLibrary struct {
	actor, key        string
	workspace, result json.RawMessage
}

func (f *fakeLibrary) Workspace(_ context.Context, actor string) (json.RawMessage, error) {
	f.actor = actor
	return f.workspace, nil
}
func (f *fakeLibrary) Command(_ context.Context, actor, key string, _ []byte) (json.RawMessage, error) {
	f.actor, f.key = actor, key
	return f.result, nil
}
func (f *fakeLibrary) Operation(_ context.Context, actor, _, key string) (json.RawMessage, error) {
	f.actor, f.key = actor, key
	return f.result, nil
}

func (f *fakeNotice) Snapshot(_ context.Context, actor string) (json.RawMessage, error) {
	f.actor = actor
	return f.snapshot, nil
}
func (f *fakeNotice) CreateSource(_ context.Context, actor, key string, _ []byte) (json.RawMessage, error) {
	f.actor, f.key = actor, key
	return f.result, nil
}
func (f *fakeNotice) CreateVersion(_ context.Context, actor, _, key string, _ []byte) (json.RawMessage, error) {
	f.actor, f.key = actor, key
	return f.result, nil
}
func (f *fakeNotice) Review(_ context.Context, actor, _, key string, _ []byte) (json.RawMessage, error) {
	f.actor, f.key = actor, key
	return f.result, nil
}
func (f *fakeNotice) Distribute(_ context.Context, actor, _, key string, _ []byte) (json.RawMessage, error) {
	f.actor, f.key = actor, key
	return f.result, nil
}
func (f *fakeNotice) Operation(_ context.Context, actor, _, key string) (json.RawMessage, error) {
	f.actor, f.key = actor, key
	return f.result, nil
}

func (fakeOverview) Fetch(_ context.Context, _ string) contract.ConsoleOverview {
	modules := make([]contract.ConsoleModuleSummary, 0, 6)
	for _, id := range []string{"portal", "platform", "notice", "library", "quizcraft", "food"} {
		modules = append(modules, contract.ConsoleModuleSummary{ID: id, Status: "unavailable", Metrics: []contract.ConsoleModuleMetric{}, StatusMessage: "摘要暂不可用", RequestID: "req_" + id})
	}
	return contract.ConsoleOverview{Modules: modules, GeneratedAt: time.Now()}
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

func (fake *fakePlatform) CheckPlatformOperations(_ context.Context, token string) error {
	if token != fake.exchange.ExchangeToken {
		return platformcore.ErrUnauthorized
	}
	return fake.checkErr
}

func (fake *fakePlatform) CheckPlatformOperationsWrite(_ context.Context, token string) error {
	if token != fake.exchange.ExchangeToken {
		return platformcore.ErrUnauthorized
	}
	return fake.checkErr
}

func (fake *fakePlatform) CheckNotice(_ context.Context, token, _ string) error {
	if token != fake.exchange.ExchangeToken {
		return platformcore.ErrUnauthorized
	}
	return fake.checkErr
}

func (fake *fakePlatform) CheckLibrary(_ context.Context, token, permission string) error {
	if token != fake.exchange.ExchangeToken {
		return platformcore.ErrUnauthorized
	}
	fake.libraryPermissions = append(fake.libraryPermissions, permission)
	return fake.checkErr
}

func (fake *fakePlatform) PlatformOperations(_ context.Context, token string) (json.RawMessage, error) {
	fake.operationToken = token
	return fake.operations, fake.checkErr
}

func (fake *fakePlatform) RevokeSession(_ context.Context, token, _, key string, _ []byte) (json.RawMessage, error) {
	fake.operationToken, fake.operationKey = token, key
	return fake.operationResult, fake.checkErr
}

func (fake *fakePlatform) UpdateAccess(_ context.Context, token, _, key string, _ []byte) (json.RawMessage, error) {
	fake.operationToken, fake.operationKey = token, key
	return fake.operationResult, fake.checkErr
}

func (fake *fakePlatform) OperationStatus(_ context.Context, token, _, key string) (json.RawMessage, error) {
	fake.operationToken, fake.operationKey = token, key
	return fake.operationResult, fake.checkErr
}

func TestPlatformOperationsUsesServerSessionAndForwardsIdempotency(t *testing.T) {
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	fake := &fakePlatform{
		exchange:        platformcore.Exchange{ExchangeToken: "exchange_token_with_at_least_32_characters"},
		operations:      json.RawMessage(`{"accounts":[],"sessions":[],"mail":{"pending":0,"processing":0,"retry_due":0,"accepted":0,"delivered":0,"failed":0,"dead_letters":0},"inbox_items":[],"audit":[],"dependencies":{"postgres":"ready","redis":"ready"},"generated_at":"2026-07-19T00:00:00Z"}`),
		operationResult: json.RawMessage(`{"operation":"session_revoke","status":"succeeded","resource_id":"171f1c6f-7b10-4c92-91a2-b39bf5af5302"}`),
	}
	handler, _ := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", fake, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	encoded, _ := codec.Encode(session.Value{UserID: "171f1c6f-7b10-4c92-91a2-b39bf5af5302", ExchangeToken: fake.exchange.ExchangeToken, ExpiresAt: time.Now().Add(time.Minute)})

	read, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/operations", nil)
	read.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	readResponse, err := server.Client().Do(read)
	if err != nil {
		t.Fatal(err)
	}
	readPayload, _ := io.ReadAll(readResponse.Body)
	readResponse.Body.Close()
	if readResponse.StatusCode != http.StatusOK || strings.Contains(string(readPayload), fake.exchange.ExchangeToken) || fake.operationToken != fake.exchange.ExchangeToken {
		t.Fatalf("operations read = %d %s token-forwarded=%t", readResponse.StatusCode, readPayload, fake.operationToken == fake.exchange.ExchangeToken)
	}

	revoke, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/operations/sessions/171f1c6f-7b10-4c92-91a2-b39bf5af5302/revocations", strings.NewReader(`{"expected_active":true}`))
	revoke.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	revoke.Header.Set("Idempotency-Key", "idem_console_operation")
	revokeResponse, err := server.Client().Do(revoke)
	if err != nil {
		t.Fatal(err)
	}
	revokeResponse.Body.Close()
	if revokeResponse.StatusCode != http.StatusOK || fake.operationKey != "idem_console_operation" {
		t.Fatalf("operations write = %d key=%q", revokeResponse.StatusCode, fake.operationKey)
	}
}

func TestNoticeForwardingUsesServerActorAndPreservesIdempotency(t *testing.T) {
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	userID := "171f1c6f-7b10-4c92-91a2-b39bf5af5302"
	token := "exchange_token_with_at_least_32_characters"
	platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: token}}
	owner := &fakeNotice{snapshot: json.RawMessage(`{"items":[],"generated_at":"2026-07-19T00:00:00Z"}`), result: json.RawMessage(`{"state":"approved","revision":2}`)}
	handler, _ := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, owner, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	encoded, _ := codec.Encode(session.Value{UserID: userID, ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})
	read, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/notices", nil)
	read.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	response, err := server.Client().Do(read)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || owner.actor != userID {
		t.Fatalf("Notice read status/actor=%d/%s", response.StatusCode, owner.actor)
	}
	review, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/notices/versions/471f1c6f-7b10-4c92-91a2-b39bf5af5302/reviews", strings.NewReader(`{"decision":"approved","expected_revision":1}`))
	review.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	review.Header.Set("Content-Type", "application/json")
	review.Header.Set("Idempotency-Key", "idem_notice_review_test")
	response, err = server.Client().Do(review)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || owner.key != "idem_notice_review_test" {
		t.Fatalf("Notice review status/key=%d/%s", response.StatusCode, owner.key)
	}
}

func TestLibraryForwardingUsesServerActorAndPreservesIdempotency(t *testing.T) {
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	userID := "171f1c6f-7b10-4c92-91a2-b39bf5af5302"
	token := "exchange_token_with_at_least_32_characters"
	platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: token}}
	owner := &fakeLibrary{workspace: json.RawMessage(`{"status":"partial","status_message":"one source unavailable","degraded":true,"courses":[],"materials":[],"downloads":[],"submissions":[],"corrections":[],"generated_at":"2026-07-19T00:00:00Z"}`), result: json.RawMessage(`{"operation":"submission_approve","resource_id":"22222222-2222-4222-8222-222222222222","state":"succeeded"}`)}
	handler, _ := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)), owner)
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	encoded, _ := codec.Encode(session.Value{UserID: userID, ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})

	read, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/library", nil)
	read.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	response, err := server.Client().Do(read)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || owner.actor != userID {
		t.Fatalf("Library read status/actor=%d/%s", response.StatusCode, owner.actor)
	}

	command, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/library/commands", strings.NewReader(`{"kind":"submission_approve","resource_id":"22222222-2222-4222-8222-222222222222","expected_version":"2026-07-19T00:00:00Z","payload":{"reviewReason":"checked"}}`))
	command.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	command.Header.Set("Content-Type", "application/json")
	command.Header.Set("Idempotency-Key", "idem_library_gateway")
	response, err = server.Client().Do(command)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || owner.key != "idem_library_gateway" {
		t.Fatalf("Library command status/key=%d/%s", response.StatusCode, owner.key)
	}

	operation, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/library/operations/submission_approve", nil)
	operation.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	operation.Header.Set("Idempotency-Key", "idem_library_gateway")
	response, err = server.Client().Do(operation)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || platform.libraryPermissions[len(platform.libraryPermissions)-1] != "library.review" {
		t.Fatalf("Library operation status/permission=%d/%v", response.StatusCode, platform.libraryPermissions)
	}
}

func TestRequestContextReplacesContractInvalidRequestID(t *testing.T) {
	handler := (&Handler{}).requestContext(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
		if requestID(request) != writer.Header().Get("X-Request-Id") {
			t.Error("request context and response header use different request IDs")
		}
	}))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-Id", "req_invalid value!")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	requestID := response.Header().Get("X-Request-Id")
	if requestID == "req_invalid value!" || len(requestID) > 100 || !regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`).MatchString(requestID) {
		t.Fatalf("invalid replacement request ID %q", requestID)
	}
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
	handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", fake, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
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
	if err := json.Unmarshal(payload, &envelope); err != nil || envelope.Data.User.ID != fake.exchange.UserID || len(envelope.Data.AccessContext.Permissions) != 6 || len(envelope.Data.AccessContext.Scopes) != 2 || envelope.Data.AccessContext.Scopes[0].Kind != "platform" || envelope.Data.AccessContext.Scopes[1].ProductCode == nil || *envelope.Data.AccessContext.Scopes[1].ProductCode != "notice" {
		t.Fatalf("invalid access context: %s (%v)", payload, err)
	}
	overviewRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/overview", nil)
	overviewRequest.AddCookie(sessionValue)
	overviewResponse, err := client.Do(overviewRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer overviewResponse.Body.Close()
	var overviewEnvelope struct {
		Data contract.ConsoleOverview `json:"data"`
	}
	if err := json.NewDecoder(overviewResponse.Body).Decode(&overviewEnvelope); err != nil || overviewResponse.StatusCode != http.StatusOK || len(overviewEnvelope.Data.Modules) != 6 {
		t.Fatalf("overview conformance = %d %+v (%v)", overviewResponse.StatusCode, overviewEnvelope.Data, err)
	}
}

func TestConsoleSessionDefaultsToDenyAndClearsRevokedSession(t *testing.T) {
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	fake := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: "exchange_token_with_at_least_32_characters"}, checkErr: platformcore.ErrForbidden}
	handler, _ := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", fake, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
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
	handler, _ := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", fake, nil, fakeOverview{}, redisClient, codec, slog.New(slog.NewJSONHandler(io.Discard, nil)))
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
