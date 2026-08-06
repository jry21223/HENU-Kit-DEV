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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/contract"
)

const (
	testClientID       = "console-gateway"
	testClientSecret   = "test-client-secret-with-enough-entropy"
	testRedirectURI    = "https://console.henukit.test/auth/callback"
	testCoreToken      = "core_test_session_token_32_bytes_long"
	testKeyID          = "primary"
	testRetiringSecret = "retiring-client-secret-with-enough-entropy"
	testRevokedSecret  = "revoked-client-secret-with-enough-entropy"
)

var testIdempotencyEncryptionKey = []byte("0123456789abcdef0123456789abcdef")
var testVerificationEncryptionKey = []byte("abcdef0123456789abcdef0123456789")

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
	code := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
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
			User struct {
				DisplayName string `json:"display_name"`
			} `json:"user"`
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
	if payload.Data.User.DisplayName != "OAuth 测试用户" {
		t.Fatalf("exchange display_name = %q, want registration display name", payload.Data.User.DisplayName)
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

	code := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
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
	code := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
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
	code := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
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
	code := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
	idempotencyKey := "idem_" + uuid.NewString()
	missingHeaderRequest := signedExchangeRequest(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "", "", exchangeClientCreds{})
	missingHeaderRequest.Header.Del(contract.IdempotencyKeyHeader)
	missingHeaderResponse, err := server.Client().Do(missingHeaderRequest)
	if err != nil {
		t.Fatalf("missing contract header request: %v", err)
	}
	missingHeaderResponse.Body.Close()
	if missingHeaderResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing contract header status = %d, want 400", missingHeaderResponse.StatusCode)
	}
	badSignature := exchangeCodeWith(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "not-a-valid-signature")
	badSignature.Body.Close()
	if badSignature.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d, want 401", badSignature.StatusCode)
	}
	wrongKeyRequest := signedExchangeRequest(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "", "", exchangeClientCreds{})
	wrongKeyRequest.Header.Set(contract.KeyIDHeader, "revoked-key")
	wrongKeyRequest.SetBasicAuth(testClientID, testRevokedSecret)
	signExchangeRequestWithSecret(t, wrongKeyRequest, testRevokedSecret)
	wrongKeyResponse, err := server.Client().Do(wrongKeyRequest)
	if err != nil {
		t.Fatalf("wrong key request: %v", err)
	}
	wrongKeyResponse.Body.Close()
	if wrongKeyResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong key status = %d, want 401", wrongKeyResponse.StatusCode)
	}
	expiredTimestamp := fmt.Sprintf("%d", time.Now().Add(-6*time.Minute).Unix())
	expiredRequest := signedExchangeRequest(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), expiredTimestamp, "", exchangeClientCreds{})
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
	idempotencyLockHash := sha256.Sum256([]byte(testClientID + "\x00" + idempotencyKey))
	if err := redisClient.Set(ctx, "platform-core:oauth-idempotency:"+hex.EncodeToString(idempotencyLockHash[:]), "stale-owner", 30*time.Second).Err(); err != nil {
		t.Fatalf("seed stale idempotency lock: %v", err)
	}
	second := exchangeCodeWith(t, server, code, verifier, idempotencyKey, "nonce_"+uuid.NewString(), "")
	secondBody, _ := io.ReadAll(second.Body)
	second.Body.Close()
	var firstEnvelope map[string]any
	_ = json.Unmarshal(firstBody, &firstEnvelope)
	if second.StatusCode != http.StatusConflict || !bytes.Contains(secondBody, []byte(`"AUTH_CODE_ALREADY_USED"`)) {
		t.Fatalf("completed exchange replay status/body = %d/%s, want safe 409 without credential recovery", second.StatusCode, secondBody)
	}
	token := firstEnvelope["data"].(map[string]any)["session_exchange_token"].(string)
	var cachedCiphertext []byte
	if err := pool.QueryRow(ctx, `SELECT response_ciphertext FROM oauth_exchange_idempotency WHERE client_id = $1 AND idempotency_key = $2`, testClientID, idempotencyKey).Scan(&cachedCiphertext); err != nil {
		t.Fatalf("read cached idempotency response: %v", err)
	}
	if len(cachedCiphertext) != 0 || bytes.Contains(cachedCiphertext, []byte(token)) {
		t.Fatal("idempotency cache persisted a recoverable Session token")
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
	rotatingCode := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
	rotatingRequest := signedExchangeRequest(t, server, rotatingCode, verifier, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "", "", exchangeClientCreds{})
	rotatingRequest.Header.Set(contract.KeyIDHeader, "retiring-key")
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
	queryCode := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
	queryRequest := signedExchangeRequest(t, server, queryCode, verifier, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "", "", exchangeClientCreds{})
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

func TestConcurrentIdempotentExchangesIssueOnlyOneCredential(t *testing.T) {
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
	code := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
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
	conflicts := 0
	for result := range results {
		if result == "status:409" {
			conflicts++
			continue
		}
		if strings.HasPrefix(result, "status:") || strings.HasPrefix(result, "decode:") {
			t.Error(result)
			continue
		}
		if first == "" {
			first = result
		} else {
			t.Errorf("concurrent exchange recovered a second credential")
		}
	}
	if first == "" || conflicts != 9 {
		t.Fatalf("concurrent exchange outcomes: credential=%v conflicts=%d, want one credential and nine conflicts", first != "", conflicts)
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
	if _, err := pool.Exec(ctx, `ALTER TABLE mail_outbox_audit_events DISABLE TRIGGER mail_outbox_audit_events_immutable; ALTER TABLE operations_inbox_audit_events DISABLE TRIGGER operations_inbox_audit_events_immutable; ALTER TABLE platform_operations_audit_events DISABLE TRIGGER platform_operations_audit_events_immutable; ALTER TABLE operator_bootstrap_audit_events DISABLE TRIGGER operator_bootstrap_audit_events_immutable; ALTER TABLE account_operator_role_grant_audit_events DISABLE TRIGGER account_operator_role_grant_audit_events_immutable; TRUNCATE account_operator_role_grant_audit_events, operator_bootstrap_audit_events, platform_operations_audit_events, platform_operations_idempotency, operations_inbox_audit_events, operations_inbox_idempotency, operations_inbox_items, mail_outbox_audit_events, mail_delivery_receipts, mail_outbox, verification_codes, permission_codes, authorization_roles, authorization_codes, sessions, oauth_clients, users CASCADE; ALTER TABLE account_operator_role_grant_audit_events ENABLE TRIGGER account_operator_role_grant_audit_events_immutable; ALTER TABLE operator_bootstrap_audit_events ENABLE TRIGGER operator_bootstrap_audit_events_immutable; ALTER TABLE platform_operations_audit_events ENABLE TRIGGER platform_operations_audit_events_immutable; ALTER TABLE operations_inbox_audit_events ENABLE TRIGGER operations_inbox_audit_events_immutable; ALTER TABLE mail_outbox_audit_events ENABLE TRIGGER mail_outbox_audit_events_immutable`); err != nil {
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
	revokedSecretHash := sha256.Sum256([]byte(testRevokedSecret))
	tokenHash := sha256.Sum256([]byte(testCoreToken))
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email_verified, status, display_name) VALUES ($1, true, 'active', 'OAuth 测试用户')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ($1, $2)`, testClientID, []string{testRedirectURI}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_client_keys (client_id, key_id, secret_hash, status) VALUES ($1, $2, $3, 'active'), ($1, 'retiring-key', $4, 'retiring'), ($1, 'revoked-key', $5, 'revoked')`, testClientID, testKeyID, secretHash[:], retiringSecretHash[:], revokedSecretHash[:]); err != nil {
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
	req := signedExchangeRequest(t, server, code, verifier, idempotencyKey, nonce, "", signatureOverride, exchangeClientCreds{})
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("exchange code: %v", err)
	}
	return response
}

// exchangeClientCreds selects the OAuth client signing an exchange request.
// The zero value resolves to the default console test client (testClientID,
// testClientSecret, testRedirectURI, testKeyID).
type exchangeClientCreds struct {
	clientID     string
	clientSecret string
	redirectURI  string
	keyID        string
}

func signedExchangeRequest(t *testing.T, server *httptest.Server, code, verifier, idempotencyKey, nonce, timestamp, signatureOverride string, creds exchangeClientCreds) *http.Request {
	t.Helper()
	clientID, clientSecret, redirectURI, keyID := creds.clientID, creds.clientSecret, creds.redirectURI, creds.keyID
	if clientID == "" {
		clientID, clientSecret, redirectURI, keyID = testClientID, testClientSecret, testRedirectURI, testKeyID
	}
	body := fmt.Sprintf(`{"grant_type":"authorization_code","code":%q,"redirect_uri":%q,"client_id":%q,"code_verifier":%q}`, code, redirectURI, clientID, verifier)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(clientID, clientSecret)
	if timestamp == "" {
		timestamp = fmt.Sprintf("%d", time.Now().Unix())
	}
	req.Header.Set(contract.ServiceIDHeader, clientID)
	req.Header.Set(contract.KeyIDHeader, keyID)
	req.Header.Set(contract.TimestampHeader, timestamp)
	req.Header.Set(contract.NonceHeader, nonce)
	signExchangeRequestWithSecret(t, req, clientSecret)
	signature := req.Header.Get(contract.SignatureHeader)
	if signatureOverride != "" {
		signature = signatureOverride
	}
	req.Header.Set(contract.SignatureHeader, signature)
	req.Header.Set(contract.IdempotencyKeyHeader, idempotencyKey)
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
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get(contract.TimestampHeader), request.Header.Get(contract.NonceHeader), hex.EncodeToString(bodyHash[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set(contract.SignatureHeader, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

// authorizeClientCreds selects the OAuth client for issueAuthorizationCode.
// The zero value resolves to the default console test client (testClientID,
// testRedirectURI).
type authorizeClientCreds struct {
	clientID    string
	redirectURI string
}

func issueAuthorizationCode(t *testing.T, server *httptest.Server, verifier string, creds authorizeClientCreds) string {
	t.Helper()
	clientID, redirectURI := creds.clientID, creds.redirectURI
	if clientID == "" {
		clientID = testClientID
	}
	if redirectURI == "" {
		redirectURI = testRedirectURI
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	authorizeURL := server.URL + "/api/v1/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
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
	if location.Scheme+"://"+location.Host+location.Path != redirectURI {
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

// exchangeRemaining exchanges code for a Session and returns the time left
// until the exchange Session expires. Unlike exchangeCodeWith it lets the
// caller pick the OAuth client credentials, which the per-client TTL override
// test needs.
func exchangeRemaining(t *testing.T, server *httptest.Server, code, verifier, clientID, secret, redirectURI string) time.Duration {
	t.Helper()
	req := signedExchangeRequest(t, server, code, verifier, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "", "", exchangeClientCreds{clientID: clientID, clientSecret: secret, redirectURI: redirectURI, keyID: "primary"})
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("exchange code: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("exchange status = %d, want 200", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode exchange response: %v", err)
	}
	if envelope.Data.ExpiresAt.IsZero() {
		t.Fatal("exchange response omitted expires_at")
	}
	return time.Until(envelope.Data.ExpiresAt)
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

// The 30-day stay-signed-in Portal requirement is delivered through the
// per-client exchange Session override: the portal-gateway client receives a
// 30-day exchange Session (driving the Portal Session cookie MaxAge and every
// subsequent permission check), while every other client keeps the short 8h
// high-privilege default. The Core Session TTL alone cannot extend the Portal
// Session because the exchange response's expires_at comes from the exchange
// Session, not the Core Session.
func TestExchangeSessionTTLOverrideExtendsPortalSessionsOnly(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)

	const portalClientID = "portal-gateway"
	const portalRedirectURI = "https://portal.henukit.test/api/v1/auth/callback"
	const portalSecret = "portal-client-secret-with-enough-entropy"
	portalSecretHash := sha256.Sum256([]byte(portalSecret))
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ($1, $2)`, portalClientID, []string{portalRedirectURI}); err != nil {
		t.Fatalf("seed portal client: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_client_keys (client_id, key_id, secret_hash, status) VALUES ($1, 'primary', $2, 'active')`, portalClientID, portalSecretHash[:]); err != nil {
		t.Fatalf("seed portal client key: %v", err)
	}

	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		ExchangeSessionTTL:          8 * time.Hour,
		ExchangeSessionTTLOverrides: map[string]time.Duration{portalClientID: 30 * 24 * time.Hour},
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	server.Client().CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	verifier := "test-pkce-verifier-that-is-at-least-forty-three-characters"

	consoleCode := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{})
	consoleRemaining := exchangeRemaining(t, server, consoleCode, verifier, testClientID, testClientSecret, testRedirectURI)
	if consoleRemaining < 7*time.Hour+59*time.Minute || consoleRemaining > 8*time.Hour+time.Minute {
		t.Fatalf("console exchange Session lifetime = %s, want the 8h default", consoleRemaining)
	}

	portalCode := issueAuthorizationCode(t, server, verifier, authorizeClientCreds{clientID: portalClientID, redirectURI: portalRedirectURI})
	portalRemaining := exchangeRemaining(t, server, portalCode, verifier, portalClientID, portalSecret, portalRedirectURI)
	if portalRemaining < 29*24*time.Hour+23*time.Hour || portalRemaining > 30*24*time.Hour+time.Minute {
		t.Fatalf("portal exchange Session lifetime = %s, want the 30-day override", portalRemaining)
	}

	var portalSessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind = 'client_exchange' AND client_id = $1`, portalClientID).Scan(&portalSessions); err != nil {
		t.Fatalf("count portal exchange sessions: %v", err)
	}
	if portalSessions != 1 {
		t.Fatalf("portal exchange sessions = %d, want 1", portalSessions)
	}
}
