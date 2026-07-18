package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	"henukit.dev/platform-core/internal/store"
)

const testStudentEmail = "student@henu.edu.cn"

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
	processed, err := worker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("process verification mail: processed=%v err=%v", processed, err)
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
	replay := verifyCode(t, server, message.Code, key)
	replay.Body.Close()
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("idempotent verification replay = %d, want 200", replay.StatusCode)
	}
	delivered, err := worker.MarkDelivered(ctx, sender.messageID)
	if err != nil || !delivered {
		t.Fatalf("mark provider delivery: delivered=%v err=%v", delivered, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM mail_outbox WHERE verification_code_id = $1`, verificationID).Scan(&status); err != nil || status != "delivered" {
		t.Fatalf("final delivery status = %s err=%v, want delivered", status, err)
	}
}

func TestVerificationRateLimitAndRedisLossDoNotLoseOutbox(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey, StudentEmailDomains: []string{"henu.edu.cn"},
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
	if limited.StatusCode != http.StatusTooManyRequests || limited.Header.Get("Retry-After") != "60" {
		t.Fatalf("rate-limited request = %d retry=%q, want 429/60", limited.StatusCode, limited.Header.Get("Retry-After"))
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
	processed, err := worker.ProcessOne(ctx)
	if err != nil || !processed || sender.lastMessage().Code == "" {
		t.Fatalf("process durable outbox without Redis: processed=%v err=%v", processed, err)
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

func TestMailWorkerRetriesAndRecoversExpiredLease(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newVerificationServer(t, pool, redisClient)
	response := requestVerificationCode(t, server, "request_retry_001")
	response.Body.Close()

	sender := &captureSender{messageID: "provider_retry_001", failures: 1}
	worker, _ := mailworker.New(store.New(pool), sender, "worker_retry", testVerificationEncryptionKey, time.Minute, time.Second)
	processed, err := worker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("first retry attempt: processed=%v err=%v", processed, err)
	}
	var status string
	var attempts int
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count FROM mail_outbox`).Scan(&status, &attempts); err != nil || status != "retry_due" || attempts != 1 {
		t.Fatalf("retry state = %s/%d err=%v, want retry_due/1", status, attempts, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE mail_outbox SET available_at = now() WHERE status = 'retry_due'`); err != nil {
		t.Fatalf("make retry due: %v", err)
	}
	processed, err = worker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("second retry attempt: processed=%v err=%v", processed, err)
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
	processed, err = recoveryWorker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("recover abandoned lease: processed=%v err=%v", processed, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, attempt_count FROM mail_outbox`).Scan(&status, &attempts); err != nil || status != "accepted" || attempts != 2 {
		t.Fatalf("recovered state = %s/%d err=%v, want accepted/2", status, attempts, err)
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
	processed, err := timeoutWorker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("timeout attempt: processed=%v err=%v", processed, err)
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
	processed, err = permanentWorker.ProcessOne(ctx)
	if err != nil || !processed {
		t.Fatalf("permanent failure attempt: processed=%v err=%v", processed, err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, last_error_code FROM mail_outbox`).Scan(&status, &errorCode); err != nil || status != "failed" || errorCode != "RECIPIENT_REJECTED" {
		t.Fatalf("permanent state = %s/%s err=%v, want failed/RECIPIENT_REJECTED", status, errorCode, err)
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
	body := fmt.Sprintf(`{"email":%q,"purpose":%q,"client_id":%q}`, email, purpose, clientID)
	request, _ := http.NewRequest(http.MethodPost, server.URL+contract.RequestVerificationCodeRoute, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(contract.IdempotencyKeyHeader, idempotencyKey)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("request verification code: %v", err)
	}
	return response
}

func verifyCode(t *testing.T, server *httptest.Server, code, idempotencyKey string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(contract.VerifyVerificationCodeRequest{Email: testStudentEmail, Code: code, Purpose: "login"})
	request, _ := http.NewRequest(http.MethodPost, server.URL+contract.VerifyVerificationCodeRoute, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(contract.IdempotencyKeyHeader, idempotencyKey)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("verify code: %v", err)
	}
	return response
}
