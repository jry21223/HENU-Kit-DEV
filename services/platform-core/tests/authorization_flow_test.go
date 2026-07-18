package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
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
	testClientID       = "console-gateway"
	testClientSecret   = "test-client-secret-with-enough-entropy"
	testRedirectURI    = "https://console.henukit.test/auth/callback"
	testCoreToken      = "core_test_session_token_32_bytes_long"
	testKeyID          = "primary"
	testRetiringSecret = "retiring-client-secret-with-enough-entropy"
)

var testIdempotencyEncryptionKey = []byte("0123456789abcdef0123456789abcdef")

func TestAuthorizationCodeIsSingleUseAndCreatesDurableSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)

	handler, err := platformcore.New(platformcore.Config{
		Database:                 pool,
		Redis:                    redisClient,
		CoreCookieName:           "__Host-henukit_core_session",
		AuthorizationTTL:         90 * time.Second,
		ExchangeSessionTTL:       5 * time.Minute,
		IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
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
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey})
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
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey})
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
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey})
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

func TestHMACNonceIdempotencyAndRequestAudit(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	var logs bytes.Buffer
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	server.Client().CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	verifier := "test-pkce-verifier-that-is-at-least-forty-three-characters"
	code := issueAuthorizationCode(t, server, verifier)
	idempotencyKey := "idem_" + uuid.NewString()
	badSignature := exchangeCodeWith(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "not-a-valid-signature")
	badSignature.Body.Close()
	if badSignature.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d, want 401", badSignature.StatusCode)
	}
	wrongKeyRequest := signedExchangeRequest(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "", "")
	wrongKeyRequest.Header.Set("X-Key-Id", "retired-key")
	wrongKeyResponse, err := server.Client().Do(wrongKeyRequest)
	if err != nil {
		t.Fatalf("wrong key request: %v", err)
	}
	wrongKeyResponse.Body.Close()
	if wrongKeyResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong key status = %d, want 401", wrongKeyResponse.StatusCode)
	}
	expiredTimestamp := fmt.Sprintf("%d", time.Now().Add(-6*time.Minute).Unix())
	expiredRequest := signedExchangeRequest(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), expiredTimestamp, "")
	expiredResponse, err := server.Client().Do(expiredRequest)
	if err != nil {
		t.Fatalf("expired timestamp request: %v", err)
	}
	expiredResponse.Body.Close()
	if expiredResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired timestamp status = %d, want 401", expiredResponse.StatusCode)
	}
	firstNonce := "nonce_" + uuid.NewString()
	first := exchangeCodeWith(t, server, code, verifier, idempotencyKey, firstNonce, "")
	firstBody, _ := io.ReadAll(first.Body)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("signed exchange status = %d: %s", first.StatusCode, firstBody)
	}
	second := exchangeCodeWith(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "")
	secondBody, _ := io.ReadAll(second.Body)
	second.Body.Close()
	var firstEnvelope, secondEnvelope map[string]any
	_ = json.Unmarshal(firstBody, &firstEnvelope)
	_ = json.Unmarshal(secondBody, &secondEnvelope)
	if second.StatusCode != http.StatusOK || !reflect.DeepEqual(firstEnvelope["data"], secondEnvelope["data"]) {
		t.Fatalf("idempotent retry status/body = %d/%s, want original 200 response", second.StatusCode, secondBody)
	}
	token := firstEnvelope["data"].(map[string]any)["session_exchange_token"].(string)
	var cachedCiphertext []byte
	if err := pool.QueryRow(ctx, `SELECT response_ciphertext FROM oauth_exchange_idempotency WHERE client_id = $1 AND idempotency_key = $2`, testClientID, idempotencyKey).Scan(&cachedCiphertext); err != nil {
		t.Fatalf("read cached idempotency response: %v", err)
	}
	if bytes.Contains(cachedCiphertext, []byte(token)) {
		t.Fatal("idempotency cache persisted a plaintext Session token")
	}
	var idempotencyRetention time.Duration
	if err := pool.QueryRow(ctx, `SELECT expires_at - created_at FROM oauth_exchange_idempotency WHERE client_id = $1 AND idempotency_key = $2`, testClientID, idempotencyKey).Scan(&idempotencyRetention); err != nil || idempotencyRetention < 24*time.Hour-time.Minute {
		t.Fatalf("idempotency retention = %s (query error %v), want at least 24h", idempotencyRetention, err)
	}
	nonceReplay := exchangeCodeWith(t, server, code, verifier, "idem_"+uuid.NewString(), firstNonce, "")
	nonceReplay.Body.Close()
	if nonceReplay.StatusCode != http.StatusConflict {
		t.Fatalf("nonce replay status = %d, want 409", nonceReplay.StatusCode)
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind = 'client_exchange'`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("idempotent retry created %d exchange sessions (query error %v)", sessions, err)
	}
	rotatingCode := issueAuthorizationCode(t, server, verifier)
	rotatingRequest := signedExchangeRequest(t, server, rotatingCode, verifier, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "", "")
	rotatingRequest.Header.Set("X-Key-Id", "retiring-key")
	rotatingRequest.SetBasicAuth(testClientID, testRetiringSecret)
	signExchangeRequestWithSecret(t, rotatingRequest, testRetiringSecret)
	rotatingResponse, err := server.Client().Do(rotatingRequest)
	if err != nil {
		t.Fatalf("retiring key request: %v", err)
	}
	rotatingResponse.Body.Close()
	if rotatingResponse.StatusCode != http.StatusOK {
		t.Fatalf("retiring key status = %d, want 200", rotatingResponse.StatusCode)
	}
	queryCode := issueAuthorizationCode(t, server, verifier)
	queryRequest := signedExchangeRequest(t, server, queryCode, verifier, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "", "")
	queryRequest.URL.RawQuery = "trace=signed"
	signExchangeRequest(t, queryRequest)
	queryResponse, err := server.Client().Do(queryRequest)
	if err != nil {
		t.Fatalf("signed query request: %v", err)
	}
	queryResponse.Body.Close()
	if queryResponse.StatusCode != http.StatusOK {
		t.Fatalf("signed path/query status = %d, want 200", queryResponse.StatusCode)
	}
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/healthz", nil)
	request.Header.Set("X-Request-Id", "req_upstream_123")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	response.Body.Close()
	if response.Header.Get("X-Request-Id") != "req_upstream_123" {
		t.Fatal("upstream request id was not preserved")
	}
	logText := logs.String()
	if !strings.Contains(logText, `"request_id":"req_upstream_123"`) || strings.Contains(logText, testClientSecret) || strings.Contains(logText, code) {
		t.Fatalf("audit log missing request id or leaked a secret: %s", logText)
	}
}

func TestConcurrentIdempotentRetriesReturnFirstResult(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	server.Client().CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	verifier := "test-pkce-verifier-that-is-at-least-forty-three-characters"
	code := issueAuthorizationCode(t, server, verifier)
	idempotencyKey := "idem_" + uuid.NewString()
	results := make(chan string, 10)
	var wait sync.WaitGroup
	for range 10 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := exchangeCodeWith(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "")
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				results <- fmt.Sprintf("status:%d", response.StatusCode)
				return
			}
			var envelope struct {
				Data struct {
					Token string `json:"session_exchange_token"`
				} `json:"data"`
			}
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				results <- "decode:" + err.Error()
				return
			}
			results <- envelope.Data.Token
		}()
	}
	wait.Wait()
	close(results)
	first := ""
	for result := range results {
		if strings.HasPrefix(result, "status:") || strings.HasPrefix(result, "decode:") {
			t.Error(result)
			continue
		}
		if first == "" {
			first = result
		} else if result != first {
			t.Errorf("idempotent retry token differs from first result")
		}
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind = 'client_exchange'`).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("concurrent idempotent retries created %d sessions (query error %v)", sessions, err)
	}
}

