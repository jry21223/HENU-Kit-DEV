package tests

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/mailworker"
	"henukit.dev/platform-core/internal/password"
	"henukit.dev/platform-core/internal/store"
)

var csrfValuePattern = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

func submitPasswordLogin(t *testing.T, server *httptest.Server, deviceID, email, passwordValue string) *http.Response {
	t.Helper()
	client := clientForDevice(server, deviceID)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	page, err := client.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("open password login page: %v", err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrfMatch := csrfValuePattern.FindSubmatch(body)
	if len(csrfMatch) != 2 {
		t.Fatal("password login page omitted CSRF token")
	}
	response, err := client.PostForm(server.URL+"/login/password", url.Values{
		"csrf_token": {string(csrfMatch[1])}, "email": {email},
		"password": {passwordValue}, "return_to": {"/"},
	})
	if err != nil {
		t.Fatalf("submit password login: %v", err)
	}
	return response
}

func prepareRegistrationCode(t *testing.T, ctx context.Context, pool *pgxpool.Pool, server *httptest.Server, deviceID string) (*http.Client, string, string) {
	t.Helper()
	client := clientForDevice(server, deviceID)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	page, err := client.Get(server.URL + "/register")
	if err != nil {
		t.Fatalf("open registration page: %v", err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrfMatch := csrfValuePattern.FindSubmatch(body)
	if len(csrfMatch) != 2 {
		t.Fatal("registration page omitted CSRF token")
	}
	csrfToken := string(csrfMatch[1])
	requested, err := client.PostForm(server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
	if err != nil {
		t.Fatalf("request registration code: %v", err)
	}
	requested.Body.Close()
	sender := &captureSender{messageID: "provider_prepare_" + uuid.NewString()}
	worker, err := mailworker.New(store.New(pool), sender, "worker_prepare_"+uuid.NewString(), testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("create registration worker: %v", err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver registration code: outcome=%+v err=%v", outcome, err)
	}
	return client, csrfToken, sender.lastMessage().Code
}

func TestAccountCenterRegistrationRejectsWeakPasswordWithoutConsumingCode(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	client, csrfToken, code := prepareRegistrationCode(t, ctx, pool, server, "weak-password-device")

	response, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"弱密码测试"}, "email": {testStudentEmail},
		"code": {code}, "password": {"password123"},
	})
	if err != nil {
		t.Fatalf("submit weak password registration: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "注册失败") {
		t.Fatalf("weak password registration = %d %s, want rejected form", response.StatusCode, body)
	}

	var users, credentials, sessions, consumed int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM password_credentials),
			(SELECT count(*) FROM sessions),
			(SELECT count(*) FROM verification_codes WHERE used_at IS NOT NULL)
	`).Scan(&users, &credentials, &sessions, &consumed); err != nil {
		t.Fatalf("read weak-password facts: %v", err)
	}
	if users != 0 || credentials != 0 || sessions != 0 || consumed != 0 {
		t.Fatalf("weak-password facts users=%d credentials=%d sessions=%d consumed=%d", users, credentials, sessions, consumed)
	}
}

func TestAccountCenterDuplicateRegistrationDoesNotOverwriteCredential(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedClient := clientForDevice(server, "duplicate-seed-device")
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, seedClient)

	var originalDisplayName, originalVerifier string
	if err := pool.QueryRow(ctx, `
		SELECT u.display_name, p.verifier
		FROM users u JOIN password_credentials p ON p.user_id = u.id
	`).Scan(&originalDisplayName, &originalVerifier); err != nil {
		t.Fatalf("read original registration: %v", err)
	}
	client, csrfToken, code := prepareRegistrationCode(t, ctx, pool, server, "duplicate-registration-device")
	response, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"覆盖尝试"}, "email": {testStudentEmail},
		"code": {code}, "password": {"another strong 密码 value"},
	})
	if err != nil {
		t.Fatalf("submit duplicate registration: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "已注册") {
		t.Fatalf("duplicate registration = %d %s, want already-registered form", response.StatusCode, body)
	}

	var displayName, verifier string
	var users, credentials, sessions, consumed int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT display_name FROM users LIMIT 1),
			(SELECT verifier FROM password_credentials LIMIT 1),
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM password_credentials),
			(SELECT count(*) FROM sessions),
			(SELECT count(*) FROM verification_codes WHERE used_at IS NOT NULL)
	`).Scan(&displayName, &verifier, &users, &credentials, &sessions, &consumed); err != nil {
		t.Fatalf("read duplicate-registration facts: %v", err)
	}
	if displayName != originalDisplayName || verifier != originalVerifier || users != 1 || credentials != 1 || sessions != 0 || consumed != 0 {
		t.Fatalf("duplicate registration mutated facts display=%q users=%d credentials=%d sessions=%d consumed=%d", displayName, users, credentials, sessions, consumed)
	}
}

func TestAccountCenterRegistrationRollsBackEveryFactWhenCredentialInsertFails(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	client, csrfToken, code := prepareRegistrationCode(t, ctx, pool, server, "registration-rollback-device")

	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION fail_password_credential_insert() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced credential insert failure'; END $$;
		CREATE TRIGGER password_credential_insert_failure
		BEFORE INSERT ON password_credentials
		FOR EACH ROW EXECUTE FUNCTION fail_password_credential_insert();
	`); err != nil {
		t.Fatalf("install credential failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS password_credential_insert_failure ON password_credentials;
			DROP FUNCTION IF EXISTS fail_password_credential_insert();
		`)
	})

	response, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"事务回滚"}, "email": {testStudentEmail},
		"code": {code}, "password": {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("submit registration with forced failure: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("registration with forced failure = %d, want rejected form", response.StatusCode)
	}

	var users, identities, credentials, sessions, consumed int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM email_identities),
			(SELECT count(*) FROM password_credentials),
			(SELECT count(*) FROM sessions),
			(SELECT count(*) FROM verification_codes WHERE used_at IS NOT NULL)
	`).Scan(&users, &identities, &credentials, &sessions, &consumed); err != nil {
		t.Fatalf("read rollback facts: %v", err)
	}
	if users != 0 || identities != 0 || credentials != 0 || sessions != 0 || consumed != 0 {
		t.Fatalf("rollback facts users=%d identities=%d credentials=%d sessions=%d consumed=%d", users, identities, credentials, sessions, consumed)
	}
}

func seedRegisteredAccount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, redisClient *redis.Client, serverURL string, client *http.Client) {
	t.Helper()
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() {
		client.CheckRedirect = previousRedirect
	}()
	page, err := client.Get(serverURL + "/register")
	if err != nil {
		t.Fatalf("open registration seed page: %v", err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrfMatch := csrfValuePattern.FindSubmatch(body)
	if len(csrfMatch) != 2 {
		t.Fatal("registration seed page omitted CSRF token")
	}
	csrfToken := string(csrfMatch[1])
	requested, err := client.PostForm(serverURL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
	if err != nil {
		t.Fatalf("request registration seed code: %v", err)
	}
	requested.Body.Close()
	id := uuid.NewString()
	sender := &captureSender{messageID: "provider_seed_" + id}
	worker, err := mailworker.New(store.New(pool), sender, "worker_seed_"+id, testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("create registration seed worker: %v", err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver registration seed code: outcome=%+v err=%v", outcome, err)
	}
	registered, err := client.PostForm(serverURL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"测试用户"}, "email": {testStudentEmail},
		"code": {sender.lastMessage().Code}, "password": {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("submit registration seed: %v", err)
	}
	registered.Body.Close()
	if registered.StatusCode != http.StatusSeeOther {
		t.Fatalf("registration seed = %d, want 303", registered.StatusCode)
	}
	if _, err := pool.Exec(ctx, `
		ALTER TABLE mail_outbox_audit_events DISABLE TRIGGER mail_outbox_audit_events_immutable;
		TRUNCATE mail_outbox_audit_events, mail_dead_letters, mail_delivery_receipts, mail_outbox, verification_codes, sessions CASCADE;
		ALTER TABLE mail_outbox_audit_events ENABLE TRIGGER mail_outbox_audit_events_immutable;
	`); err != nil {
		t.Fatalf("clear registration seed flow facts: %v", err)
	}
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("clear registration seed rate limits: %v", err)
	}
}

func TestAccountCenterRegistrationAtomicallyCreatesCredentialAndSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "registration-device")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	page, err := client.Get(server.URL + "/register")
	if err != nil {
		t.Fatalf("open registration page: %v", err)
	}
	pageBody, _ := io.ReadAll(page.Body)
	page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Fatalf("registration page = %d %s, want 200", page.StatusCode, pageBody)
	}
	csrfMatch := csrfValuePattern.FindSubmatch(pageBody)
	if len(csrfMatch) != 2 {
		t.Fatalf("registration page omitted CSRF token: %s", pageBody)
	}
	csrfToken := string(csrfMatch[1])

	requested, err := client.PostForm(server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken},
		"email":      {testStudentEmail},
	})
	if err != nil {
		t.Fatalf("request registration code: %v", err)
	}
	requested.Body.Close()
	if requested.StatusCode != http.StatusOK {
		t.Fatalf("request registration code = %d, want 200", requested.StatusCode)
	}
	sender := &captureSender{messageID: "provider_registration_001"}
	worker, err := mailworker.New(store.New(pool), sender, "worker_registration", testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("create registration mail worker: %v", err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver registration code: outcome=%+v err=%v", outcome, err)
	}

	registered, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token":   {csrfToken},
		"display_name": {"小河同学"},
		"email":        {testStudentEmail},
		"code":         {sender.lastMessage().Code},
		"password":     {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("submit registration: %v", err)
	}
	registered.Body.Close()
	if registered.StatusCode != http.StatusSeeOther || registered.Header.Get("Location") != "/" {
		t.Fatalf("registration completion = %d %q, want 303 to account root", registered.StatusCode, registered.Header.Get("Location"))
	}

	var users, identities, credentials, sessions, consumed int
	var displayName, verifier string
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM email_identities),
			(SELECT count(*) FROM password_credentials),
			(SELECT count(*) FROM sessions WHERE kind='core'),
			(SELECT count(*) FROM verification_codes WHERE used_at IS NOT NULL),
			(SELECT display_name FROM users LIMIT 1),
			(SELECT verifier FROM password_credentials LIMIT 1)
	`).Scan(&users, &identities, &credentials, &sessions, &consumed, &displayName, &verifier); err != nil {
		t.Fatalf("read registration facts: %v", err)
	}
	if users != 1 || identities != 1 || credentials != 1 || sessions != 1 || consumed != 1 {
		t.Fatalf("registration facts users=%d identities=%d credentials=%d sessions=%d consumed=%d", users, identities, credentials, sessions, consumed)
	}
	if displayName != "小河同学" || !strings.HasPrefix(verifier, "$argon2id$v=") {
		t.Fatalf("registration display/verifier = %q %q", displayName, verifier)
	}
	accountURL, _ := url.Parse(server.URL)
	var hasCoreSession bool
	for _, cookie := range client.Jar.Cookies(accountURL) {
		hasCoreSession = hasCoreSession || cookie.Name == "__Host-henukit_core_session" && cookie.Value != ""
	}
	if !hasCoreSession {
		t.Fatal("registration did not establish a Core Session cookie")
	}
}

