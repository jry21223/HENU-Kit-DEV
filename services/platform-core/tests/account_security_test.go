package tests

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func postExplicitCredentialForm(t *testing.T, client *http.Client, endpoint string, values url.Values) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatalf("create explicit credential request: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Henukit-Form-Response", "status")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("submit explicit credential request: %v", err)
	}
	return response
}

func accountBootstrapCSRF(t *testing.T, client *http.Client, serverURL, flow string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, serverURL+"/account/bootstrap?flow="+url.QueryEscape(flow), nil)
	if err != nil {
		t.Fatalf("create %s Account Center Bootstrap: %v", flow, err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("open %s Account Center Bootstrap: %v", flow, err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data struct {
			Flow      string `json:"flow"`
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode %s Account Center Bootstrap: %v", flow, err)
	}
	if response.StatusCode != http.StatusOK || envelope.Data.Flow != flow || len(envelope.Data.CSRFToken) < 32 || envelope.RequestID == "" {
		t.Fatalf("%s Account Center Bootstrap = %d %+v, want bounded success", flow, response.StatusCode, envelope)
	}
	return envelope.Data.CSRFToken
}

func TestExplicitPasswordRecoveryReturnsStatusWithoutHTML(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "explicit-recovery-registration-seed"))
	client := clientForDevice(server, "explicit-recovery-device")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	csrfToken := accountBootstrapCSRF(t, client, server.URL, "recover")

	requested := postExplicitCredentialForm(t, client, server.URL+"/recover/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
	requestedBody, _ := io.ReadAll(requested.Body)
	requested.Body.Close()
	if requested.StatusCode != http.StatusNoContent || len(requestedBody) != 0 {
		t.Fatalf("explicit recovery-code request = %d %q, want empty 204", requested.StatusCode, requestedBody)
	}

	sender := &captureSender{messageID: "provider_explicit_recovery"}
	worker, err := mailworker.New(store.New(pool), sender, "worker_explicit_recovery", testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver explicit recovery code: outcome=%+v err=%v", outcome, err)
	}
	recovered := postExplicitCredentialForm(t, client, server.URL+"/recover", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
		"code": {sender.lastMessage().Code}, "password": {"recovered horse 电池 staple"},
	})
	recoveredBody, _ := io.ReadAll(recovered.Body)
	recovered.Body.Close()
	if recovered.StatusCode != http.StatusNoContent || recovered.Header.Get("Location") != "" || len(recoveredBody) != 0 {
		t.Fatalf("explicit recovery = %d location=%q body=%q, want empty 204", recovered.StatusCode, recovered.Header.Get("Location"), recoveredBody)
	}
	accountURL, _ := url.Parse(server.URL)
	var hasCoreSession bool
	for _, cookie := range client.Jar.Cookies(accountURL) {
		hasCoreSession = hasCoreSession || cookie.Name == "__Host-henukit_core_session" && cookie.Value != ""
	}
	if !hasCoreSession {
		t.Fatal("explicit recovery did not establish the new Core Session")
	}
}

func TestExplicitSecurityJourneyReturnsStatusWithoutHTML(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "explicit-security-registration-seed"))

	client := clientForDevice(server, "explicit-security-device")
	login := submitPasswordLogin(t, server, "explicit-security-device", testStudentEmail, "correct horse 电池 staple")
	login.Body.Close()
	if login.StatusCode != http.StatusSeeOther {
		t.Fatalf("create security Core Session = %d, want 303", login.StatusCode)
	}
	csrfToken := accountBootstrapCSRF(t, client, server.URL, "security")
	requested := postExplicitCredentialForm(t, client, server.URL+"/account/security/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
	requestedBody, _ := io.ReadAll(requested.Body)
	requested.Body.Close()
	if requested.StatusCode != http.StatusNoContent || len(requestedBody) != 0 {
		t.Fatalf("explicit security-code request = %d %q, want empty 204", requested.StatusCode, requestedBody)
	}

	sender := &captureSender{messageID: "provider_explicit_security_change"}
	worker, err := mailworker.New(store.New(pool), sender, "worker_explicit_security_change", testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver explicit security code: outcome=%+v err=%v", outcome, err)
	}
	changed := postExplicitCredentialForm(t, client, server.URL+"/account/security/password", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "code": {sender.lastMessage().Code},
		"current_password": {"correct horse 电池 staple"}, "new_password": {"changed horse 电池 staple"},
	})
	changedBody, _ := io.ReadAll(changed.Body)
	changed.Body.Close()
	if changed.StatusCode != http.StatusNoContent || changed.Header.Get("Location") != "" || len(changedBody) != 0 {
		t.Fatalf("explicit security change = %d location=%q body=%q, want empty 204", changed.StatusCode, changed.Header.Get("Location"), changedBody)
	}

	security, err := client.Get(server.URL + "/account/security")
	if err != nil {
		t.Fatalf("reuse Core Session after password change: %v", err)
	}
	security.Body.Close()
	if security.StatusCode != http.StatusOK {
		t.Fatalf("Core Session after password change = %d, want 200", security.StatusCode)
	}
	newPasswordLogin := submitPasswordLogin(t, server, "explicit-security-new-password", testStudentEmail, "changed horse 电池 staple")
	newPasswordLogin.Body.Close()
	if newPasswordLogin.StatusCode != http.StatusSeeOther {
		t.Fatalf("changed password login = %d, want 303", newPasswordLogin.StatusCode)
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
	response := postExplicitCredentialForm(t, currentClient, server.URL+"/account/security/password", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "code": {code},
		"current_password": {"correct horse 电池 staple"}, "new_password": {"changed horse 电池 staple"},
	})
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("explicit password change = %d, want 204", response.StatusCode)
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

