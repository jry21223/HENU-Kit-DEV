package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/mailworker"
	"henukit.dev/platform-core/internal/store"
)

const (
	careerDigestClientID = "career-digest-mail"
	careerDigestKeyID    = "career-digest-key-1"
	careerDigestSecret   = "career-digest-secret-at-least-32-characters"
)

func seedDigestRecipient(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string, verified bool) string {
	t.Helper()
	userID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email_verified, status, display_name) VALUES ($1, $2, 'active', '求职雷达测试用户')`, userID, verified); err != nil {
		t.Fatalf("seed digest user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_identities (user_id, email_lookup_hash, email_ciphertext, verified_at) VALUES ($1, $2, $3, now())`, userID, lookupHash(email), sealTestEmail(t, email)); err != nil {
		t.Fatalf("seed digest identity: %v", err)
	}
	return userID.String()
}

func newDigestTestServer(t *testing.T, pool *pgxpool.Pool, redisClient *redis.Client, configureCredentials bool) *httptest.Server {
	t.Helper()
	config := platformcore.Config{
		Database: pool, Redis: redisClient, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		IdempotencyEncryptionKey:  testIdempotencyEncryptionKey,
		VerificationEncryptionKey: testVerificationEncryptionKey,
	}
	if configureCredentials {
		config.CareerDigestClientID = careerDigestClientID
		config.CareerDigestKeyID = careerDigestKeyID
		config.CareerDigestSecret = careerDigestSecret
	}
	handler, err := platformcore.New(config)
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return server
}

func sendCareerDigestEnqueue(t *testing.T, server *httptest.Server, userID, searchID string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/career-digest-mails", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Id", careerDigestClientID)
	request.Header.Set("X-Key-Id", careerDigestKeyID)
	request.Header.Set("X-Request-Id", "req_digest_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	request.Header.Set("Idempotency-Key", "career_search_completed:"+searchID)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{http.MethodPost, "/api/v1/career-digest-mails", timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(careerDigestSecret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.SetBasicAuth(careerDigestClientID, careerDigestSecret)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func digestEnqueueBody(userID, searchID string) []byte {
	return []byte(`{"user_id":"` + userID + `","search_id":"` + searchID + `","completed_at":"2026-08-15T06:30:00Z","source_count":2,"job_count":3,"matched_count":1,"summary":"已扫描 2 个来源，发现 3 个岗位，1 个推荐","career_url":"https://portal.henukit.cn/career?search=` + searchID + `","top_jobs":[{"company":"测试公司","title":"后端开发实习生","location":"郑州","url":"https://example.test/jobs/1","match_score":90,"match_reasons":["匹配目标岗位 后端开发"]}]}`)
}

func TestCareerDigestEnqueueDeliversThroughMailWorker(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	userID := seedDigestRecipient(t, ctx, pool, testStudentEmail, true)
	server := newDigestTestServer(t, pool, redisClient, true)

	response := sendCareerDigestEnqueue(t, server, userID, "search-001", digestEnqueueBody(userID, "search-001"))
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("digest enqueue = %d %s, want 202", response.StatusCode, body)
	}
	if bytes.Contains(body, []byte(testStudentEmail)) || bytes.Contains(body, []byte("ciphertext")) || bytes.Contains(body, []byte(careerDigestSecret)) {
		t.Fatalf("digest response disclosed the email, ciphertext, or secret: %s", body)
	}

	var kind, dedupeKey string
	var recipientUserID pgtype.UUID
	if err := pool.QueryRow(ctx, `SELECT kind,dedupe_key,recipient_user_id FROM mail_outbox WHERE dedupe_key='career_search_completed:search-001'`).Scan(&kind, &dedupeKey, &recipientUserID); err != nil {
		t.Fatalf("read digest outbox row: %v", err)
	}
	if kind != "career_digest" || recipientUserID.String() != userID {
		t.Fatalf("outbox row kind=%s recipient=%s, want career_digest/%s", kind, recipientUserID.String(), userID)
	}

	sender := &captureSender{messageID: "provider_digest_001"}
	worker, err := mailworker.New(store.New(pool), sender, "worker_digest_001", testVerificationEncryptionKey, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("create mail worker: %v", err)
	}
	outcome, err := worker.ProcessOne(ctx)
	if err != nil || !outcome.Processed {
		t.Fatalf("process digest mail: outcome=%+v err=%v", outcome, err)
	}
	message := sender.lastMessage()
	if message.Template != "henukit_career_digest" || message.Recipient != testStudentEmail || message.Digest == nil || message.Digest.SearchID != "search-001" {
		t.Fatalf("unexpected digest mail job: %+v", message)
	}
	if message.Digest.SourceCount != 2 || message.Digest.MatchedCount != 1 || len(message.Digest.TopJobs) != 1 || message.Digest.TopJobs[0].Company != "测试公司" {
		t.Fatalf("digest mail payload wrong: %+v", message.Digest)
	}
}

