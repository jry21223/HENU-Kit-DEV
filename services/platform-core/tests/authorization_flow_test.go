package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
)

const (
	testClientID     = "console-gateway"
	testClientSecret = "test-client-secret-with-enough-entropy"
	testRedirectURI  = "https://console.henukit.test/auth/callback"
	testCoreToken    = "core_test_session_token_32_bytes_long"
)

func TestAuthorizationCodeIsSingleUseAndCreatesDurableSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)

	handler, err := platformcore.New(platformcore.Config{
		Database:           pool,
		Redis:              redisClient,
		CoreCookieName:     "__Host-henukit_core_session",
		AuthorizationTTL:   90 * time.Second,
		ExchangeSessionTTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	server.Client().CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	verifier := "test-pkce-verifier-that-is-at-least-forty-three-characters"
	code := issueAuthorizationCode(t, server, verifier)
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("simulate Redis data loss: %v", err)
	}

	first := exchangeCode(t, server, code, verifier)
	if first.StatusCode != http.StatusOK {
		defer first.Body.Close()
		t.Fatalf("first exchange status = %d, want 200", first.StatusCode)
	}
	if len(first.Cookies()) != 0 {
		t.Fatal("server-to-server token exchange must not set a browser cookie")
	}
	var payload struct {
		Data struct {
			SessionExchangeToken string `json:"session_exchange_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(first.Body).Decode(&payload); err != nil {
		t.Fatalf("decode exchange response: %v", err)
	}
	first.Body.Close()
	if len(payload.Data.SessionExchangeToken) < 32 {
		t.Fatal("exchange token is missing or too short")
	}
	if first.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("server-to-server exchange must not opt into browser cross-origin access")
	}
	var storedTokenHash []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM sessions WHERE kind = 'client_exchange'`).Scan(&storedTokenHash); err != nil {
		t.Fatalf("read stored exchange token hash: %v", err)
	}
	expectedTokenHash := sha256.Sum256([]byte(payload.Data.SessionExchangeToken))
	if !bytes.Equal(storedTokenHash, expectedTokenHash[:]) || bytes.Equal(storedTokenHash, []byte(payload.Data.SessionExchangeToken)) {
		t.Fatal("exchange Session must persist only the token hash")
	}

	replay := exchangeCode(t, server, code, verifier)
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusConflict {
		t.Fatalf("replay status = %d, want 409", replay.StatusCode)
	}

	var exchangeSessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind = 'client_exchange'`).Scan(&exchangeSessions); err != nil {
		t.Fatalf("count exchange sessions: %v", err)
	}
	if exchangeSessions != 1 {
		t.Fatalf("exchange sessions = %d, want 1", exchangeSessions)
	}
}

func TestUnsafeCallbackAndWrongPKCEDoNotConsumeAuthorizationCode(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	server.Client().CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	verifier := "test-pkce-verifier-that-is-at-least-forty-three-characters"
	challengeBytes := sha256.Sum256([]byte(verifier))
	unsafeURL := server.URL + "/api/v1/oauth/authorize?" + url.Values{
		"response_type": {"code"}, "client_id": {testClientID},
		"redirect_uri": {"https://evil.example/auth/callback"}, "state": {"state_test_0123456789"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challengeBytes[:])}, "code_challenge_method": {"S256"},
	}.Encode()
	unsafeRequest, _ := http.NewRequest(http.MethodGet, unsafeURL, nil)
	unsafeRequest.AddCookie(&http.Cookie{Name: "__Host-henukit_core_session", Value: testCoreToken})
	unsafeResponse, err := server.Client().Do(unsafeRequest)
	if err != nil {
		t.Fatalf("unsafe authorize request: %v", err)
	}
	unsafeResponse.Body.Close()
	if unsafeResponse.StatusCode != http.StatusBadRequest || unsafeResponse.Header.Get("Location") != "" {
		t.Fatalf("unsafe callback status/location = %d/%q, want 400 without redirect", unsafeResponse.StatusCode, unsafeResponse.Header.Get("Location"))
	}
	var codeCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM authorization_codes`).Scan(&codeCount); err != nil || codeCount != 0 {
		t.Fatalf("unsafe callback persisted %d authorization codes (query error %v)", codeCount, err)
	}

	code := issueAuthorizationCode(t, server, verifier)
	wrongPKCE := exchangeCode(t, server, code, "wrong-pkce-verifier-that-is-at-least-forty-three-characters")
	wrongPKCE.Body.Close()
	if wrongPKCE.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong PKCE status = %d, want 400", wrongPKCE.StatusCode)
	}
	var consumed bool
	if err := pool.QueryRow(ctx, `SELECT used_at IS NOT NULL FROM authorization_codes`).Scan(&consumed); err != nil {
		t.Fatalf("read authorization code state: %v", err)
	}
	if consumed {
		t.Fatal("wrong PKCE verifier consumed the authorization code")
	}
	valid := exchangeCode(t, server, code, verifier)
	valid.Body.Close()
	if valid.StatusCode != http.StatusOK {
		t.Fatalf("correct exchange after PKCE failure = %d, want 200", valid.StatusCode)
	}
}

