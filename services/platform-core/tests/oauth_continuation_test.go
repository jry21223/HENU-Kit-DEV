package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/mailworker"
	"henukit.dev/platform-core/internal/store"
)

const (
	portalContinuationClientID     = "portal-gateway"
	portalContinuationRedirectURI  = "https://portal.henukit.test/api/v1/auth/callback"
	portalContinuationClientSecret = "portal-continuation-secret-with-enough-entropy"
	portalContinuationVerifier     = "portal-continuation-verifier-at-least-forty-three-characters"
)

func TestPortalOAuthContinuationRestoresValidatedAuthorizeRequest(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	var logs bytes.Buffer
	server := newVerificationServerWithLogger(t, pool, redisClient, slog.New(slog.NewJSONHandler(&logs, nil)))
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "portal-continuation-registration-seed"))
	seedPortalContinuationClient(t, ctx, pool)

	client := clientForDevice(server, "portal-continuation-browser")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	state := base64.RawURLEncoding.EncodeToString([]byte("portal-continuation-state-value-01"))
	authorize, err := client.Get(portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "code", state))
	if err != nil {
		t.Fatalf("start signed-out Portal authorize: %v", err)
	}
	authorize.Body.Close()
	if authorize.StatusCode != http.StatusFound {
		t.Fatalf("signed-out Portal authorize = %d, want 302", authorize.StatusCode)
	}
	accountCenter, err := url.Parse(authorize.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse Account Center redirect: %v", err)
	}
	if accountCenter.Path != "/account/login" {
		t.Fatalf("signed-out Portal authorize path = %q, want Portal Account Center", accountCenter.Path)
	}
	continuation := accountCenter.Query().Get("continuation")
	if decoded, decodeErr := base64.RawURLEncoding.DecodeString(continuation); decodeErr != nil || len(decoded) != 32 {
		t.Fatalf("continuation handle is not a 32-byte opaque value: len=%d err=%v", len(decoded), decodeErr)
	}
	if len(accountCenter.Query()) != 1 {
		t.Fatalf("Account Center URL exposed OAuth parameters: %s", accountCenter.RawQuery)
	}
	var secureBindingCookie bool
	for _, cookie := range authorize.Cookies() {
		secureBindingCookie = secureBindingCookie || cookie.Name == "__Host-henukit_device" && cookie.Value != "" && cookie.HttpOnly && cookie.Secure && cookie.SameSite == http.SameSiteLaxMode && cookie.Path == "/" && cookie.Domain == ""
	}
	if !secureBindingCookie {
		t.Fatalf("continuation did not issue a secure browser binding cookie: %+v", authorize.Cookies())
	}
	var boundDevice bool
	for _, cookie := range client.Jar.Cookies(mustURL(t, server.URL)) {
		boundDevice = boundDevice || cookie.Name == "__Host-henukit_device" && cookie.Value != ""
	}
	if !boundDevice {
		t.Fatal("continuation did not bind the browser device cookie")
	}
	keys, err := redisClient.Keys(ctx, "platform-core:oauth-continuation:*").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("continuation coordination keys = %v err=%v, want one", keys, err)
	}
	ttl, err := redisClient.TTL(ctx, keys[0]).Result()
	if err != nil || ttl <= 0 || ttl > 30*time.Minute || strings.Contains(keys[0], continuation) {
		t.Fatalf("continuation coordination TTL/key = %s/%q err=%v", ttl, keys[0], err)
	}

	bootstrap := requestContinuationBootstrap(t, client, server.URL, continuation)
	bootstrapBody, _ := io.ReadAll(bootstrap.Body)
	bootstrap.Body.Close()
	var bootstrapEnvelope struct {
		Data struct {
			Flow         string `json:"flow"`
			CSRFToken    string `json:"csrf_token"`
			Continuation struct {
				Available   bool   `json:"available"`
				ProductName string `json:"product_name"`
			} `json:"continuation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bootstrapBody, &bootstrapEnvelope); err != nil {
		t.Fatalf("decode continuation Bootstrap: %v body=%s", err, bootstrapBody)
	}
	if bootstrap.StatusCode != http.StatusOK || bootstrapEnvelope.Data.Flow != "login" || len(bootstrapEnvelope.Data.CSRFToken) < 32 || !bootstrapEnvelope.Data.Continuation.Available || bootstrapEnvelope.Data.Continuation.ProductName != "HENU Kit" {
		t.Fatalf("continuation Bootstrap = %d %+v, want trusted Portal destination", bootstrap.StatusCode, bootstrapEnvelope)
	}
	for _, forbidden := range []string{state, portalContinuationRedirectURI, portalContinuationChallenge(), continuation} {
		if strings.Contains(string(bootstrapBody), forbidden) {
			t.Fatalf("continuation Bootstrap exposed a protected value: %s", bootstrapBody)
		}
	}

	replacement := "A"
	if strings.HasSuffix(continuation, replacement) {
		replacement = "B"
	}
	tampered := continuation[:len(continuation)-1] + replacement
	tamperedBootstrap := requestContinuationBootstrap(t, client, server.URL, tampered)
	tamperedBootstrap.Body.Close()
	if tamperedBootstrap.StatusCode != http.StatusGone {
		t.Fatalf("tampered continuation Bootstrap = %d, want 410", tamperedBootstrap.StatusCode)
	}
	otherBrowser := clientForDevice(server, "portal-continuation-other-browser")
	boundBootstrap := requestContinuationBootstrap(t, otherBrowser, server.URL, continuation)
	boundBootstrap.Body.Close()
	if boundBootstrap.StatusCode != http.StatusGone {
		t.Fatalf("cross-browser continuation Bootstrap = %d, want 410", boundBootstrap.StatusCode)
	}

	login := postExplicitCredentialForm(t, client, server.URL+"/login/password", url.Values{
		"csrf_token": {bootstrapEnvelope.Data.CSRFToken}, "email": {testStudentEmail},
		"password": {"correct horse 电池 staple"}, "return_to": {"/"},
	})
	login.Body.Close()
	if login.StatusCode != http.StatusNoContent {
		t.Fatalf("continuation password login = %d, want 204", login.StatusCode)
	}

	csrfAttackClient := cloneBrowserClient(t, server, client)
	csrfRejected := postContinuationResume(t, csrfAttackClient, server.URL, continuation, "wrong-csrf-token-with-at-least-thirty-two-characters")
	csrfRejected.Body.Close()
	assertContinuationRecovery(t, csrfRejected, "expired")
	replayClient := cloneBrowserClient(t, server, client)
	resume := postContinuationResume(t, client, server.URL, continuation, bootstrapEnvelope.Data.CSRFToken)
	resume.Body.Close()
	if resume.StatusCode != http.StatusSeeOther {
		t.Fatalf("continuation resume = %d, want 303", resume.StatusCode)
	}
	callback, err := url.Parse(resume.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse resumed callback: %v", err)
	}
	if callback.Scheme+"://"+callback.Host+callback.Path != portalContinuationRedirectURI || callback.Query().Get("state") != state || callback.Query().Get("code") == "" {
		t.Fatalf("resumed callback = %q, want exact Portal callback with code and original state", callback.String())
	}
	exchangeRequest := signedExchangeRequest(t, server, callback.Query().Get("code"), portalContinuationVerifier, "portal_continuation_exchange", "portal_continuation_nonce", "", "", exchangeClientCreds{
		clientID: portalContinuationClientID, clientSecret: portalContinuationClientSecret,
		redirectURI: portalContinuationRedirectURI, keyID: "primary",
	})
	exchange, err := server.Client().Do(exchangeRequest)
	if err != nil {
		t.Fatalf("exchange resumed Portal authorization code: %v", err)
	}
	defer exchange.Body.Close()
	var exchangeEnvelope struct {
		Data struct {
			SessionToken string `json:"session_exchange_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(exchange.Body).Decode(&exchangeEnvelope); err != nil {
		t.Fatalf("decode resumed Portal exchange: %v", err)
	}
	if exchange.StatusCode != http.StatusOK || len(exchangeEnvelope.Data.SessionToken) < 32 {
		t.Fatalf("resumed Portal exchange = %d %+v, want product Session fact", exchange.StatusCode, exchangeEnvelope)
	}

	replay := postContinuationResume(t, replayClient, server.URL, continuation, bootstrapEnvelope.Data.CSRFToken)
	replay.Body.Close()
	if replay.StatusCode != http.StatusSeeOther {
		t.Fatalf("replayed continuation = %d, want 303 to safe recovery", replay.StatusCode)
	}
	replayLocation, err := url.Parse(replay.Header.Get("Location"))
	if err != nil || replayLocation.Path != "/account/login" || replayLocation.Query().Get("continuation_error") != "unavailable" || !strings.HasPrefix(replayLocation.Query().Get("request_id"), "req_") || strings.Contains(replay.Header.Get("Location"), continuation) {
		t.Fatalf("replayed continuation recovery = %q err=%v", replay.Header.Get("Location"), err)
	}
	logged := logs.String()
	for _, secret := range []string{continuation, state, portalContinuationChallenge(), callback.Query().Get("code"), "correct horse 电池 staple"} {
		if strings.Contains(logged, secret) {
			t.Fatalf("continuation audit log exposed a protected value: %s", logged)
		}
	}
	if !strings.Contains(logged, `"service_id":"portal-gateway"`) {
		t.Fatalf("continuation audit log omitted trusted client classification: %s", logged)
	}
}

