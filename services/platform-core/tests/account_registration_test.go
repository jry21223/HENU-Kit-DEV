package tests

import (
	"bytes"
	"context"
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
	"henukit.dev/platform-core/internal/password"
	"henukit.dev/platform-core/internal/store"
)

func TestExplicitAccountRegistrationReturnsStatusWithoutHTML(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "explicit-registration-device")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	csrfToken := accountBootstrapCSRF(t, client, server.URL, "register")

	requested := postExplicitCredentialForm(t, client, server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
		"return_to": {"/api/v1/auth/login?return_to=%2Faccount%2Fsecurity"},
	})
	requestedBody, _ := io.ReadAll(requested.Body)
	requested.Body.Close()
	if requested.StatusCode != http.StatusNoContent || len(requestedBody) != 0 {
		t.Fatalf("explicit registration-code request = %d %q, want empty 204", requested.StatusCode, requestedBody)
	}

	sender := &captureSender{messageID: "provider_explicit_registration"}
	worker, err := mailworker.New(store.New(pool), sender, "worker_explicit_registration", testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver explicit registration code: outcome=%+v err=%v", outcome, err)
	}
	registered := postExplicitCredentialForm(t, client, server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"小河同学"},
		"email": {testStudentEmail}, "code": {sender.lastMessage().Code},
		"password":  {"correct horse 电池 staple"},
		"return_to": {"/api/v1/auth/login?return_to=%2Faccount%2Fsecurity"},
	})
	registeredBody, _ := io.ReadAll(registered.Body)
	registered.Body.Close()
	if registered.StatusCode != http.StatusNoContent || registered.Header.Get("Location") != "" || len(registeredBody) != 0 {
		t.Fatalf("explicit registration = %d location=%q body=%q, want empty 204", registered.StatusCode, registered.Header.Get("Location"), registeredBody)
	}
	accountURL, _ := url.Parse(server.URL)
	var hasCoreSession bool
	for _, cookie := range client.Jar.Cookies(accountURL) {
		hasCoreSession = hasCoreSession || cookie.Name == "__Host-henukit_core_session" && cookie.Value != ""
	}
	if !hasCoreSession {
		t.Fatal("explicit registration did not establish the Core Session")
	}
}