func TestAccountCenterPasswordLoginCreatesCoreSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)

	registrationClient := clientForDevice(server, "password-seed-device")
	registrationPage, err := registrationClient.Get(server.URL + "/register")
	if err != nil {
		t.Fatalf("open registration page: %v", err)
	}
	registrationBody, _ := io.ReadAll(registrationPage.Body)
	registrationPage.Body.Close()
	csrfMatch := csrfValuePattern.FindSubmatch(registrationBody)
	if len(csrfMatch) != 2 {
		t.Fatal("registration page omitted CSRF token")
	}
	csrfToken := string(csrfMatch[1])
	requested, err := registrationClient.PostForm(server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
	if err != nil {
		t.Fatalf("request registration code: %v", err)
	}
	requested.Body.Close()
	sender := &captureSender{messageID: "provider_password_login_seed"}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_password_login_seed", testVerificationEncryptionKey, time.Minute, time.Second)
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver registration code: outcome=%+v err=%v", outcome, err)
	}
	registered, err := registrationClient.PostForm(server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"登录测试"}, "email": {testStudentEmail},
		"code": {sender.lastMessage().Code}, "password": {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("seed registered account: %v", err)
	}
	registered.Body.Close()

	loginClient := clientForDevice(server, "password-login-device")
	loginClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	loginPage, err := loginClient.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("open login page: %v", err)
	}
	loginBody, _ := io.ReadAll(loginPage.Body)
	loginPage.Body.Close()
	loginCSRF := csrfValuePattern.FindSubmatch(loginBody)
	if len(loginCSRF) != 2 {
		t.Fatal("login page omitted CSRF token")
	}
	loggedIn, err := loginClient.PostForm(server.URL+"/login/password", url.Values{
		"csrf_token": {string(loginCSRF[1])}, "email": {testStudentEmail},
		"password": {"correct horse 电池 staple"}, "return_to": {"/"},
	})
	if err != nil {
		t.Fatalf("password login: %v", err)
	}
	loggedIn.Body.Close()
	if loggedIn.StatusCode != http.StatusSeeOther || loggedIn.Header.Get("Location") != "/" {
		t.Fatalf("password login = %d %q, want 303 to account root", loggedIn.StatusCode, loggedIn.Header.Get("Location"))
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE kind='core' AND revoked_at IS NULL`).Scan(&sessions); err != nil {
		t.Fatalf("count password login sessions: %v", err)
	}
	if sessions != 2 {
		t.Fatalf("active Core Sessions = %d, want registration plus password login", sessions)
	}
	accountURL, _ := url.Parse(server.URL)
	var hasCoreSession bool
	for _, cookie := range loginClient.Jar.Cookies(accountURL) {
		hasCoreSession = hasCoreSession || cookie.Name == "__Host-henukit_core_session" && cookie.Value != ""
	}
	if !hasCoreSession {
		t.Fatal("password login did not establish a Core Session cookie")
	}
}

func TestAccountCenterPasswordLoginUpgradesOldArgon2idVerifier(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedClient := clientForDevice(server, "rehash-seed-device")
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, seedClient)

	oldManager, err := password.New(password.Parameters{
		MemoryKiB: 32 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32,
	}, 1)
	if err != nil {
		t.Fatalf("create old password manager: %v", err)
	}
	oldVerifier, err := oldManager.Hash(ctx, "correct horse 电池 staple")
	if err != nil {
		t.Fatalf("create old verifier: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE password_credentials SET verifier=$1`, oldVerifier); err != nil {
		t.Fatalf("install old verifier: %v", err)
	}

	loginClient := clientForDevice(server, "rehash-login-device")
	loginClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	page, err := loginClient.Get(server.URL + "/login")
	if err != nil {
		t.Fatalf("open password login: %v", err)
	}
	body, _ := io.ReadAll(page.Body)
	page.Body.Close()
	csrfMatch := csrfValuePattern.FindSubmatch(body)
	if len(csrfMatch) != 2 {
		t.Fatal("password login page omitted CSRF token")
	}
	response, err := loginClient.PostForm(server.URL+"/login/password", url.Values{
		"csrf_token": {string(csrfMatch[1])}, "email": {testStudentEmail},
		"password": {"correct horse 电池 staple"}, "return_to": {"/"},
	})
	if err != nil {
		t.Fatalf("password login with old verifier: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther {
		t.Fatalf("password login with old verifier = %d, want 303", response.StatusCode)
	}

	var upgradedVerifier string
	var policyVersion int32
	if err := pool.QueryRow(ctx, `SELECT verifier, policy_version FROM password_credentials`).Scan(&upgradedVerifier, &policyVersion); err != nil {
		t.Fatalf("read upgraded verifier: %v", err)
	}
	if upgradedVerifier == oldVerifier || policyVersion != password.PolicyVersion {
		t.Fatalf("verifier upgrade = changed:%t policy:%d", upgradedVerifier != oldVerifier, policyVersion)
	}
}

