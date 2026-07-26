package tests

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/mailworker"
	"henukit.dev/platform-core/internal/store"
)

func prepareCredentialCode(t *testing.T, ctx context.Context, pool *pgxpool.Pool, server *httptest.Server, client *http.Client, pagePath, codePath string) (string, string) {
	t.Helper()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	page, err := client.Get(server.URL + pagePath)
	if err != nil {
		t.Fatalf("open credential page: %v", err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrfMatch := csrfValuePattern.FindSubmatch(body)
	if len(csrfMatch) != 2 {
		t.Fatalf("credential page omitted CSRF token: %s", body)
	}
	csrfToken := string(csrfMatch[1])
	requested, err := client.PostForm(server.URL+codePath, url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
	if err != nil {
		t.Fatalf("request credential code: %v", err)
	}
	requested.Body.Close()
	if requested.StatusCode != http.StatusOK {
		t.Fatalf("request credential code = %d, want 200", requested.StatusCode)
	}
	id := uuid.NewString()
	sender := &captureSender{messageID: "provider_credential_" + id}
	worker, err := mailworker.New(store.New(pool), sender, "worker_credential_"+id, testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("create credential mail worker: %v", err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver credential code: outcome=%+v err=%v", outcome, err)
	}
	return csrfToken, sender.lastMessage().Code
}

func seedOtherSessions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID string) {
	t.Helper()
	coreID := uuid.New()
	coreHash := sha256.Sum256([]byte("other-core-" + uuid.NewString()))
	exchangeHash := sha256.Sum256([]byte("other-exchange-" + uuid.NewString()))
	if _, err := pool.Exec(ctx, `
		INSERT INTO oauth_clients (id, redirect_uris)
		VALUES ('security-test', ARRAY['https://security.example/callback'])
		ON CONFLICT (id) DO NOTHING
	`); err != nil {
		t.Fatalf("seed OAuth client: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (id,user_id,kind,token_hash,expires_at)
		VALUES ($1,$2,'core',$3,now()+interval '1 day')
	`, coreID, userID, coreHash[:]); err != nil {
		t.Fatalf("seed other core session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id,kind,token_hash,client_id,parent_session_id,expires_at)
		VALUES ($1,'client_exchange',$2,'security-test',$3,now()+interval '1 hour')
	`, userID, exchangeHash[:], coreID); err != nil {
		t.Fatalf("seed other sessions: %v", err)
	}
}

func TestPasswordRecoveryRevokesEveryOldSessionAndIssuesOneNewCoreSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "recovery-seed"))
	var userID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users`).Scan(&userID); err != nil {
		t.Fatalf("read recovery user: %v", err)
	}
	seedOtherSessions(t, ctx, pool, userID)

	client := clientForDevice(server, "recovery-device")
	csrfToken, code := prepareCredentialCode(t, ctx, pool, server, client, "/recover", "/recover/code")
	response, err := client.PostForm(server.URL+"/recover", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "code": {code},
		"password": {"recovered horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("recover password: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || response.Header.Get("Location") != "/account/security" {
		t.Fatalf("recover password = %d %q, want 303 to security", response.StatusCode, response.Header.Get("Location"))
	}
	var issuedSecureCore bool
	for _, cookie := range response.Cookies() {
		issuedSecureCore = issuedSecureCore ||
			cookie.Name == "__Host-henukit_core_session" && cookie.Value != "" &&
				cookie.Secure && cookie.HttpOnly && cookie.Path == "/" && cookie.Domain == ""
	}
	if !issuedSecureCore {
		t.Fatalf("recovery did not issue the production Core Session cookie: %+v", response.Cookies())
	}
	var activeCore, activeExchange, consumed int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE kind='core' AND revoked_at IS NULL),
			count(*) FILTER (WHERE kind='client_exchange' AND revoked_at IS NULL),
			(SELECT count(*) FROM verification_codes WHERE used_at IS NOT NULL)
		FROM sessions WHERE user_id=$1
	`, userID).Scan(&activeCore, &activeExchange, &consumed); err != nil {
		t.Fatalf("read recovery session facts: %v", err)
	}
	if activeCore != 1 || activeExchange != 0 || consumed != 1 {
		t.Fatalf("recovery sessions core=%d exchange=%d consumed=%d", activeCore, activeExchange, consumed)
	}
	login := submitPasswordLogin(t, server, "recovered-password-login", testStudentEmail, "recovered horse 电池 staple")
	login.Body.Close()
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("recovered password login = %d, want 303", login.StatusCode)
	}
}