func TestPortalContinuationRejectsInvalidAuthorizeBeforeAccountCenter(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedPortalContinuationClient(t, ctx, pool)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "portal-continuation-invalid-authorize")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	for name, authorizeURL := range map[string]string{
		"callback mismatch": portalContinuationAuthorizeURL(server.URL, "https://evil.example/callback", "code", "state_callback_mismatch"),
		"response type":     portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "token", "state_response_type"),
		"oversized state":   portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "code", strings.Repeat("s", 201)),
		"unknown client": strings.Replace(
			portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "code", "state_unknown_client"),
			"client_id="+url.QueryEscape(portalContinuationClientID), "client_id=unknown-client", 1,
		),
	} {
		t.Run(name, func(t *testing.T) {
			response, err := client.Get(authorizeURL)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest || response.Header.Get("Location") != "" {
				t.Fatalf("invalid authorize = %d location=%q, want non-redirecting 400", response.StatusCode, response.Header.Get("Location"))
			}
		})
	}
}

func TestPortalContinuationCreationFailsClosedWhenRedisIsUnavailable(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedPortalContinuationClient(t, ctx, pool)
	unavailableRedis := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond,
		ReadTimeout: 50 * time.Millisecond, WriteTimeout: 50 * time.Millisecond, MaxRetries: 0,
	})
	t.Cleanup(func() { _ = unavailableRedis.Close() })
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: unavailableRedis, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey,
	})
	if err != nil {
		t.Fatalf("create Platform Core with unavailable Redis: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client := clientForDevice(server, "portal-continuation-redis-unavailable")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	response, err := client.Get(portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "code", "state_redis_unavailable"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	assertContinuationRecovery(t, response, "service")
}

func TestPortalContinuationCreationIsRateLimited(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedPortalContinuationClient(t, ctx, pool)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "portal-continuation-rate-limit")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	for attempt := 1; attempt <= 10; attempt++ {
		response, err := client.Get(portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "code", "state_rate_limit_"+string(rune('A'+attempt))))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusFound {
			t.Fatalf("continuation creation %d = %d, want 302", attempt, response.StatusCode)
		}
	}
	limited, err := client.Get(portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "code", "state_rate_limit_final"))
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Body.Close()
	assertContinuationRecovery(t, limited, "service")
}

