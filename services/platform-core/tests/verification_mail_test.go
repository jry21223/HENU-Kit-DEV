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
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/contract"
	"henukit.dev/platform-core/internal/mailworker"
	"henukit.dev/platform-core/internal/operatorbootstrap"
	"henukit.dev/platform-core/internal/store"
	"henukit.dev/platform-core/internal/verificationmail"
)

const testStudentEmail = "student@henu.edu.cn"
const testDeliveryToken = "test-mail-delivery-token-32-characters"
const testRetiringDeliveryToken = "test-retiring-delivery-token-32-characters"

var testDeviceClients sync.Map

func TestVerificationCodeAndOutboxLifecycle(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	var logs bytes.Buffer
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, Logger: slog.New(slog.NewJSONHandler(&logs, nil)),
		IdempotencyEncryptionKey:  testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey,
		StudentEmailDomains:       []string{"henu.edu.cn"},
		MailDeliveryWebhookToken:  testDeliveryToken,
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	response := requestVerificationCode(t, server, "request_verification_001")
	responseBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("request verification = %d %s, want 202", response.StatusCode, responseBody)
	}
	if bytes.Contains(responseBody, []byte(testStudentEmail)) {
		t.Fatal("verification response exposed the email")
	}
	var codeHash, nonce, recipientCiphertext, payloadCiphertext []byte
	var verificationID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		SELECT v.id, v.code_hash, v.code_nonce, o.recipient_ciphertext, o.payload_ciphertext
		FROM verification_codes v JOIN mail_outbox o ON o.verification_code_id = v.id`).Scan(
		&verificationID, &codeHash, &nonce, &recipientCiphertext, &payloadCiphertext,
	); err != nil {
		t.Fatalf("read verification facts: %v", err)
	}
	if len(codeHash) != 32 || len(nonce) != 16 || bytes.Contains(recipientCiphertext, []byte(testStudentEmail)) || bytes.Contains(payloadCiphertext, []byte(testStudentEmail)) {
		t.Fatal("verification facts were not hashed/encrypted as required")
	}

	sender := &captureSender{messageID: "provider_message_001"}
	worker, err := mailworker.New(store.New(pool), sender, "worker_lifecycle", testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("create mail worker: %v", err)
	}
	outcome, err := worker.ProcessOne(ctx)
	if err != nil || !outcome.Processed {
		t.Fatalf("process verification mail: outcome=%+v err=%v", outcome, err)
	}
	message := sender.lastMessage()
	if message.Recipient != testStudentEmail || len(message.Code) != 6 {
		t.Fatalf("unexpected decrypted mail job: recipient=%q code_length=%d", message.Recipient, len(message.Code))
	}
	if strings.Contains(logs.String(), testStudentEmail) || strings.Contains(logs.String(), message.Code) {
		t.Fatal("HTTP logs exposed email or verification code")
	}
	if bytes.Contains(responseBody, []byte(message.Code)) || bytes.Contains(payloadCiphertext, []byte(message.Code)) {
		t.Fatal("verification response or durable Outbox exposed the plaintext code")
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM mail_outbox WHERE verification_code_id = $1`, verificationID).Scan(&status); err != nil {
		t.Fatalf("read accepted outbox: %v", err)
	}
	if status != "accepted" {
		t.Fatalf("provider acceptance status = %s, want accepted", status)
	}

	const attempts = 20
	var successes atomic.Int32
	winningKey := make(chan string, attempts)
	var wait sync.WaitGroup
	for index := range attempts {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			key := fmt.Sprintf("verify_concurrent_%02d", index)
			verifyResponse := verifyCode(t, server, message.Code, key)
			verifyResponse.Body.Close()
			if verifyResponse.StatusCode == http.StatusOK {
				successes.Add(1)
				winningKey <- key
			} else if verifyResponse.StatusCode != http.StatusConflict {
				t.Errorf("concurrent verification %d = %d, want 200 or 409", index, verifyResponse.StatusCode)
			}
		}(index)
	}
	wait.Wait()
	close(winningKey)
	if successes.Load() != 1 {
		t.Fatalf("concurrent verification successes = %d, want 1", successes.Load())
	}
	key := <-winningKey
	for replayIndex := range 40 {
		replay := verifyCode(t, server, message.Code, key)
		replay.Body.Close()
		if replay.StatusCode != http.StatusOK {
			t.Fatalf("idempotent verification replay %d = %d, want 200", replayIndex, replay.StatusCode)
		}
	}
	var issuedSessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id = (SELECT user_id FROM email_identities WHERE email_lookup_hash = (SELECT email_lookup_hash FROM verification_codes WHERE id = $1))`, verificationID).Scan(&issuedSessions); err != nil {
		t.Fatalf("count concurrent login sessions: %v", err)
	}
	if issuedSessions != 1 {
		t.Fatalf("concurrent verification and replay issued %d Core Sessions, want exactly 1", issuedSessions)
	}
	deliveryJSON := []byte(fmt.Sprintf(`{"message_id":%q,"status":"delivered"}`, sender.messageID))
	unauthorizedRequest, _ := http.NewRequest(http.MethodPost, server.URL+contract.RecordMailDeliveryRoute, bytes.NewReader(deliveryJSON))
	unauthorizedRequest.Header.Set("Content-Type", "application/json")
	signDeliveryRequest(unauthorizedRequest, deliveryJSON, "mail-provider-active", "invalid-delivery-token-invalid-delivery-token", "nonce_unauthorized_001")
	unauthorizedResponse, err := server.Client().Do(unauthorizedRequest)
	if err != nil || unauthorizedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized provider delivery: status=%v err=%v, want 401", unauthorizedResponse.StatusCode, err)
	}
	unauthorizedResponse.Body.Close()
	deliveryRequest, _ := http.NewRequest(http.MethodPost, server.URL+contract.RecordMailDeliveryRoute, bytes.NewReader(deliveryJSON))
	deliveryRequest.Header.Set("Content-Type", "application/json")
	signDeliveryRequest(deliveryRequest, deliveryJSON, "mail-provider-active", testDeliveryToken, "nonce_delivery_001")
	deliveryResponse, err := server.Client().Do(deliveryRequest)
	if err != nil {
		t.Fatalf("record provider delivery: %v", err)
	}
	if deliveryResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("record provider delivery: status=%v, want 202", deliveryResponse.StatusCode)
	}
	deliveryResponse.Body.Close()
	if err := pool.QueryRow(ctx, `SELECT status FROM mail_outbox WHERE verification_code_id = $1`, verificationID).Scan(&status); err != nil || status != "delivered" {
		t.Fatalf("final delivery status = %s err=%v, want delivered", status, err)
	}
	var auditActions string
	if err := pool.QueryRow(ctx, `
		SELECT string_agg(action, ',' ORDER BY created_at, id)
		FROM mail_outbox_audit_events
		WHERE outbox_id = (SELECT id FROM mail_outbox WHERE verification_code_id = $1)`, verificationID).Scan(&auditActions); err != nil || auditActions != "claimed,accepted,delivered" {
		t.Fatalf("delivery audit actions = %q err=%v, want claimed,accepted,delivered", auditActions, err)
	}
}

func TestLoginVerificationBootstrapsStableIdentityAndFifteenDayCoreSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)

	login := func(requestKey, verifyKey, messageID string) (string, time.Time) {
		response := requestVerificationCode(t, server, requestKey)
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("request login verification = %d, want 202", response.StatusCode)
		}
		sender := &captureSender{messageID: messageID}
		worker, err := mailworker.New(store.New(pool), sender, "worker_"+messageID, testVerificationEncryptionKey, time.Minute, time.Second)
		if err != nil {
			t.Fatalf("create mail worker: %v", err)
		}
		if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
			t.Fatalf("deliver login code: outcome=%+v err=%v", outcome, err)
		}
		verified := verifyCode(t, server, sender.lastMessage().Code, verifyKey)
		defer verified.Body.Close()
		body, _ := io.ReadAll(verified.Body)
		if verified.StatusCode != http.StatusOK {
			t.Fatalf("verify login code = %d %s, want 200", verified.StatusCode, body)
		}
		var envelope struct {
			Data struct {
				VerificationID string                `json:"verification_id"`
				User           contract.PlatformUser `json:"user"`
				SessionExpires time.Time             `json:"session_expires_at"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatalf("decode login response: %v", err)
		}
		if envelope.Data.VerificationID == "" || envelope.Data.User.UserID == "" || !envelope.Data.User.EmailVerified || envelope.Data.User.Status != "active" {
			t.Fatalf("incomplete login bootstrap response: %+v", envelope.Data)
		}
		remaining := time.Until(envelope.Data.SessionExpires)
		if remaining < 14*24*time.Hour+23*time.Hour || remaining > 15*24*time.Hour+time.Minute {
			t.Fatalf("core Session lifetime = %s, want 15 days", remaining)
		}
		accountURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatalf("parse account URL: %v", err)
		}
		cookies := clientForDevice(server, "test-device-001").Jar.Cookies(accountURL)
		var hasCoreSession bool
		for _, cookie := range cookies {
			hasCoreSession = hasCoreSession || cookie.Name == "__Host-henukit_core_session" && cookie.Value != ""
		}
		if !hasCoreSession {
			t.Fatalf("login did not set the Core Session cookie: %+v", cookies)
		}
		return envelope.Data.User.UserID, envelope.Data.SessionExpires
	}

	firstUserID, _ := login("request_login_bootstrap_001", "verify_login_bootstrap_001", "provider_login_bootstrap_001")
	if _, err := pool.Exec(ctx, `UPDATE verification_codes SET revoked_at = now() WHERE used_at IS NULL`); err != nil {
		t.Fatalf("prepare second login: %v", err)
	}
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("reset login rate limits: %v", err)
	}
	secondUserID, _ := login("request_login_bootstrap_002", "verify_login_bootstrap_002", "provider_login_bootstrap_002")
	if firstUserID != secondUserID {
		t.Fatalf("same email created different users: %s != %s", firstUserID, secondUserID)
	}
	var users, identities, sessions int
	var encryptedEmail []byte
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM email_identities), (SELECT count(*) FROM sessions WHERE kind = 'core'), (SELECT email_ciphertext FROM email_identities LIMIT 1)`).Scan(&users, &identities, &sessions, &encryptedEmail); err != nil {
		t.Fatalf("read login identity facts: %v", err)
	}
	if users != 1 || identities != 1 || sessions != 2 || bytes.Contains(encryptedEmail, []byte(testStudentEmail)) {
		t.Fatalf("identity facts users=%d identities=%d sessions=%d plaintext=%v", users, identities, sessions, bytes.Contains(encryptedEmail, []byte(testStudentEmail)))
	}
}

func TestAccountCenterLoginPageCompletesBrowserSession(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	client := clientForDevice(server, "browser-login-device")
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

	returnTo := contract.AuthorizeRoute + "?response_type=code&client_id=quizcraft&redirect_uri=https%3A%2F%2Fquiz.example%2Fauth%2Fcallback&state=12345678&code_challenge=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa&code_challenge_method=S256"
	authorize, err := client.Get(server.URL + returnTo)
	if err != nil {
		t.Fatalf("open authorize entry: %v", err)
	}
	authorize.Body.Close()
	if authorize.StatusCode != http.StatusFound || !strings.HasPrefix(authorize.Header.Get("Location"), "/login?return_to=") {
		t.Fatalf("authorize without Session = %d %q, want account login redirect", authorize.StatusCode, authorize.Header.Get("Location"))
	}
	login, err := client.Get(server.URL + authorize.Header.Get("Location"))
	if err != nil {
		t.Fatalf("open account login: %v", err)
	}
	loginBody, _ := io.ReadAll(login.Body)
	login.Body.Close()
	if login.StatusCode != http.StatusOK || !bytes.Contains(loginBody, []byte("HENU Kit 账号中心")) || !bytes.Contains(loginBody, []byte(`name="email"`)) {
		t.Fatalf("invalid account login page = %d %s", login.StatusCode, loginBody)
	}
	accountURL, _ := url.Parse(server.URL)
	var csrf string
	for _, cookie := range client.Jar.Cookies(accountURL) {
		if cookie.Name == "__Host-henukit_login_csrf" {
			csrf = cookie.Value
		}
	}
	if csrf == "" {
		t.Fatal("account login page did not set a CSRF cookie")
	}
	requestCode, err := client.PostForm(server.URL+"/login/code", url.Values{
		"csrf_token": {csrf}, "return_to": {returnTo}, "email": {testStudentEmail},
	})
	if err != nil {
		t.Fatalf("submit account email: %v", err)
	}
	requestBody, _ := io.ReadAll(requestCode.Body)
	requestCode.Body.Close()
	if requestCode.StatusCode != http.StatusOK || !bytes.Contains(requestBody, []byte(`name="code"`)) {
		t.Fatalf("account code page = %d %s", requestCode.StatusCode, requestBody)
	}
	sender := &captureSender{messageID: "provider_browser_login_001"}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_browser_login", testVerificationEncryptionKey, time.Minute, time.Second)
	if outcome, err := worker.ProcessOne(ctx); err != nil || !outcome.Processed {
		t.Fatalf("deliver browser login code: outcome=%+v err=%v", outcome, err)
	}
	verified, err := client.PostForm(server.URL+"/login/verify", url.Values{
		"csrf_token": {csrf}, "return_to": {returnTo}, "email": {testStudentEmail}, "code": {sender.lastMessage().Code},
	})
	if err != nil {
		t.Fatalf("submit browser login code: %v", err)
	}
	verified.Body.Close()
	if verified.StatusCode != http.StatusSeeOther || verified.Header.Get("Location") != returnTo {
		t.Fatalf("browser login completion = %d %q, want 303 to OAuth authorize", verified.StatusCode, verified.Header.Get("Location"))
	}
	var hasCoreSession bool
	for _, cookie := range client.Jar.Cookies(accountURL) {
		hasCoreSession = hasCoreSession || cookie.Name == "__Host-henukit_core_session" && cookie.Value != ""
	}
	if !hasCoreSession {
		t.Fatal("browser account login did not establish a Core Session")
	}
	revokeBody := bytes.NewBufferString(`{"all_sessions":true}`)
	revokeRequest, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/sessions/revoke", revokeBody)
	revokeRequest.Header.Set("Content-Type", "application/json")
	revokeRequest.Header.Set("Origin", server.URL)
	revoked, err := client.Do(revokeRequest)
	if err != nil {
		t.Fatalf("revoke Core Session: %v", err)
	}
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusOK {
		t.Fatalf("revoke Core Session = %d, want 200", revoked.StatusCode)
	}
	var activeSessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE revoked_at IS NULL`).Scan(&activeSessions); err != nil || activeSessions != 0 {
		t.Fatalf("active sessions after global revocation = %d err=%v, want 0", activeSessions, err)
	}
	afterRevoke, err := client.Get(server.URL + returnTo)
	if err != nil {
		t.Fatalf("authorize after Core Session revocation: %v", err)
	}
	afterRevoke.Body.Close()
	if afterRevoke.StatusCode != http.StatusFound || !strings.HasPrefix(afterRevoke.Header.Get("Location"), "/login?return_to=") {
		t.Fatalf("authorization after revocation = %d %q, want login redirect", afterRevoke.StatusCode, afterRevoke.Header.Get("Location"))
	}
}