func TestCareerDigestEnqueueFailsClosedWithoutVerifiedEmail(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	server := newDigestTestServer(t, pool, redisClient, true)

	unverified := seedDigestRecipient(t, ctx, pool, "unverified@henu.edu.cn", false)
	response := sendCareerDigestEnqueue(t, server, unverified, "search-unverified", digestEnqueueBody(unverified, "search-unverified"))
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unverified enqueue = %d %s, want 404", response.StatusCode, body)
	}

	unknown := uuid.NewString()
	response = sendCareerDigestEnqueue(t, server, unknown, "search-unknown", digestEnqueueBody(unknown, "search-unknown"))
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown user enqueue = %d %s, want 404", response.StatusCode, body)
	}
	var outboxCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mail_outbox`).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 0 {
		t.Fatalf("fail-closed enqueues wrote %d outbox rows, want 0", outboxCount)
	}
}

func TestCareerDigestEnqueueIsIdempotentPerSearch(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	userID := seedDigestRecipient(t, ctx, pool, testStudentEmail, true)
	server := newDigestTestServer(t, pool, redisClient, true)

	first := sendCareerDigestEnqueue(t, server, userID, "search-replay", digestEnqueueBody(userID, "search-replay"))
	firstBody, _ := io.ReadAll(first.Body)
	first.Body.Close()
	second := sendCareerDigestEnqueue(t, server, userID, "search-replay", digestEnqueueBody(userID, "search-replay"))
	secondBody, _ := io.ReadAll(second.Body)
	second.Body.Close()
	if first.StatusCode != http.StatusAccepted || second.StatusCode != http.StatusAccepted {
		t.Fatalf("replay statuses = %d/%d %s/%s, want 202/202", first.StatusCode, second.StatusCode, firstBody, secondBody)
	}
	var outboxCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM mail_outbox`).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("idempotent replay created %d outbox rows, want 1", outboxCount)
	}
}

func TestCareerDigestEnqueueRejectsBadSignature(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	userID := seedDigestRecipient(t, ctx, pool, testStudentEmail, true)
	server := newDigestTestServer(t, pool, redisClient, true)

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/career-digest-mails", bytes.NewReader(digestEnqueueBody(userID, "search-bad-auth")))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Id", careerDigestClientID)
	request.Header.Set("X-Key-Id", careerDigestKeyID)
	request.Header.Set("X-Timestamp", time.Now().Format("2006"))
	request.Header.Set("X-Nonce", base64.RawURLEncoding.EncodeToString([]byte("not-a-24-byte-nonce")))
	request.Header.Set("X-Signature", "not-a-signature")
	request.SetBasicAuth(careerDigestClientID, "wrong-secret-entirely-wrong-32-bytes!!")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad auth = %d, want 401", response.StatusCode)
	}
}

func TestCareerDigestEnqueueFailsClosedWhenUnconfigured(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	userID := seedDigestRecipient(t, ctx, pool, testStudentEmail, true)
	server := newDigestTestServer(t, pool, redisClient, false)

	response := sendCareerDigestEnqueue(t, server, userID, "search-unconfigured", digestEnqueueBody(userID, "search-unconfigured"))
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured enqueue = %d, want 503", response.StatusCode)
	}
}