func submitPasswordLogin(t *testing.T, server *httptest.Server, deviceID, email, passwordValue string) *http.Response {
	t.Helper()
	client := clientForDevice(server, deviceID)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	csrfToken := accountBootstrapCSRF(t, client, server.URL, "login")
	response, err := client.PostForm(server.URL+"/login/password", url.Values{
		"csrf_token": {csrfToken}, "email": {email},
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
	csrfToken := accountBootstrapCSRF(t, client, server.URL, "register")
	requested := postExplicitCredentialForm(t, client, server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
	requested.Body.Close()
	if requested.StatusCode != http.StatusNoContent {
		t.Fatalf("request registration code = %d, want 204", requested.StatusCode)
	}
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

	response := postExplicitCredentialForm(t, client, server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"弱密码测试"}, "email": {testStudentEmail},
		"code": {code}, "password": {"password123"},
	})
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || !bytes.Contains(body, []byte(`"code":"REGISTRATION_FAILED"`)) || bytes.Contains(body, []byte("<html")) {
		t.Fatalf("weak password registration = %d %s, want bounded registration error", response.StatusCode, body)
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
	response := postExplicitCredentialForm(t, client, server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"覆盖尝试"}, "email": {testStudentEmail},
		"code": {code}, "password": {"another strong 密码 value"},
	})
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusConflict || !bytes.Contains(body, []byte(`"code":"ACCOUNT_ALREADY_REGISTERED"`)) || bytes.Contains(body, []byte("<html")) {
		t.Fatalf("duplicate registration = %d %s, want bounded already-registered error", response.StatusCode, body)
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

	response := postExplicitCredentialForm(t, client, server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"事务回滚"}, "email": {testStudentEmail},
		"code": {code}, "password": {"correct horse 电池 staple"},
	})
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("registration with forced failure = %d, want bounded 400", response.StatusCode)
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
	csrfToken := accountBootstrapCSRF(t, client, serverURL, "register")
	requested := postExplicitCredentialForm(t, client, serverURL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
	})
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
	registered := postExplicitCredentialForm(t, client, serverURL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"测试用户"}, "email": {testStudentEmail},
		"code": {sender.lastMessage().Code}, "password": {"correct horse 电池 staple"},
	})
	registered.Body.Close()
	if registered.StatusCode != http.StatusNoContent {
		t.Fatalf("registration seed = %d, want 204", registered.StatusCode)
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

	csrfToken := accountBootstrapCSRF(t, client, server.URL, "register")

	requested := postExplicitCredentialForm(t, client, server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken},
		"email":      {testStudentEmail},
	})
	requested.Body.Close()
	if requested.StatusCode != http.StatusNoContent {
		t.Fatalf("request registration code = %d, want 204", requested.StatusCode)
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
	if registered.StatusCode != http.StatusSeeOther || registered.Header.Get("Location") != "/account/security" {
		t.Fatalf("registration completion = %d %q, want 303 to account security", registered.StatusCode, registered.Header.Get("Location"))
	}
	account, err := client.Get(server.URL + "/account/security")
	if err != nil {
		t.Fatalf("open account home after registration: %v", err)
	}
	account.Body.Close()
	if account.StatusCode != http.StatusFound || account.Header.Get("Location") != "/account/login?next=%2Faccount%2Fsecurity" {
		t.Fatalf("account security after registration = %d %q, want Portal-owned account redirect", account.StatusCode, account.Header.Get("Location"))
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

func TestRegistrationPreservesOnlyValidatedOAuthReturnTarget(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "oauth-registration-device")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	returnTo := "/api/v1/oauth/authorize?response_type=code&client_id=portal-gateway&redirect_uri=https%3A%2F%2Fsuperhuazai.me%2Fapi%2Fv1%2Fauth%2Fcallback&state=state-value&code_challenge=challenge-value&code_challenge_method=S256"

	csrfToken := accountBootstrapCSRF(t, client, server.URL, "register")
	requested := postExplicitCredentialForm(t, client, server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "return_to": {returnTo},
	})
	requested.Body.Close()
	sender := &captureSender{messageID: "provider_oauth_registration"}
	worker, err := mailworker.New(store.New(pool), sender, "worker_oauth_registration", testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("create OAuth registration worker: %v", err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver OAuth registration code: outcome=%+v err=%v", outcome, err)
	}
	registered, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"OAuth 新生"}, "email": {testStudentEmail},
		"code": {sender.lastMessage().Code}, "password": {"oauth registration password"}, "return_to": {returnTo},
	})
	if err != nil {
		t.Fatalf("complete OAuth registration: %v", err)
	}
	registered.Body.Close()
	if registered.StatusCode != http.StatusSeeOther || registered.Header.Get("Location") != returnTo {
		t.Fatalf("OAuth registration = %d %q, want 303 to validated authorize target", registered.StatusCode, registered.Header.Get("Location"))
	}

	unsafe, err := client.Get(server.URL + "/register?return_to=" + url.QueryEscape("https://evil.example/steal"))
	if err != nil {
		t.Fatalf("open registration with external return target: %v", err)
	}
	unsafe.Body.Close()
	if unsafe.StatusCode != http.StatusFound || unsafe.Header.Get("Location") != "/account/login" || strings.Contains(unsafe.Header.Get("Location"), "evil.example") {
		t.Fatalf("external registration return target = %d %q, want bounded Portal redirect", unsafe.StatusCode, unsafe.Header.Get("Location"))
	}
}

func TestLegacyPublicPrefixCannotRestoreCredentialPages(t *testing.T) {
	t.Setenv("PLATFORM_CORE_PUBLIC_PATH_PREFIX", "/account-auth")
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)

	client := server.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	for path, target := range map[string]string{"/register": "/account/login", "/recover": "/account/recover"} {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusFound || response.Header.Get("Location") != target {
			t.Fatalf("%s = %d %q, want Portal redirect %q", path, response.StatusCode, response.Header.Get("Location"), target)
		}
	}

	security, err := client.Get(server.URL + "/account/security")
	if err != nil {
		t.Fatalf("open unauthenticated security page: %v", err)
	}
	security.Body.Close()
	if security.StatusCode != http.StatusFound ||
		security.Header.Get("Location") != "/account/login?next=%2Faccount%2Fsecurity" {
		t.Fatalf("security redirect = %d %q, want Portal-owned account entry", security.StatusCode, security.Header.Get("Location"))
	}
}