func TestInitialOperatorGrantUsesLeastPrivilegeScopesAndAudit(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	requested := requestVerificationCode(t, server, "request_operator_login_001")
	requested.Body.Close()
	sender := &captureSender{messageID: "provider_operator_login_001"}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_operator_login", testVerificationEncryptionKey, time.Minute, time.Second)
	_, _ = worker.ProcessOne(ctx)
	verified := verifyCode(t, server, sender.lastMessage().Code, "verify_operator_login_001")
	verified.Body.Close()
	if verified.StatusCode != http.StatusOK {
		t.Fatalf("operator account login = %d, want 200", verified.StatusCode)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO permission_codes(code, description, status) VALUES
('platform.operations.read', 'Read bounded Platform Operations data within platform Scope', 'active'),
('platform.operations.write', 'Manage Platform Operations state within platform Scope', 'active'),
('quizcraft.workshop.read', 'Read QuizCraft Workshop banks within granted QuizCraft Scope', 'active'),
('quizcraft.workshop.write', 'Create, import, edit, and validate QuizCraft bank versions within granted QuizCraft Scope', 'active'),
('quizcraft.workshop.publish', 'Publish, unpublish, and roll back QuizCraft banks within granted QuizCraft Scope', 'active')
ON CONFLICT (code) DO UPDATE SET description=EXCLUDED.description, status='active'`); err != nil {
		t.Fatalf("seed operator permission catalog: %v", err)
	}
	result, err := operatorbootstrap.Grant(ctx, pool, testVerificationEncryptionKey, operatorbootstrap.Input{Email: testStudentEmail, ActorUnixUser: "root", RequestID: "req_initial_operator_001", Reason: "initial production operator bootstrap"})
	if err != nil || !result.Changed {
		t.Fatalf("initial operator grant = %+v err=%v", result, err)
	}
	var permissions, grants, audits int
	if err := pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM role_permissions rp JOIN authorization_roles r ON r.id=rp.role_id WHERE r.code IN ('platform-operator','quizcraft-workshop-operator')),
  (SELECT count(*) FROM user_role_grants WHERE user_id=$1 AND status='active' AND ((scope_kind='platform' AND product_code IS NULL) OR (scope_kind='product' AND product_code='quizcraft'))),
  (SELECT count(*) FROM operator_bootstrap_audit_events WHERE target_user_id=$1)`, result.UserID).Scan(&permissions, &grants, &audits); err != nil {
		t.Fatalf("read initial operator facts: %v", err)
	}
	if permissions != 5 || grants != 2 || audits != 1 {
		t.Fatalf("initial operator facts permissions=%d grants=%d audits=%d", permissions, grants, audits)
	}
	replay, err := operatorbootstrap.Grant(ctx, pool, testVerificationEncryptionKey, operatorbootstrap.Input{Email: testStudentEmail, ActorUnixUser: "root", RequestID: "req_initial_operator_002", Reason: "confirm initial operator bootstrap"})
	if err != nil || replay.Changed {
		t.Fatalf("idempotent operator grant = %+v err=%v", replay, err)
	}
}