func TestExplicitPasswordChangeRejectsMissingCoreSessionWithoutRedirect(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "expired-core-session")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	page, err := client.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("open login form for CSRF: %v", err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrf := csrfValuePattern.FindSubmatch(body)
	if len(csrf) != 2 {
		t.Fatal("login form omitted CSRF token")
	}

	response := postExplicitCredentialForm(t, client, server.URL+"/account/security/password", url.Values{
		"csrf_token": {string(csrf[1])}, "email": {testStudentEmail}, "code": {"123456"},
		"current_password": {"current password"}, "new_password": {"new password value"},
	})
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || response.Header.Get("Location") != "" {
		t.Fatalf("missing Core Session = %d location=%q, want non-redirecting 401", response.StatusCode, response.Header.Get("Location"))
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
	csrfToken := accountBootstrapCSRF(t, client, server.URL, "recover")
	response := postExplicitCredentialForm(t, client, server.URL+"/recover", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "code": {code},
		"password": {"redis failure must not commit"},
	})
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode Redis-failed recovery response: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || envelope.Error.Code != "DEPENDENCY_UNAVAILABLE" || envelope.RequestID == "" {
		t.Fatalf("Redis-failed recovery = %d %+v, want bounded 503 dependency response", response.StatusCode, envelope)
	}
	if strings.Contains(strings.ToLower(envelope.Error.Message), "redis") {
		t.Fatalf("dependency response exposed implementation detail: %+v", envelope)
	}
	oldPasswordLogin := submitPasswordLogin(t, seedServer, "redis-failure-old-password", testStudentEmail, "correct horse 电池 staple")
	oldPasswordLogin.Body.Close()
	if oldPasswordLogin.StatusCode != http.StatusSeeOther {
		t.Fatalf("old password after Redis failure = %d, want 303", oldPasswordLogin.StatusCode)
	}
	newPasswordLogin := submitPasswordLogin(t, seedServer, "redis-failure-new-password", testStudentEmail, "redis failure must not commit")
	newPasswordLogin.Body.Close()
	if newPasswordLogin.StatusCode == http.StatusSeeOther {
		t.Fatal("Redis-failed recovery changed the password")
	}
}