func TestAccountCenterPasswordLoginUsesGenericFailureForUnknownAndWrongCredentials(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "generic-failure-seed"))

	for _, test := range []struct {
		name, deviceID, email, password string
	}{
		{name: "known email wrong password", deviceID: "wrong-password-device", email: testStudentEmail, password: "wrong password value"},
		{name: "unknown email", deviceID: "unknown-email-device", email: "nobody@henu.edu.cn", password: "wrong password value"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := submitPasswordLogin(t, server, test.deviceID, test.email, test.password)
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "邮箱或密码错误，或登录暂不可用。") {
				t.Fatalf("failed password login = %d %s, want generic failure", response.StatusCode, body)
			}
		})
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions after failed logins: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("failed password logins created %d sessions", sessions)
	}
}

func TestAccountCenterPasswordLoginFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedServer := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, seedServer.URL, clientForDevice(seedServer, "redis-failure-seed"))

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
		t.Fatalf("create Platform Core with unavailable Redis: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	response := submitPasswordLogin(t, server, "redis-failure-login", testStudentEmail, "correct horse 电池 staple")
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "邮箱或密码错误，或登录暂不可用。") {
		t.Fatalf("Redis-failed password login = %d %s, want generic fail-closed response", response.StatusCode, body)
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions after Redis failure: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("Redis-failed password login created %d sessions", sessions)
	}
}