func TestMailWorkerNeverSendsExpiredVerificationPayload(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	response := requestVerificationCode(t, server, "request_expired_payload_001")
	response.Body.Close()

	var outboxID pgtype.UUID
	var recipientCiphertext, payloadCiphertext []byte
	if err := pool.QueryRow(ctx, `SELECT id, recipient_ciphertext, payload_ciphertext FROM mail_outbox`).Scan(&outboxID, &recipientCiphertext, &payloadCiphertext); err != nil {
		t.Fatalf("read queued payload: %v", err)
	}
	codec, err := verificationmail.NewCodec(testVerificationEncryptionKey)
	if err != nil {
		t.Fatalf("create payload codec: %v", err)
	}
	recipient, payload, err := codec.Decode(recipientCiphertext, payloadCiphertext)
	if err != nil {
		t.Fatalf("decode queued payload: %v", err)
	}
	payload.ExpiresAt = time.Now().Add(-time.Second)
	recipientCiphertext, payloadCiphertext, err = codec.Encode(recipient, payload)
	if err != nil {
		t.Fatalf("encode expired payload: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mail_outbox SET recipient_ciphertext = $2, payload_ciphertext = $3 WHERE id = $1`, outboxID, recipientCiphertext, payloadCiphertext); err != nil {
		t.Fatalf("store expired payload: %v", err)
	}

	sender := &captureSender{messageID: "must_not_send"}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_expired_payload", testVerificationEncryptionKey, time.Minute, time.Second)
	outcome, err := worker.ProcessOne(ctx)
	if err != nil || !outcome.Processed || outcome.Result != "failed" || outcome.ErrorCode != "VERIFICATION_EXPIRED" {
		t.Fatalf("expired payload outcome=%+v err=%v", outcome, err)
	}
	if sender.lastMessage().Recipient != "" {
		t.Fatal("worker sent an expired verification payload")
	}
	var status, errorCode string
	var deadLetters int
	if err := pool.QueryRow(ctx, `SELECT status, last_error_code, (SELECT count(*) FROM mail_dead_letters WHERE outbox_id = mail_outbox.id) FROM mail_outbox WHERE id = $1`, outboxID).Scan(&status, &errorCode, &deadLetters); err != nil || status != "failed" || errorCode != "VERIFICATION_EXPIRED" || deadLetters != 1 {
		t.Fatalf("expired payload state=%s/%s dead_letters=%d err=%v", status, errorCode, deadLetters, err)
	}
}

func TestEarlyDeliveryReceiptReconcilesAfterProviderAcceptance(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey, StudentEmailDomains: []string{"henu.edu.cn"},
		MailDeliveryWebhookToken:  testDeliveryToken,
		MailDeliveryRetiringKeyID: "mail-provider-retiring", MailDeliveryRetiringToken: testRetiringDeliveryToken,
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	response := requestVerificationCode(t, server, "request_early_receipt_001")
	response.Body.Close()

	messageID := "provider_early_receipt_001"
	body := []byte(fmt.Sprintf(`{"message_id":%q,"status":"delivered"}`, messageID))
	receiptRequest, _ := http.NewRequest(http.MethodPost, server.URL+contract.RecordMailDeliveryRoute, bytes.NewReader(body))
	receiptRequest.Header.Set("Content-Type", "application/json")
	signDeliveryRequest(receiptRequest, body, "mail-provider-retiring", testRetiringDeliveryToken, "nonce_early_receipt_001")
	receiptResponse, err := server.Client().Do(receiptRequest)
	if err != nil || receiptResponse.StatusCode != http.StatusAccepted {
		t.Fatalf("early receipt status=%v err=%v", receiptResponse.StatusCode, err)
	}
	receiptResponse.Body.Close()
	var applied bool
	if err := pool.QueryRow(ctx, `SELECT applied_at IS NOT NULL FROM mail_delivery_receipts WHERE message_id = $1`, messageID).Scan(&applied); err != nil || applied {
		t.Fatalf("early receipt applied=%v err=%v, want pending", applied, err)
	}

	worker, _ := mailworker.New(store.New(pool), &captureSender{messageID: messageID}, "worker_early_receipt", testVerificationEncryptionKey, time.Minute, time.Second)
	outcome, err := worker.ProcessOne(ctx)
	if err != nil || outcome.Result != "accepted" {
		t.Fatalf("worker outcome=%+v err=%v", outcome, err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM mail_outbox`).Scan(&status); err != nil || status != "delivered" {
		t.Fatalf("early receipt reconciled status=%s err=%v, want delivered", status, err)
	}

	replayRequest, _ := http.NewRequest(http.MethodPost, server.URL+contract.RecordMailDeliveryRoute, bytes.NewReader(body))
	replayRequest.Header.Set("Content-Type", "application/json")
	signDeliveryRequest(replayRequest, body, "mail-provider-retiring", testRetiringDeliveryToken, "nonce_early_receipt_001")
	replayResponse, err := server.Client().Do(replayRequest)
	if err != nil || replayResponse.StatusCode != http.StatusConflict {
		t.Fatalf("receipt nonce replay status=%v err=%v, want 409", replayResponse.StatusCode, err)
	}
	replayResponse.Body.Close()
}

func TestVerificationAndOutboxRollbackTogether(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	if _, err := pool.Exec(ctx, `CREATE FUNCTION fail_test_mail_outbox_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected outbox failure'; END $$; CREATE TRIGGER fail_test_mail_outbox BEFORE INSERT ON mail_outbox FOR EACH ROW EXECUTE FUNCTION fail_test_mail_outbox_insert()`); err != nil {
		t.Fatalf("install failure injection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TRIGGER IF EXISTS fail_test_mail_outbox ON mail_outbox; DROP FUNCTION IF EXISTS fail_test_mail_outbox_insert()`)
	})
	response := requestVerificationCode(t, server, "request_rollback_001")
	response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("injected outbox failure status=%d, want 500", response.StatusCode)
	}
	var codes, jobs int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM verification_codes), (SELECT count(*) FROM mail_outbox)`).Scan(&codes, &jobs); err != nil || codes != 0 || jobs != 0 {
		t.Fatalf("partial transaction codes=%d jobs=%d err=%v, want 0/0", codes, jobs, err)
	}
}

func TestVerificationRateLimitAndRedisLossDoNotLoseOutbox(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey, StudentEmailDomains: []string{"henu.edu.cn"},
		TrustedProxyCIDRs: []string{"127.0.0.0/8", "::1/128"},
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	first := requestVerificationCode(t, server, "request_rate_limit_001")
	first.Body.Close()
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first verification request = %d, want 202", first.StatusCode)
	}
	limited := requestVerificationCode(t, server, "request_rate_limit_002")
	limited.Body.Close()
	if limited.StatusCode != http.StatusAccepted {
		t.Fatalf("rate-limited request = %d, want privacy-preserving 202", limited.StatusCode)
	}
	var codes, jobs int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM verification_codes), (SELECT count(*) FROM mail_outbox)`).Scan(&codes, &jobs); err != nil || codes != 1 || jobs != 1 {
		t.Fatalf("rate limit durable rows = codes:%d jobs:%d err=%v, want 1/1", codes, jobs, err)
	}
	if err := redisClient.Close(); err != nil {
		t.Fatalf("simulate Redis loss: %v", err)
	}
	unavailable := requestVerificationCodeWith(t, server, "other@henu.edu.cn", "login", "portal", "request_redis_unavailable")
	unavailable.Body.Close()
	if unavailable.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("new request without Redis = %d, want 503", unavailable.StatusCode)
	}
	sender := &captureSender{messageID: "provider_after_redis_loss"}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_no_redis", testVerificationEncryptionKey, time.Minute, time.Second)
	outcome, err := worker.ProcessOne(ctx)
	if err != nil || !outcome.Processed || sender.lastMessage().Code == "" {
		t.Fatalf("process durable outbox without Redis: outcome=%+v err=%v", outcome, err)
	}
}