func TestExpiredPortalContinuationFailsClosedThroughBootstrapAndResume(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "portal-expired-registration-seed"))
	seedPortalContinuationClient(t, ctx, pool)
	client := clientForDevice(server, "portal-expired-continuation-browser")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	continuation := startPortalContinuation(t, client, server.URL, "state_expired_continuation")
	csrfToken := continuationCSRF(t, client, server.URL, "login", continuation)
	login := postExplicitCredentialForm(t, client, server.URL+"/login/password", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
		"password": {"correct horse 电池 staple"}, "return_to": {"/"},
	})
	login.Body.Close()
	if login.StatusCode != http.StatusNoContent {
		t.Fatalf("expired continuation login = %d, want 204", login.StatusCode)
	}
	keys, err := redisClient.Keys(ctx, "platform-core:oauth-continuation:*:*").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("find expiring continuation key = %v err=%v", keys, err)
	}
	if err := redisClient.PExpire(ctx, keys[0], time.Millisecond).Err(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	bootstrap := requestContinuationBootstrap(t, client, server.URL, continuation)
	bootstrap.Body.Close()
	if bootstrap.StatusCode != http.StatusGone {
		t.Fatalf("expired continuation Bootstrap = %d, want 410", bootstrap.StatusCode)
	}
	resume := postContinuationResume(t, client, server.URL, continuation, csrfToken)
	resume.Body.Close()
	assertContinuationRecovery(t, resume, "expired")
}