func TestAccountCenterRegistrationAndLogoutUseDistinctLocalHTTPCookies(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey, StudentEmailDomains: []string{"henu.edu.cn"},
		LocalCoreCookieName: "henukit_test_core_local",
	})
	if err != nil {
		t.Fatalf("create local HTTP Platform Core: %v", err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, csrfToken, code := prepareRegistrationCode(t, ctx, pool, server, "local-http-registration-device")

	registered, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"本地开发"}, "email": {testStudentEmail},
		"code": {code}, "password": {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("submit local HTTP registration: %v", err)
	}
	registered.Body.Close()
	if registered.StatusCode != http.StatusSeeOther {
		t.Fatalf("local HTTP registration = %d, want 303", registered.StatusCode)
	}
	var issuedLocalSession bool
	for _, cookie := range registered.Cookies() {
		if strings.HasPrefix(cookie.Name, "__Host-") || cookie.Secure {
			t.Fatalf("local HTTP registration issued production cookie: %+v", cookie)
		}
		issuedLocalSession = issuedLocalSession || cookie.Name == "henukit_test_core_local" && cookie.Value != ""
	}
	if !issuedLocalSession {
		t.Fatal("local HTTP registration did not issue the configured local Core Session cookie")
	}

	logoutRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/sessions/revoke", strings.NewReader(`{"all_sessions":false}`))
	logoutRequest.Header.Set("Content-Type", "application/json")
	logoutRequest.Header.Set("Origin", server.URL)
	logoutResponse, err := client.Do(logoutRequest)
	if err != nil {
		t.Fatalf("logout local HTTP session: %v", err)
	}
	logoutResponse.Body.Close()
	if logoutResponse.StatusCode != http.StatusOK {
		t.Fatalf("local HTTP logout = %d, want 200", logoutResponse.StatusCode)
	}
	var clearedLocalSession bool
	for _, cookie := range logoutResponse.Cookies() {
		clearedLocalSession = clearedLocalSession ||
			cookie.Name == "henukit_test_core_local" && cookie.MaxAge < 0 && !cookie.Secure
	}
	if !clearedLocalSession {
		t.Fatal("local HTTP logout did not clear the same configured local Core Session cookie")
	}
}

func TestCookieTransportTrustsForwardedHTTPSOnlyFromConfiguredProxy(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	for _, test := range []struct {
		name           string
		trustedProxies []string
		wantSecure     bool
		wantCSRFName   string
	}{
		{name: "configured proxy", trustedProxies: []string{"127.0.0.0/8"}, wantSecure: true, wantCSRFName: "__Host-henukit_login_csrf"},
		{name: "untrusted peer", wantSecure: false, wantCSRFName: "henukit_proxy_test_local_csrf"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, err := platformcore.New(platformcore.Config{
				Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
				VerificationEncryptionKey: testVerificationEncryptionKey, StudentEmailDomains: []string{"henu.edu.cn"},
				LocalCoreCookieName: "henukit_proxy_test_local", TrustedProxyCIDRs: test.trustedProxies,
			})
			if err != nil {
				t.Fatalf("create proxy-cookie Platform Core: %v", err)
			}
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/account/bootstrap?flow=login", nil)
			request.Header.Set("X-Forwarded-Proto", "https")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatalf("open proxy-cookie login: %v", err)
			}
			response.Body.Close()
			var foundCSRF bool
			for _, cookie := range response.Cookies() {
				if cookie.Name == test.wantCSRFName {
					foundCSRF = true
					if cookie.Secure != test.wantSecure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" {
						t.Fatalf("unexpected proxy-cookie attributes: %+v", cookie)
					}
				}
			}
			if !foundCSRF {
				t.Fatalf("login omitted expected CSRF cookie %q: %+v", test.wantCSRFName, response.Cookies())
			}
		})
	}
}

func TestAccountCenterPasswordLoginCreatesCoreSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)

	registrationClient := clientForDevice(server, "password-seed-device")
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, registrationClient)

	loginClient := clientForDevice(server, "password-login-device")
	loginClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	loginCSRF := accountBootstrapCSRF(t, loginClient, server.URL, "login")
	loggedIn, err := loginClient.PostForm(server.URL+"/login/password", url.Values{
		"csrf_token": {loginCSRF}, "email": {testStudentEmail},
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
	if sessions != 1 {
		t.Fatalf("active Core Sessions = %d, want password login session", sessions)
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
	csrfToken := accountBootstrapCSRF(t, loginClient, server.URL, "login")
	response, err := loginClient.PostForm(server.URL+"/login/password", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
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
			if response.StatusCode != http.StatusUnauthorized || !bytes.Contains(body, []byte(`"code":"AUTHENTICATION_FAILED"`)) || bytes.Contains(body, []byte("<html")) {
				t.Fatalf("failed password login = %d %s, want bounded generic failure", response.StatusCode, body)
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
	if response.StatusCode != http.StatusServiceUnavailable || !bytes.Contains(body, []byte(`"code":"DEPENDENCY_UNAVAILABLE"`)) || bytes.Contains(body, []byte("<html")) {
		t.Fatalf("Redis-failed password login = %d %s, want bounded fail-closed response", response.StatusCode, body)
	}
	var sessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions after Redis failure: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("Redis-failed password login created %d sessions", sessions)
	}
}