func TestVerificationIdempotencyExpiryAndAttemptLimit(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	first := requestVerificationCode(t, server, "request_idempotent_001")
	first.Body.Close()
	replay := requestVerificationCode(t, server, "request_idempotent_001")
	replay.Body.Close()
	if first.StatusCode != http.StatusAccepted || replay.StatusCode != http.StatusAccepted {
		t.Fatalf("idempotent request statuses = %d/%d, want 202/202", first.StatusCode, replay.StatusCode)
	}
	conflict := requestVerificationCodeWith(t, server, testStudentEmail, "security", "portal", "request_idempotent_001")
	conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("conflicting idempotency request = %d, want 409", conflict.StatusCode)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_codes`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("idempotent verification rows = %d err=%v, want 1", rows, err)
	}
	sender := &captureSender{messageID: "provider_expiry_001"}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_expiry", testVerificationEncryptionKey, time.Minute, time.Second)
	_, _ = worker.ProcessOne(ctx)
	if _, err := pool.Exec(ctx, `UPDATE verification_codes SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatalf("expire verification: %v", err)
	}
	expired := verifyCode(t, server, sender.lastMessage().Code, "verify_expired_001")
	expired.Body.Close()
	if expired.StatusCode != http.StatusBadRequest {
		t.Fatalf("expired verification = %d, want 400", expired.StatusCode)
	}

	resetIdentityTables(t, ctx, pool, redisClient)
	requested := requestVerificationCode(t, server, "request_attempt_limit_001")
	requested.Body.Close()
	sender = &captureSender{messageID: "provider_attempt_limit_001"}
	worker, _ = mailworker.New(store.New(pool), sender, "worker_attempt_limit", testVerificationEncryptionKey, time.Minute, time.Second)
	_, _ = worker.ProcessOne(ctx)
	wrongCode := "000000"
	if sender.lastMessage().Code == wrongCode {
		wrongCode = "999999"
	}
	for attempt := range 5 {
		wrong := verifyCode(t, server, wrongCode, fmt.Sprintf("verify_wrong_%02d", attempt))
		wrong.Body.Close()
		if wrong.StatusCode != http.StatusBadRequest {
			t.Fatalf("wrong attempt %d = %d, want 400", attempt, wrong.StatusCode)
		}
	}
	correctAfterLock := verifyCode(t, server, sender.lastMessage().Code, "verify_after_lock")
	correctAfterLock.Body.Close()
	if correctAfterLock.StatusCode != http.StatusBadRequest {
		t.Fatalf("correct code after attempt lock = %d, want 400", correctAfterLock.StatusCode)
	}
	var failedAttempts int
	var revoked bool
	if err := pool.QueryRow(ctx, `SELECT failed_attempts, revoked_at IS NOT NULL FROM verification_codes`).Scan(&failedAttempts, &revoked); err != nil || failedAttempts != 5 || !revoked {
		t.Fatalf("attempt lock state = attempts:%d revoked:%v err=%v", failedAttempts, revoked, err)
	}
}