func openDependencies(t *testing.T, ctx context.Context) (*pgxpool.Pool, *redis.Client) {
	t.Helper()
	databaseURL := os.Getenv("PLATFORM_CORE_TEST_DATABASE_URL")
	redisAddr := os.Getenv("PLATFORM_CORE_TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		t.Fatal("TestMain did not configure real PostgreSQL and Redis dependencies")
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
	retiringSecretHash := sha256.Sum256([]byte(testRetiringSecret))
	tokenHash := sha256.Sum256([]byte(testCoreToken))
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email_verified, status) VALUES ($1, true, 'active')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ($1, $2)`, testClientID, []string{testRedirectURI}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_client_keys (client_id, key_id, secret_hash, status) VALUES ($1, $2, $3, 'active'), ($1, 'retiring-key', $4, 'retiring')`, testClientID, testKeyID, secretHash[:], retiringSecretHash[:]); err != nil {
		t.Fatalf("seed client keys: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sessions (id, user_id, kind, token_hash, expires_at) VALUES ($1, $2, 'core', $3, now() + interval '1 hour')`, sessionID, userID, tokenHash[:]); err != nil {
		t.Fatalf("seed core session: %v", err)
	}
}

func exchangeCode(t *testing.T, server *httptest.Server, code, verifier string) *http.Response {
	t.Helper()
	return exchangeCodeWith(t, server, code, verifier, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "")
}

func exchangeCodeWith(t *testing.T, server *httptest.Server, code, verifier, idempotencyKey, nonce, signatureOverride string) *http.Response {
	t.Helper()
	req := signedExchangeRequest(t, server, code, verifier, idempotencyKey, nonce, "", signatureOverride)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("exchange code: %v", err)
	}
	return response
}

func signedExchangeRequest(t *testing.T, server *httptest.Server, code, verifier, idempotencyKey, nonce, timestamp, signatureOverride string) *http.Request {
	t.Helper()
	body := fmt.Sprintf(`{"grant_type":"authorization_code","code":%q,"redirect_uri":%q,"client_id":%q,"code_verifier":%q}`, code, testRedirectURI, testClientID, verifier)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(testClientID, testClientSecret)
	if timestamp == "" {
		timestamp = fmt.Sprintf("%d", time.Now().Unix())
	}
	req.Header.Set("X-Service-Id", testClientID)
	req.Header.Set("X-Key-Id", testKeyID)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Nonce", nonce)
	signExchangeRequest(t, req)
	signature := req.Header.Get("X-Signature")
	if signatureOverride != "" {
		signature = signatureOverride
	}
	req.Header.Set("X-Signature", signature)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	return req
}

func signExchangeRequest(t *testing.T, request *http.Request) {
	signExchangeRequestWithSecret(t, request, testClientSecret)
}

func signExchangeRequestWithSecret(t *testing.T, request *http.Request, secret string) {
	t.Helper()
	bodyReader, err := request.GetBody()
	if err != nil {
		t.Fatalf("reopen exchange body: %v", err)
	}
	body, err := io.ReadAll(bodyReader)
	bodyReader.Close()
	if err != nil {
		t.Fatalf("read exchange body: %v", err)
	}
	bodyHash := sha256.Sum256(body)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(bodyHash[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
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