func TestPortalContinuationResumeDependencyFailureUsesPortalRecovery(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "portal-resume-dependency-registration-seed"))
	seedPortalContinuationClient(t, ctx, pool)
	client := clientForDevice(server, "portal-resume-dependency-browser")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	continuation := startPortalContinuation(t, client, server.URL, "state_resume_dependency")
	csrfToken := continuationCSRF(t, client, server.URL, "login", continuation)
	login := postExplicitCredentialForm(t, client, server.URL+"/login/password", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
		"password": {"correct horse 电池 staple"}, "return_to": {"/"},
	})
	login.Body.Close()
	if login.StatusCode != http.StatusNoContent {
		t.Fatalf("resume dependency login = %d, want 204", login.StatusCode)
	}
	if err := redisClient.Close(); err != nil {
		t.Fatal(err)
	}
	resume := postContinuationResume(t, client, server.URL, continuation, csrfToken)
	resume.Body.Close()
	assertContinuationRecovery(t, resume, "service")
}

func TestPortalContinuationResumesAfterEmailCodeLogin(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "portal-code-continuation-registration-seed"))
	seedPortalContinuationClient(t, ctx, pool)
	client := clientForDevice(server, "portal-code-continuation-browser")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	state := "state_email_code_continuation"
	continuation := startPortalContinuation(t, client, server.URL, state)
	csrfToken := continuationCSRF(t, client, server.URL, "login", continuation)

	requested := postExplicitCredentialForm(t, client, server.URL+"/login/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "return_to": {"/"},
	})
	requested.Body.Close()
	if requested.StatusCode != http.StatusNoContent {
		t.Fatalf("continuation login-code request = %d, want 204", requested.StatusCode)
	}
	code := deliverContinuationCode(t, ctx, pool, "provider_portal_code_continuation")
	verified := postExplicitCredentialForm(t, client, server.URL+"/login/verify", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "code": {code}, "return_to": {"/"},
	})
	verified.Body.Close()
	if verified.StatusCode != http.StatusNoContent {
		t.Fatalf("continuation email-code login = %d, want 204", verified.StatusCode)
	}
	resume := postContinuationResume(t, client, server.URL, continuation, csrfToken)
	resume.Body.Close()
	assertResumedPortalCallback(t, resume, state)
}

func TestPortalContinuationResumesAfterRegistration(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedPortalContinuationClient(t, ctx, pool)
	client := clientForDevice(server, "portal-registration-continuation-browser")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	state := "state_registration_continuation"
	continuation := startPortalContinuation(t, client, server.URL, state)
	csrfToken := continuationCSRF(t, client, server.URL, "register", continuation)

	requested := postExplicitCredentialForm(t, client, server.URL+"/register/code", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail}, "return_to": {"/"},
	})
	requested.Body.Close()
	if requested.StatusCode != http.StatusNoContent {
		t.Fatalf("continuation registration-code request = %d, want 204", requested.StatusCode)
	}
	code := deliverContinuationCode(t, ctx, pool, "provider_portal_registration_continuation")
	registered := postExplicitCredentialForm(t, client, server.URL+"/register", url.Values{
		"csrf_token": {csrfToken}, "display_name": {"Continuation 测试用户"},
		"email": {testStudentEmail}, "code": {code}, "password": {"correct horse 电池 staple"}, "return_to": {"/"},
	})
	registered.Body.Close()
	if registered.StatusCode != http.StatusNoContent {
		t.Fatalf("continuation registration = %d, want 204", registered.StatusCode)
	}
	resume := postContinuationResume(t, client, server.URL, continuation, csrfToken)
	resume.Body.Close()
	assertResumedPortalCallback(t, resume, state)
}