func TestVerificationAppliesIPAndDeviceTimeBuckets(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	for index := range 31 {
		response := requestVerificationCodeFromDevice(t, server,
			fmt.Sprintf("ip-limit-%02d@henu.edu.cn", index), "login", "portal",
			fmt.Sprintf("device-ip-%02d", index), fmt.Sprintf("request_ip_limit_%02d", index))
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("IP bucket request %d = %d, want 202", index, response.StatusCode)
		}
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_codes`).Scan(&rows); err != nil || rows != 30 {
		t.Fatalf("IP hourly bucket rows = %d err=%v, want 30", rows, err)
	}

	resetIdentityTables(t, ctx, pool, redisClient)
	for index := range 11 {
		response := requestVerificationCodeFromDevice(t, server,
			fmt.Sprintf("device-limit-%02d@henu.edu.cn", index), "login", "portal",
			"shared-device-001", fmt.Sprintf("request_device_limit_%02d", index))
		response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("device bucket request %d = %d, want 202", index, response.StatusCode)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM verification_codes`).Scan(&rows); err != nil || rows != 10 {
		t.Fatalf("device hourly bucket rows = %d err=%v, want 10", rows, err)
	}
}

func TestVerificationAttemptsUseDeviceAndIPBuckets(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	for index := range 31 {
		response := verifyCodeWithEmail(t, server, fmt.Sprintf("verify-limit-%02d@henu.edu.cn", index), "000000", fmt.Sprintf("verify_limit_%02d", index))
		response.Body.Close()
		want := http.StatusBadRequest
		if index == 30 {
			want = http.StatusTooManyRequests
		}
		if response.StatusCode != want {
			t.Fatalf("verify device bucket request %d=%d, want %d", index, response.StatusCode, want)
		}
	}
}