func TestConcurrentRecoverySerializesChangeAndOldPasswordLogin(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "serialized-recovery-seed"))

	currentClient := clientForDevice(server, "serialized-change-device")
	currentLogin := submitPasswordLogin(t, server, "serialized-change-device", testStudentEmail, "correct horse 电池 staple")
	currentLogin.Body.Close()
	if currentLogin.StatusCode != http.StatusSeeOther {
		t.Fatalf("create Session for queued change = %d, want 303", currentLogin.StatusCode)
	}
	securityPage, err := currentClient.Get(server.URL + "/account/security")
	if err != nil {
		t.Fatalf("open queued security page: %v", err)
	}
	securityBody, _ := io.ReadAll(securityPage.Body)
	securityPage.Body.Close()
	securityCSRF := csrfValuePattern.FindSubmatch(securityBody)
	if len(securityCSRF) != 2 {
		t.Fatal("queued security page omitted CSRF token")
	}
	recoveryClient := clientForDevice(server, "serialized-recovery-device")
	recoveryCSRF, code := prepareCredentialCode(t, ctx, pool, server, recoveryClient, "/recover", "/recover/code")
	loginClient := clientForDevice(server, "serialized-old-login-device")
	loginClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	loginPage, err := loginClient.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("open queued login page: %v", err)
	}
	loginBody, _ := io.ReadAll(loginPage.Body)
	loginPage.Body.Close()
	loginCSRF := csrfValuePattern.FindSubmatch(loginBody)
	if len(loginCSRF) != 2 {
		t.Fatal("queued login page omitted CSRF token")
	}

	lockTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin credential lock holder: %v", err)
	}
	defer func() { _ = lockTx.Rollback(ctx) }()
	if _, err := lockTx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, testCredentialLockID(testStudentEmail)); err != nil {
		t.Fatalf("hold credential scope: %v", err)
	}
	observer, err := pgx.Connect(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatalf("open independent lock observer: %v", err)
	}
	defer func() { _ = observer.Close(ctx) }()

	type httpResult struct {
		response *http.Response
		err      error
	}
	recoveryResult := make(chan httpResult, 1)
	go func() {
		response, requestErr := recoveryClient.PostForm(server.URL+"/recover", url.Values{
			"csrf_token": {recoveryCSRF}, "email": {testStudentEmail}, "code": {code},
			"password": {"serialized recovered password"},
		})
		recoveryResult <- httpResult{response: response, err: requestErr}
	}()
	waitForCredentialLockWaiters(t, ctx, observer, 1)

	changeResult := make(chan httpResult, 1)
	go func() {
		response, requestErr := currentClient.PostForm(server.URL+"/account/security/password", url.Values{
			"csrf_token": {string(securityCSRF[1])}, "email": {testStudentEmail}, "code": {code},
			"current_password": {"correct horse 电池 staple"}, "new_password": {"queued changed password"},
		})
		changeResult <- httpResult{response: response, err: requestErr}
	}()
	waitForCredentialLockWaiters(t, ctx, observer, 2)

	loginResult := make(chan httpResult, 1)
	go func() {
		response, requestErr := loginClient.PostForm(server.URL+"/login/password", url.Values{
			"csrf_token": {string(loginCSRF[1])}, "email": {testStudentEmail},
			"password": {"correct horse 电池 staple"}, "return_to": {"/"},
		})
		loginResult <- httpResult{response: response, err: requestErr}
	}()
	waitForCredentialLockWaiters(t, ctx, observer, 3)
	if err := lockTx.Commit(ctx); err != nil {
		t.Fatalf("release credential scope: %v", err)
	}

	recovered := <-recoveryResult
	if recovered.err != nil {
		t.Fatalf("complete queued recovery: %v", recovered.err)
	}
	recovered.response.Body.Close()
	if recovered.response.StatusCode != http.StatusSeeOther {
		t.Fatalf("queued recovery = %d, want 303", recovered.response.StatusCode)
	}
	changed := <-changeResult
	if changed.err != nil {
		t.Fatalf("complete queued password change: %v", changed.err)
	}
	changedBody, _ := io.ReadAll(changed.response.Body)
	changed.response.Body.Close()
	if changed.response.StatusCode != http.StatusOK || !strings.Contains(string(changedBody), "无法更改密码") {
		t.Fatalf("password change after recovery = %d %s, want generic rejection", changed.response.StatusCode, changedBody)
	}
	oldLogin := <-loginResult
	if oldLogin.err != nil {
		t.Fatalf("complete queued old-password login: %v", oldLogin.err)
	}
	oldLoginBody, _ := io.ReadAll(oldLogin.response.Body)
	oldLogin.response.Body.Close()
	if oldLogin.response.StatusCode != http.StatusOK || !strings.Contains(string(oldLoginBody), "邮箱或密码错误") {
		t.Fatalf("old-password login after recovery = %d %s, want generic rejection", oldLogin.response.StatusCode, oldLoginBody)
	}
	var activeCore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind='core' AND revoked_at IS NULL`).Scan(&activeCore); err != nil {
		t.Fatalf("count serialized recovery sessions: %v", err)
	}
	if activeCore != 1 {
		t.Fatalf("serialized recovery left %d active Core Sessions, want 1", activeCore)
	}
}

func testCredentialLockID(email string) int64 {
	mac := hmac.New(sha256.New, testVerificationEncryptionKey)
	_, _ = mac.Write([]byte("henukit-verification:email"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(email))
	return int64(binary.BigEndian.Uint64(mac.Sum(nil)[:8]))
}

func waitForCredentialLockWaiters(t *testing.T, ctx context.Context, observer *pgx.Conn, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var count int
		err := observer.QueryRow(ctx, `
			SELECT count(*)
			FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND wait_event = 'advisory'
			  AND query LIKE 'SELECT pg_advisory_xact_lock%'
		`).Scan(&count)
		if err != nil {
			t.Fatalf("inspect credential lock waiters: %v", err)
		}
		if count >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("credential lock waiters did not reach %d", want)
}
