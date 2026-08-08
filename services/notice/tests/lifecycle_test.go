package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	notice "henukit.dev/notice"
)

const testSecret = "notice-gateway-secret-at-least-32-bytes"
const portalReadSecret = "notice-portal-read-secret-at-least-32-bytes"

func TestNoticeLifecycleIsImmutableScopedIdempotentAndAudited(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := notice.New(notice.Config{
		Database: pool, Redis: redisClient,
		ClientID: "console-gateway", Keys: map[string]string{"active": testSecret},
		ReadClientID: "portal-gateway", ReadKeys: map[string]string{"portal-read": portalReadSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	actor := uuid.NewString()

	sourceBody := `{"code":"henu-office","name":"学校办公室","canonical_url":"https://example.edu/notices"}`
	responses := make([][]byte, 2)
	var wait sync.WaitGroup
	for index := range responses {
		wait.Add(1)
		go func() {
			defer wait.Done()
			responses[index] = send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources", sourceBody, "idem_source_1")
		}()
	}
	wait.Wait()
	if fmt.Sprint(dataObject(t, responses[0])) != fmt.Sprint(dataObject(t, responses[1])) {
		t.Fatalf("concurrent idempotent source results differ: %s != %s", responses[0], responses[1])
	}
	source := responses[0]
	sourceID := dataString(t, source, "id")
	version := send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources/"+sourceID+"/versions", `{"title":"暑期安排","body":"不可变正文","source_url":"https://example.edu/notices/1"}`, "idem_version_1")
	versionID := dataString(t, version, "id")

	if _, err := pool.Exec(context.Background(), "UPDATE notice_versions SET body = 'mutated' WHERE id = $1", versionID); err == nil {
		t.Fatal("immutable Notice version accepted UPDATE")
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE notice_audit_events"); err == nil {
		t.Fatal("append-only Notice audit accepted TRUNCATE")
	}

	reviewBody := `{"decision":"approved","note":"来源与正文已核验","expected_revision":1}`
	firstReview := send(t, server.URL, actor, "notice.review", http.MethodPost, "/api/v1/versions/"+versionID+"/reviews", reviewBody, "idem_review_1")
	replayedReview := send(t, server.URL, actor, "notice.review", http.MethodPost, "/api/v1/versions/"+versionID+"/reviews", reviewBody, "idem_review_1")
	if fmt.Sprint(dataObject(t, firstReview)) != fmt.Sprint(dataObject(t, replayedReview)) {
		t.Fatalf("review replay changed result: %s != %s", firstReview, replayedReview)
	}

	distributionBody := `{"channel":"in_app","audience":{"kind":"all_students"},"expected_revision":2}`
	distribution := send(t, server.URL, actor, "notice.distribute", http.MethodPost, "/api/v1/versions/"+versionID+"/distributions", distributionBody, "idem_distribution_1")
	if dataString(t, distribution, "status") != "queued" {
		t.Fatalf("distribution not queued: %s", distribution)
	}
	worker, err := notice.NewWorker(pool, &flakySender{})
	if err != nil {
		t.Fatal(err)
	}
	if processed, err := worker.RunOnce(context.Background()); !processed || err == nil || err.Error() != "temporary provider failure" {
		t.Fatalf("first worker attempt = %v, %v; want processed provider retry", processed, err)
	}
	if processed, err := worker.RunOnce(context.Background()); !processed || err != nil {
		t.Fatalf("second worker attempt = %v, %v; want delivered", processed, err)
	}

	snapshot := send(t, server.URL, actor, "notice.read", http.MethodGet, "/api/v1/console-notices", "", "")
	if !bytes.Contains(snapshot, []byte(`"state":"distributed"`)) || !bytes.Contains(snapshot, []byte(`"distribution_status":"delivered"`)) || !bytes.Contains(snapshot, []byte(`"title":"暑期安排"`)) {
		t.Fatalf("snapshot omitted lifecycle: %s", snapshot)
	}
	if bytes.Contains(snapshot, []byte(`"source_published_at"`)) {
		t.Fatalf("snapshot emitted nullable source_published_at: %s", snapshot)
	}
	portalSnapshot := sendAs(t, server.URL, "portal-gateway", "portal-read", portalReadSecret, actor, "notice.read", http.MethodGet, "/api/v1/console-notices", "", "")
	if !bytes.Contains(portalSnapshot, []byte(`"title":"暑期安排"`)) {
		t.Fatalf("Portal read credential could not read snapshot: %s", portalSnapshot)
	}
	if status := sendStatusAs(t, server.URL, "portal-gateway", "portal-read", portalReadSecret, actor, "notice.manage", "product", http.MethodPost, "/api/v1/sources", sourceBody, "portal_must_not_write"); status != http.StatusForbidden {
		t.Fatalf("Portal read credential write status = %d, want 403", status)
	}
	operation := send(t, server.URL, actor, "notice.read", http.MethodGet, "/api/v1/operations/review", "", "idem_review_1")
	if dataString(t, operation, "state") != "approved" {
		t.Fatalf("operation status omitted stored review: %s", operation)
	}
	summary := send(t, server.URL, "", "", http.MethodGet, "/api/v1/console-summary", "", "")
	if dataString(t, summary, "id") != "notice" {
		t.Fatalf("summary is not Notice: %s", summary)
	}
	health, err := http.Get(server.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", health.StatusCode)
	}
	failedDistributionID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_distributions (id,notice_version_id,channel,audience_kind,actor_user_id,request_id) VALUES ($1,$2,'email','all_students',$3,'req_permanent_failure')`, failedDistributionID, versionID, actor); err != nil {
		t.Fatal(err)
	}
	failedWorker, err := notice.NewWorker(pool, alwaysFailSender{})
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		if processed, runErr := failedWorker.RunOnce(context.Background()); !processed || runErr == nil {
			t.Fatalf("permanent failure attempt %d = %v, %v", attempt, processed, runErr)
		}
	}
	var failedStatus, lastError string
	var failedAttempts, failedAudits int
	if err := pool.QueryRow(context.Background(), `SELECT status,attempt_count,last_error,(SELECT count(*) FROM notice_audit_events WHERE resource_id=$1 AND action='distribution.failed') FROM notice_distributions WHERE id=$1`, failedDistributionID).Scan(&failedStatus, &failedAttempts, &lastError, &failedAudits); err != nil {
		t.Fatal(err)
	}
	if failedStatus != "failed" || failedAttempts != 3 || lastError != "permanent provider failure" || failedAudits != 1 {
		t.Fatalf("failed distribution = %s/%d/%q audits=%d", failedStatus, failedAttempts, lastError, failedAudits)
	}

	denied := sendStatus(t, server.URL, actor, "notice.read", "resource", http.MethodGet, "/api/v1/console-notices", "", "")
	if denied != http.StatusForbidden {
		t.Fatalf("resource-scoped Notice read = %d, want 403", denied)
	}

	nonce := base64.RawURLEncoding.EncodeToString([]byte("fixed-replay-nonce-value"))
	firstReplay := signedRequestWithNonce(t, server.URL, actor, "notice.read", "product", http.MethodGet, "/api/v1/console-notices", "", "", nonce)
	firstReplay.Body.Close()
	secondReplay := signedRequestWithNonce(t, server.URL, actor, "notice.read", "product", http.MethodGet, "/api/v1/console-notices", "", "", nonce)
	defer secondReplay.Body.Close()
	if secondReplay.StatusCode != http.StatusConflict {
		t.Fatalf("replayed service nonce = %d, want 409", secondReplay.StatusCode)
	}

	var reviews, distributions, audits int
	if err := pool.QueryRow(context.Background(), "SELECT (SELECT count(*) FROM notice_reviews), (SELECT count(*) FROM notice_distributions), (SELECT count(*) FROM notice_audit_events)").Scan(&reviews, &distributions, &audits); err != nil {
		t.Fatal(err)
	}
	if reviews != 1 || distributions != 2 || audits != 9 {
		t.Fatalf("facts reviews/distributions/audits=%d/%d/%d, want 1/2/9", reviews, distributions, audits)
	}
}

type flakySender struct{ attempts int }

func (s *flakySender) Deliver(_ context.Context, _ notice.Delivery) error {
	s.attempts++
	if s.attempts == 1 {
		return errors.New("temporary provider failure")
	}
	return nil
}

type alwaysFailSender struct{}

func (alwaysFailSender) Deliver(context.Context, notice.Delivery) error {
	return errors.New("permanent provider failure")
}

func send(t *testing.T, baseURL, actor, permission, method, path, body, key string) []byte {
	t.Helper()
	return sendAs(t, baseURL, "console-gateway", "active", testSecret, actor, permission, method, path, body, key)
}

func sendAs(t *testing.T, baseURL, clientID, keyID, secret, actor, permission, method, path, body, key string) []byte {
	t.Helper()
	response := signedRequestAs(t, baseURL, clientID, keyID, secret, actor, permission, "product", method, path, body, key)
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s = %d: %s", method, path, response.StatusCode, payload)
	}
	return payload
}

func sendStatus(t *testing.T, baseURL, actor, permission, scope, method, path, body, key string) int {
	t.Helper()
	return sendStatusAs(t, baseURL, "console-gateway", "active", testSecret, actor, permission, scope, method, path, body, key)
}

func sendStatusAs(t *testing.T, baseURL, clientID, keyID, secret, actor, permission, scope, method, path, body, key string) int {
	t.Helper()
	response := signedRequestAs(t, baseURL, clientID, keyID, secret, actor, permission, scope, method, path, body, key)
	defer response.Body.Close()
	return response.StatusCode
}

func signedRequest(t *testing.T, baseURL, actor, permission, scope, method, path, body, key string) *http.Response {
	t.Helper()
	return signedRequestAs(t, baseURL, "console-gateway", "active", testSecret, actor, permission, scope, method, path, body, key)
}

func signedRequestAs(t *testing.T, baseURL, clientID, keyID, secret, actor, permission, scope, method, path, body, key string) *http.Response {
	t.Helper()
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	return signedRequestWithCredentialsAndNonce(t, baseURL, clientID, keyID, secret, actor, permission, scope, method, path, body, key, nonce)
}

func signedRequestWithNonce(t *testing.T, baseURL, actor, permission, scope, method, path, body, key, nonce string) *http.Response {
	t.Helper()
	return signedRequestWithCredentialsAndNonce(t, baseURL, "console-gateway", "active", testSecret, actor, permission, scope, method, path, body, key, nonce)
}

func signedRequestWithCredentialsAndNonce(t *testing.T, baseURL, clientID, keyID, secret, actor, permission, scope, method, path, body, key, nonce string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth(clientID, secret)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	digest := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Id", clientID)
	request.Header.Set("X-Key-Id", keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Actor-User-Id", actor)
	request.Header.Set("X-Permission-Code", permission)
	request.Header.Set("X-Scope-Kind", scope)
	request.Header.Set("X-Product-Code", "notice")
	request.Header.Set("X-Request-Id", "req_"+uuid.NewString())
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func dataString(t *testing.T, payload []byte, key string) string {
	t.Helper()
	value, ok := dataObject(t, payload)[key].(string)
	if !ok {
		t.Fatalf("missing string %s in %s", key, payload)
	}
	return value
}

func dataObject(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}