func TestMailWorkerRetriesAndRecoversExpiredLease(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	response := requestVerificationCode(t, server, "request_retry_001")
	response.Body.Close()

	sender := &captureSender{messageID: "provider_retry_001", failures: 1}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_retry", testVerificationEncryptionKey, time.Minute, time.Second)
	outcome, err := worker.ProcessOne(ctx)
	if err != nil || !outcome.Processed {
		t.Fatalf("first retry attempt: outcome=%+v err=%v", outcome, err)
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count FROM mail_outbox`).Scan(&status, &attempts); err != nil || status != "retry_due" || attempts != 1 {
		t.Fatalf("retry state = %s/%d err=%v, want retry_due/1", status, attempts, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mail_outbox SET available_at = now() WHERE status = 'retry_due'`); err != nil {
		t.Fatalf("make retry due: %v", err)
	}
	outcome, err = worker.ProcessOne(ctx)
	if err != nil || !outcome.Processed {
		t.Fatalf("second retry attempt: outcome=%+v err=%v", outcome, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count FROM mail_outbox`).Scan(&status, &attempts); err != nil || status != "accepted" || attempts != 2 {
		t.Fatalf("accepted retry state = %s/%d err=%v, want accepted/2", status, attempts, err)
	}

	resetIdentityTables(t, ctx, pool, redisClient)
	response = requestVerificationCode(t, server, "request_recover_001")
	response.Body.Close()
	if _, err := pool.Exec(ctx, `UPDATE mail_outbox SET status = 'processing', attempt_count = 1, locked_by = 'dead_worker', locked_at = now() - interval '5 minutes'`); err != nil {
		t.Fatalf("seed abandoned lease: %v", err)
	}
	recoverySender := &captureSender{messageID: "provider_recovered_001"}
	recoveryWorker, _ := mailworker.New(store.New(pool), recoverySender, "worker_recovery", testVerificationEncryptionKey, time.Minute, time.Second)
	outcome, err = recoveryWorker.ProcessOne(ctx)
	if err != nil || !outcome.Processed {
		t.Fatalf("recover abandoned lease: outcome=%+v err=%v", outcome, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count FROM mail_outbox`).Scan(&status, &attempts); err != nil || status != "accepted" || attempts != 2 {
		t.Fatalf("recovered state = %s/%d err=%v, want accepted/2", status, attempts, err)
	}
}

func TestMailWorkerRecoveryReliesOnProviderIdempotencyAfterAcceptanceCrash(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	response := requestVerificationCode(t, server, "request_acceptance_crash_001")
	response.Body.Close()
	queries := store.New(pool)
	claimed, err := queries.ClaimMailOutbox(ctx, store.ClaimMailOutboxParams{
		WorkerID:      pgtype.Text{String: "worker_killed", Valid: true},
		ReclaimBefore: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
	})
	if err != nil {
		t.Fatalf("claim before simulated crash: %v", err)
	}
	provider := newIdempotentSender("provider_acceptance_crash_001")
	if _, err := provider.Send(ctx, mailworker.Message{IdempotencyKey: claimed.DedupeKey}); err != nil {
		t.Fatalf("provider acceptance: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mail_outbox SET locked_at = now() - interval '5 minutes' WHERE id = $1`, claimed.ID); err != nil {
		t.Fatalf("expire crashed lease: %v", err)
	}
	recoveryWorker, _ := mailworker.New(queries, provider, "worker_after_kill", testVerificationEncryptionKey, time.Minute, time.Second)
	outcome, err := recoveryWorker.ProcessOne(ctx)
	if err != nil || outcome.Result != "accepted" {
		t.Fatalf("recovery outcome=%+v err=%v", outcome, err)
	}
	attempts, deliveries := provider.counts()
	if attempts != 2 || deliveries != 1 {
		t.Fatalf("provider attempts=%d deliveries=%d, want 2/1", attempts, deliveries)
	}
}

func TestMailWorkerTimeoutAndPermanentFailureReachDurableStates(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	response := requestVerificationCode(t, server, "request_timeout_001")
	response.Body.Close()
	timeoutWorker, _ := mailworker.New(store.New(pool), senderFunc(func(ctx context.Context, _ mailworker.Message) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}), "worker_timeout", testVerificationEncryptionKey, time.Minute, 20*time.Millisecond)
	outcome, err := timeoutWorker.ProcessOne(ctx)
	if err != nil || !outcome.Processed {
		t.Fatalf("timeout attempt: outcome=%+v err=%v", outcome, err)
	}
	var status, errorCode string
	if err := pool.QueryRow(ctx, `SELECT status, last_error_code FROM mail_outbox`).Scan(&status, &errorCode); err != nil || status != "retry_due" || errorCode != "SEND_TIMEOUT" {
		t.Fatalf("timeout state = %s/%s err=%v, want retry_due/SEND_TIMEOUT", status, errorCode, err)
	}

	resetIdentityTables(t, ctx, pool, redisClient)
	response = requestVerificationCode(t, server, "request_permanent_001")
	response.Body.Close()
	permanentWorker, _ := mailworker.New(store.New(pool), senderFunc(func(context.Context, mailworker.Message) (string, error) {
		return "", &mailworker.SendError{Code: "RECIPIENT_REJECTED", Permanent: true}
	}), "worker_permanent", testVerificationEncryptionKey, time.Minute, time.Second)
	outcome, err = permanentWorker.ProcessOne(ctx)
	if err != nil || !outcome.Processed {
		t.Fatalf("permanent failure attempt: outcome=%+v err=%v", outcome, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, last_error_code FROM mail_outbox`).Scan(&status, &errorCode); err != nil || status != "failed" || errorCode != "RECIPIENT_REJECTED" {
		t.Fatalf("permanent state = %s/%s err=%v, want failed/RECIPIENT_REJECTED", status, errorCode, err)
	}
	var outboxID string
	var openDeadLetters int
	if err := pool.QueryRow(ctx, `SELECT id::text, (SELECT count(*) FROM mail_dead_letters WHERE outbox_id = mail_outbox.id AND requeued_at IS NULL) FROM mail_outbox`).Scan(&outboxID, &openDeadLetters); err != nil || openDeadLetters != 1 {
		t.Fatalf("permanent failure dead letter = %d err=%v, want 1", openDeadLetters, err)
	}
	if err := mailworker.Requeue(ctx, store.New(pool), outboxID, "req_manual_requeue_001", "operator-test", "provider configuration repaired"); err != nil {
		t.Fatalf("requeue permanent failure: %v", err)
	}
	var attempts int
	var requeuedDeadLetters int
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count, (SELECT count(*) FROM mail_dead_letters WHERE outbox_id = mail_outbox.id AND requeued_at IS NOT NULL) FROM mail_outbox`).Scan(&status, &attempts, &requeuedDeadLetters); err != nil || status != "pending" || attempts != 0 || requeuedDeadLetters != 1 {
		t.Fatalf("requeued state=%s/%d dead_letters=%d err=%v", status, attempts, requeuedDeadLetters, err)
	}
	secondFailureWorker, _ := mailworker.New(store.New(pool), senderFunc(func(context.Context, mailworker.Message) (string, error) {
		return "", &mailworker.SendError{Code: "RECIPIENT_REJECTED", Permanent: true}
	}), "worker_permanent_second_cycle", testVerificationEncryptionKey, time.Minute, time.Second)
	secondOutcome, err := secondFailureWorker.ProcessOne(ctx)
	if err != nil || secondOutcome.Result != "failed" {
		t.Fatalf("second dead-letter cycle outcome=%+v err=%v", secondOutcome, err)
	}
	var totalDeadLetters int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mail_dead_letters WHERE outbox_id = $1::uuid`, outboxID).Scan(&totalDeadLetters); err != nil || totalDeadLetters != 2 {
		t.Fatalf("dead-letter cycles=%d err=%v, want 2", totalDeadLetters, err)
	}
	var auditActions string
	if err := pool.QueryRow(ctx, `SELECT string_agg(action, ',' ORDER BY created_at, id) FROM mail_outbox_audit_events WHERE outbox_id = $1::uuid`, outboxID).Scan(&auditActions); err != nil || auditActions != "claimed,failed,requeued,claimed,failed" {
		t.Fatalf("requeue audit actions=%q err=%v, want claimed,failed,requeued,claimed,failed", auditActions, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mail_outbox_audit_events SET actor_id = 'tampered' WHERE outbox_id = $1::uuid`, outboxID); err == nil {
		t.Fatal("mail outbox audit events allowed mutation")
	}
	if _, err := pool.Exec(ctx, `TRUNCATE mail_outbox_audit_events`); err == nil {
		t.Fatal("mail outbox audit events allowed truncation")
	}
}

type senderFunc func(context.Context, mailworker.Message) (string, error)

func (function senderFunc) Send(ctx context.Context, message mailworker.Message) (string, error) {
	return function(ctx, message)
}

type captureSender struct {
	mu        sync.Mutex
	messageID string
	message   mailworker.Message
	failures  int
}

type idempotentSender struct {
	mu        sync.Mutex
	messageID string
	attempts  int
	delivered map[string]struct{}
}

func newIdempotentSender(messageID string) *idempotentSender {
	return &idempotentSender{messageID: messageID, delivered: map[string]struct{}{}}
}

func (s *idempotentSender) Send(_ context.Context, message mailworker.Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts++
	s.delivered[message.IdempotencyKey] = struct{}{}
	return s.messageID, nil
}

func (s *idempotentSender) counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts, len(s.delivered)
}