func TestRevokedCoreSessionBlocksCodeExchange(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	server.Client().CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	verifier := "test-pkce-verifier-that-is-at-least-forty-three-characters"
	code := issueAuthorizationCode(t, server, verifier)
	if _, err := pool.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE kind = 'core'`); err != nil {
		t.Fatalf("revoke Core Session: %v", err)
	}
	response := exchangeCode(t, server, code, verifier)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("exchange after Core Session revocation = %d, want 401", response.StatusCode)
	}
	var consumed bool
	if err := pool.QueryRow(ctx, `SELECT used_at IS NOT NULL FROM authorization_codes`).Scan(&consumed); err != nil {
		t.Fatalf("read authorization code state: %v", err)
	}
	if consumed {
		t.Fatal("revoked Core Session consumed the authorization code")
	}
}

func TestConcurrentAuthorizationCodeExchangeHasOneWinner(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	server.Client().CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	verifier := "test-pkce-verifier-that-is-at-least-forty-three-characters"
	code := issueAuthorizationCode(t, server, verifier)
	var successes atomic.Int32
	unexpected := make(chan string, 20)
	var wait sync.WaitGroup
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := exchangeCode(t, server, code, verifier)
			defer response.Body.Close()
			switch response.StatusCode {
			case http.StatusOK:
				successes.Add(1)
			case http.StatusConflict:
			default:
				unexpected <- fmt.Sprintf("unexpected concurrent exchange status %d", response.StatusCode)
			}
		}()
	}
	wait.Wait()
	close(unexpected)
	for message := range unexpected {
		t.Error(message)
	}
	if successes.Load() != 1 {
		t.Fatalf("successful concurrent exchanges = %d, want 1", successes.Load())
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind = 'client_exchange'`).Scan(&sessions); err != nil {
		t.Fatalf("count exchange sessions: %v", err)
	}
	if sessions != 1 {
		t.Fatalf("exchange sessions = %d, want 1", sessions)
	}
}

func openDependencies(t *testing.T, ctx context.Context) (*pgxpool.Pool, *redis.Client) {
	t.Helper()
	databaseURL := os.Getenv("PLATFORM_CORE_TEST_DATABASE_URL")
	redisAddr := os.Getenv("PLATFORM_CORE_TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		t.Skip("real PostgreSQL/Redis integration environment is not configured")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	t.Cleanup(func() { _ = redisClient.Close() })
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping Redis: %v", err)
	}
	return pool, redisClient
}

func resetIdentityTables(t *testing.T, ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client) {
	t.Helper()
	if _, err := pool.Exec(ctx, `TRUNCATE authorization_codes, sessions, oauth_clients, users CASCADE`); err != nil {
		t.Fatalf("reset identity tables: %v", err)
	}
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush Redis: %v", err)
	}
}

func seedIdentity(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	userID := uuid.New()
	sessionID := uuid.New()
	secretHash := sha256.Sum256([]byte(testClientSecret))
	tokenHash := sha256.Sum256([]byte(testCoreToken))
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email_verified, status) VALUES ($1, true, 'active')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_clients (id, secret_hash, redirect_uris) VALUES ($1, $2, $3)`, testClientID, secretHash[:], []string{testRedirectURI}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (id, user_id, kind, token_hash, expires_at) VALUES ($1, $2, 'core', $3, now() + interval '1 hour')`, sessionID, userID, tokenHash[:]); err != nil {
		t.Fatalf("seed core session: %v", err)
	}
}

func exchangeCode(t *testing.T, server *httptest.Server, code, verifier string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"grant_type":"authorization_code","code":%q,"redirect_uri":%q,"client_id":%q,"code_verifier":%q}`, code, testRedirectURI, testClientID, verifier)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(testClientID, testClientSecret)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("exchange code: %v", err)
	}
	return response
}

func issueAuthorizationCode(t *testing.T, server *httptest.Server, verifier string) string {
	t.Helper()
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	authorizeURL := server.URL + "/api/v1/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {testClientID},
		"redirect_uri":          {testRedirectURI},
		"state":                 {"state_test_0123456789"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()
	request, _ := http.NewRequest(http.MethodGet, authorizeURL, nil)
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_core_session", Value: testCoreToken})
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("authorize request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d, want 302", response.StatusCode)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse callback: %v", err)
	}
	if location.Scheme+"://"+location.Host+location.Path != testRedirectURI {
		t.Fatalf("callback = %q, want exact registered URI", location.String())
	}
	code := location.Query().Get("code")
	if code == "" || location.Query().Get("state") != "state_test_0123456789" {
		t.Fatalf("callback missing code or preserved state: %q", location.String())
	}
	assertSecureHostCookie(t, response.Cookies())
	if response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("authorization redirect must not be cached or leak its code through a Referer")
	}
	return code
}

func assertSecureHostCookie(t *testing.T, cookies []*http.Cookie) {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name != "__Host-henukit_core_session" {
			continue
		}
		if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
			t.Fatalf("unsafe Core Session cookie: %+v", cookie)
		}
		return
	}
	t.Fatal("Core Session cookie was not refreshed")
}