func TestAuthenticatedPasswordChangeRetainsOnlyCurrentCoreSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "change-seed"))

	currentClient := clientForDevice(server, "current-session-device")
	login := submitPasswordLogin(t, server, "current-session-device", testStudentEmail, "correct horse 电池 staple")
	login.Body.Close()
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("create current session = %d, want 303", login.StatusCode)
	}
	var userID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users`).Scan(&userID); err != nil {
		t.Fatalf("read change user: %v", err)
	}
	seedOtherSessions(t, ctx, pool, userID)
	csrfToken, code := prepareCredentialCode(t, ctx, pool, server, currentClient, "/account/security", "/account/security/code")
	response, err := currentClient.PostForm(server.URL+"/account/security/password", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "code": {code},
		"current_password": {"correct horse 电池 staple"}, "new_password": {"changed horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("change password = %d, want 303", response.StatusCode)
	}
	var activeCore, activeExchange int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE kind='core' AND revoked_at IS NULL),
			count(*) FILTER (WHERE kind='client_exchange' AND revoked_at IS NULL)
		FROM sessions WHERE user_id=$1
	`, userID).Scan(&activeCore, &activeExchange); err != nil {
		t.Fatalf("read changed session facts: %v", err)
	}
	if activeCore != 1 || activeExchange != 0 {
		t.Fatalf("changed sessions core=%d exchange=%d", activeCore, activeExchange)
	}
	security, err := currentClient.Get(server.URL + "/account/security")
	if err != nil {
		t.Fatalf("reuse current session after password change: %v", err)
	}
	security.Body.Close()
	if security.StatusCode != http.StatusOK {
		t.Fatalf("current session after change = %d, want 200", security.StatusCode)
	}
	login = submitPasswordLogin(t, server, "changed-password-login", testStudentEmail, "changed horse 电池 staple")
	login.Body.Close()
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("changed password login = %d, want 303", login.StatusCode)
	}
}

func TestPasswordFailuresEscalateToCodeLoginAndCodeLoginClearsChallenge(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "challenge-seed"))

	for attempt := 1; attempt <= 5; attempt++ {
		response := submitPasswordLogin(t, server, "challenge-device", testStudentEmail, "wrong password value")
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if attempt < 5 && !strings.Contains(string(body), "邮箱或密码错误") {
			t.Fatalf("failure %d did not use generic message: %s", attempt, body)
		}
		if attempt == 5 && !strings.Contains(string(body), "改用邮箱验证码登录") {
			t.Fatalf("failure %d did not escalate to code login: %s", attempt, body)
		}
	}

	client := clientForDevice(server, "challenge-device")
	page, err := client.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("open code login: %v", err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrf := csrfValuePattern.FindSubmatch(pageBody)
	requested, err := client.PostForm(server.URL+"/login/code", url.Values{
		"csrf_token": {string(csrf[1])}, "email": {testStudentEmail},
	})
	if err != nil {
		t.Fatalf("request challenge code: %v", err)
	}
	requested.Body.Close()
	sender := &captureSender{messageID: "provider_challenge_clear"}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_challenge_clear", testVerificationEncryptionKey, time.Minute, time.Second)
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver challenge code: outcome=%+v err=%v", outcome, err)
	}
	verified, err := client.PostForm(server.URL+"/login/verify", url.Values{
		"csrf_token": {string(csrf[1])}, "email": {testStudentEmail},
		"code": {sender.lastMessage().Code}, "return_to": {"/"},
	})
	if err != nil {
		t.Fatalf("complete challenge code login: %v", err)
	}
	verified.Body.Close()
	if verified.StatusCode != http.StatusSeeOther {
		t.Fatalf("challenge code login = %d, want 303", verified.StatusCode)
	}

	passwordLogin := submitPasswordLogin(t, server, "challenge-device", testStudentEmail, "correct horse 电池 staple")
	passwordLogin.Body.Close()
	if passwordLogin.StatusCode != http.StatusSeeOther {
		t.Fatalf("password login after challenge clear = %d, want 303", passwordLogin.StatusCode)
	}
}

func TestPasswordRecoveryFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedServer := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, seedServer.URL, clientForDevice(seedServer, "recovery-redis-seed"))
	codeClient := clientForDevice(seedServer, "recovery-code-device")
	_, code := prepareCredentialCode(t, ctx, pool, seedServer, codeClient, "/recover", "/recover/code")

	var originalVerifier string
	if err := pool.QueryRow(ctx, `SELECT verifier FROM password_credentials`).Scan(&originalVerifier); err != nil {
		t.Fatalf("read original verifier: %v", err)
	}
	unavailableRedis := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond, WriteTimeout: 50 * time.Millisecond,
		MaxRetries: 0,
	})
	t.Cleanup(func() { _ = unavailableRedis.Close() })
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: unavailableRedis, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey, StudentEmailDomains: []string{"henu.edu.cn"},
	})
	if err != nil {
		t.Fatalf("create recovery server with unavailable Redis: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client := clientForDevice(server, "recovery-redis-device")
	page, err := client.Get(server.URL + "/recover")
	if err != nil {
		t.Fatalf("open recovery page with unavailable Redis: %v", err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrf := csrfValuePattern.FindSubmatch(pageBody)
	if len(csrf) != 2 {
		t.Fatal("recovery page omitted CSRF token")
	}
	response, err := client.PostForm(server.URL+"/recover", url.Values{
		"csrf_token": {string(csrf[1])}, "email": {testStudentEmail}, "code": {code},
		"password": {"redis failure must not commit"},
	})
	if err != nil {
		t.Fatalf("submit recovery with unavailable Redis: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "无法重置密码") {
		t.Fatalf("Redis-failed recovery = %d %s, want generic fail-closed response", response.StatusCode, body)
	}
	var verifier string
	var sessions, consumed int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT verifier FROM password_credentials),
			(SELECT count(*) FROM sessions),
			(SELECT count(*) FROM verification_codes WHERE used_at IS NOT NULL)
	`).Scan(&verifier, &sessions, &consumed); err != nil {
		t.Fatalf("read Redis-failed recovery facts: %v", err)
	}
	if verifier != originalVerifier || sessions != 0 || consumed != 0 {
		t.Fatalf("Redis-failed recovery mutated verifier/session/code: changed=%t sessions=%d consumed=%d",
			verifier != originalVerifier, sessions, consumed)
	}
}