func (s *captureSender) Send(_ context.Context, message mailworker.Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
	if s.failures > 0 {
		s.failures--
		return "", &mailworker.SendError{Code: "PROVIDER_UNAVAILABLE"}
	}
	return s.messageID, nil
}

func (s *captureSender) lastMessage() mailworker.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.message
}

func newVerificationServer(t *testing.T, pool *pgxpool.Pool, redisClient *redis.Client) *httptest.Server {
	t.Helper()
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey, StudentEmailDomains: []string{"henu.edu.cn"},
		TrustedProxyCIDRs: []string{"127.0.0.0/8", "::1/128"},
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return server
}

func requestVerificationCode(t *testing.T, server *httptest.Server, idempotencyKey string) *http.Response {
	t.Helper()
	return requestVerificationCodeWith(t, server, testStudentEmail, "login", "portal", idempotencyKey)
}

func requestVerificationCodeWith(t *testing.T, server *httptest.Server, email, purpose, clientID, idempotencyKey string) *http.Response {
	t.Helper()
	return requestVerificationCodeFromDevice(t, server, email, purpose, clientID, "test-device-001", idempotencyKey)
}

func requestVerificationCodeFromDevice(t *testing.T, server *httptest.Server, email, purpose, clientID, deviceID, idempotencyKey string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"email":%q,"purpose":%q,"client_id":%q}`, email, purpose, clientID)
	request, _ := http.NewRequest(http.MethodPost, server.URL+contract.RequestVerificationCodeRoute, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(contract.IdempotencyKeyHeader, idempotencyKey)
	request.Header.Set("X-Forwarded-For", "198.51.100.99, 203.0.113.10")
	response, err := clientForDevice(server, deviceID).Do(request)
	if err != nil {
		t.Fatalf("request verification code: %v", err)
	}
	return response
}

func verifyCode(t *testing.T, server *httptest.Server, code, idempotencyKey string) *http.Response {
	t.Helper()
	return verifyCodeWithEmail(t, server, testStudentEmail, code, idempotencyKey)
}

func verifyCodeWithEmail(t *testing.T, server *httptest.Server, email, code, idempotencyKey string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(contract.VerifyVerificationCodeRequest{Email: email, Code: code, Purpose: "login"})
	request, _ := http.NewRequest(http.MethodPost, server.URL+contract.VerifyVerificationCodeRoute, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(contract.IdempotencyKeyHeader, idempotencyKey)
	response, err := clientForDevice(server, "test-device-001").Do(request)
	if err != nil {
		t.Fatalf("verify code: %v", err)
	}
	return response
}

func clientForDevice(server *httptest.Server, deviceID string) *http.Client {
	key := server.URL + "|" + deviceID
	if existing, ok := testDeviceClients.Load(key); ok {
		return existing.(*http.Client)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Transport: server.Client().Transport, Jar: jar}
	actual, _ := testDeviceClients.LoadOrStore(key, client)
	return actual.(*http.Client)
}

func signDeliveryRequest(request *http.Request, body []byte, keyID, secret, nonce string) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(body)
	canonical := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", request.Method, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:]))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set(contract.KeyIDHeader, keyID)
	request.Header.Set(contract.TimestampHeader, timestamp)
	request.Header.Set(contract.NonceHeader, nonce)
	request.Header.Set(contract.SignatureHeader, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}