func TestPortalContinuationConcurrentResumeHasOneWinner(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	seedRegisteredAccount(t, ctx, pool, redisClient, server.URL, clientForDevice(server, "portal-concurrent-registration-seed"))
	seedPortalContinuationClient(t, ctx, pool)
	client := clientForDevice(server, "portal-concurrent-continuation-browser")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	continuation := startPortalContinuation(t, client, server.URL, "state_concurrent_continuation")
	csrfToken := continuationCSRF(t, client, server.URL, "login", continuation)
	login := postExplicitCredentialForm(t, client, server.URL+"/login/password", url.Values{
		"csrf_token": {csrfToken}, "email": {testStudentEmail},
		"password": {"correct horse 电池 staple"}, "return_to": {"/"},
	})
	login.Body.Close()
	if login.StatusCode != http.StatusNoContent {
		t.Fatalf("concurrent continuation login = %d, want 204", login.StatusCode)
	}

	type resumeResult struct {
		response *http.Response
		err      error
	}
	results := make(chan resumeResult, 2)
	for range 2 {
		browser := cloneBrowserClient(t, server, client)
		request, err := newContinuationResumeRequest(server.URL, continuation, csrfToken)
		if err != nil {
			t.Fatal(err)
		}
		go func() {
			response, err := browser.Do(request)
			results <- resumeResult{response: response, err: err}
		}()
	}
	var callbacks, recoveries int
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent continuation resume: %v", result.err)
		}
		result.response.Body.Close()
		location, err := url.Parse(result.response.Header.Get("Location"))
		if err != nil || result.response.StatusCode != http.StatusSeeOther {
			t.Fatalf("concurrent continuation response = %d %q err=%v", result.response.StatusCode, result.response.Header.Get("Location"), err)
		}
		if location.Scheme+"://"+location.Host+location.Path == portalContinuationRedirectURI && location.Query().Get("code") != "" {
			callbacks++
		} else if location.Path == "/account/login" && location.Query().Get("continuation_error") == "unavailable" {
			recoveries++
		}
	}
	if callbacks != 1 || recoveries != 1 {
		t.Fatalf("concurrent continuation outcomes callbacks=%d recoveries=%d, want 1/1", callbacks, recoveries)
	}
}

func TestPortalAuthorizeWithCoreSessionBypassesContinuation(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	seedPortalContinuationClient(t, ctx, pool)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "portal-existing-core-session")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	request, err := http.NewRequest(http.MethodGet, portalContinuationAuthorizeURL(server.URL, portalContinuationRedirectURI, "code", "state_existing_core_session"), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_core_session", Value: testCoreToken})
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || response.StatusCode != http.StatusFound || location.Scheme+"://"+location.Host+location.Path != portalContinuationRedirectURI || location.Query().Get("code") == "" || location.Query().Get("state") != "state_existing_core_session" || location.Query().Get("continuation") != "" {
		t.Fatalf("existing Core Session authorize = %d %q err=%v", response.StatusCode, response.Header.Get("Location"), err)
	}
}

func seedPortalContinuationClient(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	secretHash := sha256.Sum256([]byte(portalContinuationClientSecret))
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ($1, $2)`, portalContinuationClientID, []string{portalContinuationRedirectURI}); err != nil {
		t.Fatalf("seed Portal OAuth client: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_client_keys (client_id, key_id, secret_hash, status) VALUES ($1, 'primary', $2, 'active')`, portalContinuationClientID, secretHash[:]); err != nil {
		t.Fatalf("seed Portal OAuth client key: %v", err)
	}
}

func portalContinuationChallenge() string {
	digest := sha256.Sum256([]byte(portalContinuationVerifier))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func portalContinuationAuthorizeURL(serverURL, redirectURI, responseType, state string) string {
	return serverURL + "/api/v1/oauth/authorize?" + url.Values{
		"response_type": {responseType}, "client_id": {portalContinuationClientID},
		"redirect_uri": {redirectURI}, "state": {state},
		"code_challenge": {portalContinuationChallenge()}, "code_challenge_method": {"S256"},
	}.Encode()
}

func startPortalContinuation(t *testing.T, client *http.Client, serverURL, state string) string {
	t.Helper()
	response, err := client.Get(portalContinuationAuthorizeURL(serverURL, portalContinuationRedirectURI, "code", state))
	if err != nil {
		t.Fatalf("start Portal continuation: %v", err)
	}
	defer response.Body.Close()
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || response.StatusCode != http.StatusFound || location.Path != "/account/login" || location.Query().Get("continuation") == "" {
		t.Fatalf("Portal continuation start = %d location=%q err=%v", response.StatusCode, response.Header.Get("Location"), err)
	}
	return location.Query().Get("continuation")
}

func continuationCSRF(t *testing.T, client *http.Client, serverURL, flow, continuation string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, serverURL+"/account/bootstrap?"+url.Values{
		"flow": {flow}, "continuation": {continuation},
	}.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil || response.StatusCode != http.StatusOK || len(envelope.Data.CSRFToken) < 32 {
		t.Fatalf("%s continuation Bootstrap = %d %+v err=%v", flow, response.StatusCode, envelope, err)
	}
	return envelope.Data.CSRFToken
}

func deliverContinuationCode(t *testing.T, ctx context.Context, pool *pgxpool.Pool, messageID string) string {
	t.Helper()
	sender := &captureSender{messageID: messageID}
	worker, err := mailworker.New(store.New(pool), sender, "worker_"+messageID, testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver continuation code: outcome=%+v err=%v", outcome, err)
	}
	return sender.lastMessage().Code
}

func assertResumedPortalCallback(t *testing.T, response *http.Response, state string) {
	t.Helper()
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || response.StatusCode != http.StatusSeeOther || location.Scheme+"://"+location.Host+location.Path != portalContinuationRedirectURI || location.Query().Get("state") != state || location.Query().Get("code") == "" {
		t.Fatalf("resumed Portal callback = %d %q err=%v", response.StatusCode, response.Header.Get("Location"), err)
	}
}

func assertContinuationRecovery(t *testing.T, response *http.Response, category string) {
	t.Helper()
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || response.StatusCode != http.StatusSeeOther || location.Path != "/account/login" || location.Query().Get("continuation_error") != category || !strings.HasPrefix(location.Query().Get("request_id"), "req_") {
		t.Fatalf("continuation recovery = %d %q err=%v, want Portal %s recovery", response.StatusCode, response.Header.Get("Location"), err, category)
	}
}

func requestContinuationBootstrap(t *testing.T, client *http.Client, serverURL, continuation string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, serverURL+"/account/bootstrap?"+url.Values{
		"flow": {"login"}, "continuation": {continuation},
	}.Encode(), nil)
	if err != nil {
		t.Fatalf("create continuation Bootstrap: %v", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("request continuation Bootstrap: %v", err)
	}
	return response
}

func cloneBrowserClient(t *testing.T, server *httptest.Server, source *http.Client) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	serverURL := mustURL(t, server.URL)
	jar.SetCookies(serverURL, source.Jar.Cookies(serverURL))
	client := &http.Client{Transport: server.Client().Transport, Jar: jar}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

func postContinuationResume(t *testing.T, client *http.Client, serverURL, continuation, csrfToken string) *http.Response {
	t.Helper()
	request, err := newContinuationResumeRequest(serverURL, continuation, csrfToken)
	if err != nil {
		t.Fatalf("create continuation resume: %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("resume continuation: %v", err)
	}
	return response
}

func newContinuationResumeRequest(serverURL, continuation, csrfToken string) (*http.Request, error) {
	request, err := http.NewRequest(http.MethodPost, serverURL+"/account/continuation/resume", strings.NewReader(url.Values{
		"continuation": {continuation}, "csrf_token": {csrfToken},
	}.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request, nil
}
