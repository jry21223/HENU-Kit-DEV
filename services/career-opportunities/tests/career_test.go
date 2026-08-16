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
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	career "henukit.dev/career"
)

const (
	careerClientID = "portal-gateway-career"
	careerSecret   = "career-service-secret-at-least-32-bytes"
	actorA         = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	actorB         = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

var errInjected = &injectedError{}

type injectedError struct{}

func (e *injectedError) Error() string { return "injected crawler failure" }

func newCareerServer(t *testing.T, work career.WorkFunc) (*httptest.Server, *pgxpool.Pool) {
	return newCareerServerWithExtract(t, work, nil, 0)
}

func newCareerServerWithExtract(t *testing.T, work career.WorkFunc, extract career.ExtractFunc, rateLimit int) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("CAREER_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("CAREER_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `TRUNCATE career_search_operations, career_search_results, career_searches, career_profiles, career_resume_extractions`); err != nil {
		t.Fatal(err)
	}
	service, err := career.New(career.Config{
		Database: pool, Redis: redisClient, ClientID: careerClientID, Keys: map[string]string{"active": careerSecret}, Work: work,
		Extract: extract, ExtractRateLimit: rateLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })
	return httptest.NewServer(service), pool
}

// newCareerServerWithDigest is the digest-enabled variant used by the search
// digest tests.
func newCareerServerWithDigest(t *testing.T, work career.WorkFunc, sender career.DigestSender, resultURL string) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("CAREER_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("CAREER_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `TRUNCATE career_search_operations, career_search_results, career_searches, career_profiles, career_resume_extractions`); err != nil {
		t.Fatal(err)
	}
	service, err := career.New(career.Config{
		Database: pool, Redis: redisClient, ClientID: careerClientID, Keys: map[string]string{"active": careerSecret}, Work: work,
		DigestSender: sender, DigestResultURL: resultURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })
	return httptest.NewServer(service), pool
}

func send(t *testing.T, baseURL, actorID, method, path string, body []byte, idempotencyKey string) *http.Response {
	t.Helper()
	return sendSigned(t, baseURL, actorID, method, path, body, "application/json", idempotencyKey)
}

// sendMultipart signs and sends a raw multipart body the same way the Portal
// Gateway forwards an upload: the multipart bytes are the signed body.
func sendMultipart(t *testing.T, baseURL, actorID, path, fileName string, content []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return sendSigned(t, baseURL, actorID, http.MethodPost, path, body.Bytes(), writer.FormDataContentType(), "")
}

func sendSigned(t *testing.T, baseURL, actorID, method, path string, body []byte, contentType, idempotencyKey string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("X-Service-Id", careerClientID)
	request.Header.Set("X-Key-Id", "active")
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Request-Id", "req_career_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	digest := sha256.Sum256(body)
	// The actor UUID is the sixth canonical line: Career binds the actor into
	// the signature exactly like Account Portfolio, matching Portal Gateway's
	// SignWithActor. A signed request whose actor was not bound could be
	// replayed with a different X-Actor-User-Id.
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:]), actorID}, "\n")
	mac := hmac.New(sha256.New, []byte(careerSecret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.SetBasicAuth(careerClientID, careerSecret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func decodeData(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func profileBody(actor string) []byte {
	return []byte(`{"profile":{"directions":["后端"],"skills":["go"],"location":"远程"}}`)
}

// --- Seam 1: create returns quickly, persists actor + frozen snapshot -------

func TestCreateSearchReturnsQueuedImmediately(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	start := time.Now()
	response := send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_create_fast")
	payload := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.StatusCode, payload)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("create took %s, expected to return fast", elapsed)
	}
	data := decodeData(t, payload)
	search := data["search"].(map[string]any)
	if search["status"] != "queued" {
		t.Fatalf("created search status = %v, want queued", search["status"])
	}
	id, ok := search["id"].(string)
	if !ok || id == "" {
		t.Fatalf("created search missing id: %v", search)
	}
	var userID, snapshot string
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT user_id,status,profile_snapshot::text FROM career_searches WHERE id=$1`, id).Scan(&userID, &status, &snapshot); err != nil {
		t.Fatal(err)
	}
	if userID != actorA || status != "queued" {
		t.Fatalf("persisted user_id=%s status=%s", userID, status)
	}
	if !strings.Contains(snapshot, "后端") || !strings.Contains(snapshot, "go") {
		t.Fatalf("profile snapshot not frozen: %s", snapshot)
	}
}

func TestCreateSearchRejectsBrowserSuppliedUserID(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	// The body carries a self-reported user_id field. It must never become the
	// owner: the signed X-Actor-User-Id header is authoritative, so a body that
	// tries to self-report is rejected outright by the strict decoder.
	body := []byte(`{"profile":{"directions":["前端"]},"user_id":"cccccccc-cccc-4ccc-8ccc-cccccccccccc"}`)
	response := send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", body, "idem_actor_bound")
	payload := readBody(t, response)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("create status = %d, want 400: %s", response.StatusCode, payload)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_searches`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("self-reported user_id created %d rows, want 0", count)
	}
}

// --- Seam 2: actor-isolated status and history reads ------------------------

func TestStatusAndHistoryAreActorIsolated(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	create := send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_iso")
	createPayload := readBody(t, create)
	if create.StatusCode != http.StatusOK {
		t.Fatalf("create failed: %d", create.StatusCode)
	}
	id := decodeData(t, createPayload)["search"].(map[string]any)["id"].(string)

	// Actor B must not see A's search.
	other := send(t, server.URL, actorB, http.MethodGet, "/api/v1/career/searches/"+id, nil, "")
	if other.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-actor status status = %d, want 404", other.StatusCode)
	}
	history := decodeData(t, readBody(t, send(t, server.URL, actorB, http.MethodGet, "/api/v1/career/searches", nil, "")))
	if list := history["searches"].([]any); len(list) != 0 {
		t.Fatalf("actor B history not empty: %v", list)
	}

	// Actor A sees it in status and history.
	own := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches/"+id, nil, "")))
	if own["search"].(map[string]any)["status"] != "queued" {
		t.Fatalf("own status = %v", own["search"].(map[string]any)["status"])
	}
	ownHistory := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches", nil, "")))
	if list := ownHistory["searches"].([]any); len(list) != 1 {
		t.Fatalf("actor A history len = %d, want 1", len(list))
	}
}

func TestStatusNotFoundForUnknownID(t *testing.T) {
	server, _ := newCareerServer(t, nil)
	defer server.Close()
	unknown := "11111111-1111-4111-8111-111111111111"
	response := send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches/"+unknown, nil, "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.StatusCode)
	}
}

// --- Seam 3: worker advances queued -> running -> completed -----------------

func TestWorkerAdvancesSearchToCompleted(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_worker"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	done, err := worker.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("worker claimed nothing despite a queued search")
	}

	var status string
	var resultCount int
	if err := pool.QueryRow(context.Background(), `SELECT status FROM career_searches WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("status = %s, want completed", status)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_search_results WHERE search_id=$1`, id).Scan(&resultCount); err != nil {
		t.Fatal(err)
	}
	if resultCount != 1 {
		t.Fatalf("result rows = %d, want 1", resultCount)
	}
	status = readStatus(t, pool, id)
	if status != "completed" {
		t.Fatalf("re-read status = %s", status)
	}
}

// --- Seam 4: failure lands on failed with a stable code ---------------------

func TestWorkerFailureLandsOnFailedWithStableCode(t *testing.T) {
	server, pool := newCareerServer(t, func(ctx context.Context, profile any) (career.WorkResult, error) {
		return career.WorkResult{}, errInjected
	})
	defer server.Close()
	defer pool.Close()

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_fail"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}

	var status, errorCode string
	var resultCount int
	if err := pool.QueryRow(context.Background(), `SELECT status,COALESCE(error_code,'') FROM career_searches WHERE id=$1`, id).Scan(&status, &errorCode); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("status = %s, want failed", status)
	}
	if errorCode != "CAREER_WORK_FAILED" {
		t.Fatalf("error_code = %s, want CAREER_WORK_FAILED", errorCode)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_search_results WHERE search_id=$1`, id).Scan(&resultCount); err != nil {
		t.Fatal(err)
	}
	if resultCount != 0 {
		t.Fatalf("failed search wrote %d result rows, want 0", resultCount)
	}
}

// --- Seam 5: idempotent replay never writes a second result -----------------

func TestReplayedCompletionWritesNoSecondResult(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_dup"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Replaying Step after completion must be a no-op.
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	var resultCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_search_results WHERE search_id=$1`, id).Scan(&resultCount); err != nil {
		t.Fatal(err)
	}
	if resultCount != 1 {
		t.Fatalf("result rows = %d, want exactly 1", resultCount)
	}
}

func TestIdempotencyKeyReplayReturnsSameSearch(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	key := "idem_replay_same"
	first := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), key)))
	firstID := first["search"].(map[string]any)["id"].(string)
	second := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), key)))
	secondID := second["search"].(map[string]any)["id"].(string)
	if firstID != secondID {
		t.Fatalf("idempotent replay returned different search: %s vs %s", firstID, secondID)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_searches WHERE id=$1`, firstID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent replay created %d rows, want 1", count)
	}
}

// --- Seam 6: migration applies twice without error --------------------------

func TestMigrationAppliesTwice(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("CAREER_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, name := range []string{"000001_career_searches", "000002_career_profiles"} {
		up, err := os.ReadFile("../db/migrations/" + name + ".up.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(up)); err != nil {
			t.Fatalf("reapplying %s failed: %v", name, err)
		}
	}
}

// --- Seam 7: Career profile is actor-scoped and validated -------------------

func TestProfileRoundTripAndActorIsolation(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	body := []byte(`{"target_roles":"后端开发","tech_stack":"go,postgres","locations":"郑州","job_type":"daily_intern","graduation_year":2027,"resume_text":"校内项目经历","email_notification_enabled":true}`)
	put := send(t, server.URL, actorA, http.MethodPut, "/api/v1/career/profile", body, "")
	if put.StatusCode != http.StatusOK {
		t.Fatalf("put status = %d: %s", put.StatusCode, readBody(t, put))
	}
	got := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/profile", nil, "")))
	profile := got["profile"].(map[string]any)
	if profile["target_roles"] != "后端开发" || profile["tech_stack"] != "go,postgres" {
		t.Fatalf("profile round trip wrong: %v", profile)
	}
	if profile["graduation_year"] != float64(2027) {
		t.Fatalf("graduation_year = %v, want 2027", profile["graduation_year"])
	}

	// Actor B must not see A's profile.
	other := decodeData(t, readBody(t, send(t, server.URL, actorB, http.MethodGet, "/api/v1/career/profile", nil, "")))
	if otherProfile := other["profile"].(map[string]any); otherProfile["target_roles"] != "" {
		t.Fatalf("actor B saw A's profile: %v", otherProfile)
	}

	var storedInCareer int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_profiles WHERE user_id=$1`, actorA).Scan(&storedInCareer); err != nil {
		t.Fatal(err)
	}
	if storedInCareer != 1 {
		t.Fatalf("profile rows in career = %d, want 1", storedInCareer)
	}
}

func TestProfileRejectsInvalidJobTypeAndYear(t *testing.T) {
	server, _ := newCareerServer(t, nil)
	defer server.Close()

	cases := []string{
		`{"job_type":"bogus"}`,
		`{"graduation_year":1800}`,
		`{"graduation_year":2999}`,
	}
	for _, body := range cases {
		response := send(t, server.URL, actorA, http.MethodPut, "/api/v1/career/profile", []byte(body), "")
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid profile %s got status %d", body, response.StatusCode)
		}
	}
}

// --- helpers -----------------------------------------------------------------

func TestWorkerReclaimsStaleRunningSearch(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_stale"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	// Simulate a worker that died mid-run: the row is stuck in 'running' with a
	// stale started_at, beyond the stale-claim window.
	if _, err := pool.Exec(context.Background(), `UPDATE career_searches SET status='running',stage='crawling',started_at=now()-interval '30 minutes' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}

	worker := server.Config.Handler.(*career.Service).Claims()
	done, err := worker.Step(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("worker did not reclaim the stale running search")
	}
	if status := readStatus(t, pool, id); status != "completed" {
		t.Fatalf("status after reclaim = %s, want completed", status)
	}
	var resultCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_search_results WHERE search_id=$1`, id).Scan(&resultCount); err != nil {
		t.Fatal(err)
	}
	if resultCount != 1 {
		t.Fatalf("result rows = %d, want 1", resultCount)
	}
}

// --- Seam 8: completed search enqueues the #397 digest mail ----------------

type digestRecorder struct {
	mu       sync.Mutex
	requests []career.DigestRequest
	err      error
}

func (recorder *digestRecorder) SendDigest(_ context.Context, request career.DigestRequest) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.requests = append(recorder.requests, request)
	return recorder.err
}

func (recorder *digestRecorder) count() int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return len(recorder.requests)
}

func (recorder *digestRecorder) last() career.DigestRequest {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.requests) == 0 {
		return career.DigestRequest{}
	}
	return recorder.requests[len(recorder.requests)-1]
}

func seedProfileNotifications(t *testing.T, pool *pgxpool.Pool, userID string, enabled bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO career_profiles(user_id,email_notification_enabled) VALUES($1,$2)`, userID, enabled); err != nil {
		t.Fatal(err)
	}
}

func digestWork() career.WorkFunc {
	return func(ctx context.Context, profile any) (career.WorkResult, error) {
		return career.WorkResult{
			Payload: map[string]any{"jobs": []career.Job{
				{SourceKey: "fake.a", Company: "测试公司", Title: "后端开发实习生", Location: "郑州", URL: "https://example.test/jobs/1", MatchScore: 90, MatchReasons: []string{"匹配目标岗位 后端开发"}},
				{SourceKey: "fake.b", Company: "内推科技", Title: "前端实习生", Location: "远程", URL: "https://example.test/jobs/2", MatchScore: 70, MatchReasons: nil},
				{SourceKey: "fake.a", Company: "测试公司", Title: "数据实习生", Location: "郑州", URL: "https://example.test/jobs/3", MatchScore: 55, MatchReasons: nil},
			}},
			SourceCount: 2, JobCount: 3, MatchedCount: 2,
			Summary: "已扫描 2 个来源，发现 3 个岗位，2 个推荐",
		}, nil
	}
}

func TestCompletedSearchEnqueuesDigestWhenNotificationsEnabled(t *testing.T) {
	recorder := &digestRecorder{}
	server, pool := newCareerServerWithDigest(t, digestWork(), recorder, "https://portal.henukit.cn/career")
	defer server.Close()
	defer pool.Close()
	seedProfileNotifications(t, pool, actorA, true)

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_digest_on"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.count() != 1 {
		t.Fatalf("digest enqueues = %d, want 1", recorder.count())
	}
	digest := recorder.last()
	if digest.UserID != actorA || digest.SearchID != id || digest.SourceCount != 2 || digest.JobCount != 3 || digest.MatchedCount != 2 {
		t.Fatalf("unexpected digest request: %+v", digest)
	}
	if digest.Summary != "已扫描 2 个来源，发现 3 个岗位，2 个推荐" || digest.CompletedAt == "" {
		t.Fatalf("digest summary/time missing: %+v", digest)
	}
	if digest.CareerURL != "https://portal.henukit.cn/career?search="+id {
		t.Fatalf("digest career URL = %q", digest.CareerURL)
	}
	if digest.RequestID == "" || !strings.HasPrefix(digest.RequestID, "req_") {
		t.Fatalf("digest request id missing: %+v", digest)
	}
	if len(digest.TopJobs) != 3 || digest.TopJobs[0].Title != "后端开发实习生" || digest.TopJobs[1].Title != "前端实习生" {
		t.Fatalf("digest top jobs not score-ordered: %+v", digest.TopJobs)
	}
	if digest.TopJobs[0].Company != "测试公司" || len(digest.TopJobs[0].MatchReasons) != 1 {
		t.Fatalf("digest job fields missing: %+v", digest.TopJobs[0])
	}
	rawDigest, _ := json.Marshal(digest)
	for _, internal := range []string{"source_key", "description", "requirements", "job_type", "profile_snapshot"} {
		if strings.Contains(string(rawDigest), internal) {
			t.Fatalf("digest request leaked internal field %q: %s", internal, rawDigest)
		}
	}
	var emailSentAt any
	if err := pool.QueryRow(context.Background(), `SELECT email_sent_at FROM career_searches WHERE id=$1`, id).Scan(&emailSentAt); err != nil {
		t.Fatal(err)
	}
	if emailSentAt == nil {
		t.Fatal("email_sent_at was not recorded after a successful enqueue")
	}
}

func TestCompletedSearchSkipsDigestWhenNotificationsDisabled(t *testing.T) {
	recorder := &digestRecorder{}
	server, pool := newCareerServerWithDigest(t, digestWork(), recorder, "")
	defer server.Close()
	defer pool.Close()
	seedProfileNotifications(t, pool, actorA, false)

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_digest_off"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.count() != 0 {
		t.Fatalf("digest enqueues = %d, want 0", recorder.count())
	}
	var emailSentAt any
	if err := pool.QueryRow(context.Background(), `SELECT email_sent_at FROM career_searches WHERE id=$1`, id).Scan(&emailSentAt); err != nil {
		t.Fatal(err)
	}
	if emailSentAt != nil {
		t.Fatal("email_sent_at set for a disabled digest")
	}
}

func TestCompletedSearchDefaultsToNotificationsEnabled(t *testing.T) {
	recorder := &digestRecorder{}
	server, pool := newCareerServerWithDigest(t, digestWork(), recorder, "")
	defer server.Close()
	defer pool.Close()

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_digest_default"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.count() != 1 || recorder.last().SearchID != id {
		t.Fatalf("digest enqueues = %d, want 1 for the default-enabled profile", recorder.count())
	}
}

func TestDigestEnqueueFailureDoesNotRollBackSearch(t *testing.T) {
	recorder := &digestRecorder{err: errors.New("platform core rejected the digest enqueue")}
	server, pool := newCareerServerWithDigest(t, digestWork(), recorder, "")
	defer server.Close()
	defer pool.Close()
	seedProfileNotifications(t, pool, actorA, true)

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_digest_fail"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status string
	var resultCount int
	var emailSentAt any
	if err := pool.QueryRow(context.Background(), `SELECT status FROM career_searches WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("status after digest failure = %s, want completed (best-effort)", status)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_search_results WHERE search_id=$1`, id).Scan(&resultCount); err != nil {
		t.Fatal(err)
	}
	if resultCount != 1 {
		t.Fatalf("result rows after digest failure = %d, want 1", resultCount)
	}
	if err := pool.QueryRow(context.Background(), `SELECT email_sent_at FROM career_searches WHERE id=$1`, id).Scan(&emailSentAt); err != nil {
		t.Fatal(err)
	}
	if emailSentAt != nil {
		t.Fatal("email_sent_at set despite a failed enqueue; a later retry must be possible")
	}
}

func TestFailureErrorMessageIsBrowserSafe(t *testing.T) {
	server, pool := newCareerServer(t, func(ctx context.Context, profile any) (career.WorkResult, error) {
		return career.WorkResult{}, errors.New("dial tcp /var/run/internal-secret-socket: connection refused")
	})
	defer server.Close()
	defer pool.Close()

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_browser_safe"))
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	status := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches/"+id, nil, "")))["search"].(map[string]any)
	if msg := status["error_message"]; msg != "job execution failed" {
		t.Fatalf("error_message leaked internal detail: %v", msg)
	}
	if code := status["error_code"]; code != "CAREER_WORK_FAILED" {
		t.Fatalf("error_code = %v, want CAREER_WORK_FAILED", code)
	}
}

// --- Seam 9: database unavailable fails closed ------------------------------

// newCareerServerWithBrokenDatabase serves the API against a pool that can
// never connect. The Redis nonce store stays healthy, so the request passes
// service authentication and the fail-closed behavior is observed at the
// database boundary rather than earlier in the chain.
func newCareerServerWithBrokenDatabase(t *testing.T, work career.WorkFunc) *httptest.Server {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), "postgres://career:career@127.0.0.1:1/career_test?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("CAREER_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	service, err := career.New(career.Config{
		Database: pool, Redis: redisClient, ClientID: careerClientID, Keys: map[string]string{"active": careerSecret}, Work: work,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_ = redisClient.Close()
	})
	return httptest.NewServer(service)
}

func TestDatabaseUnavailableFailsClosed(t *testing.T) {
	server := newCareerServerWithBrokenDatabase(t, nil)
	defer server.Close()

	// Create cannot begin its transaction: 503, never a partial row.
	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_db_down"))
	if code := decodeErrorCode(t, create); code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("create with dead database error = %v, want DEPENDENCY_UNAVAILABLE: %s", code, create)
	}

	// List and status reads fail the same way instead of serving empty data.
	if code := decodeErrorCode(t, readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches", nil, ""))); code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("list with dead database did not fail closed")
	}
	status := readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", nil, ""))
	if code := decodeErrorCode(t, status); code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("status with dead database error = %v, want DEPENDENCY_UNAVAILABLE: %s", code, status)
	}
}

func decodeErrorCode(t *testing.T, payload []byte) string {
	t.Helper()
	var envelope struct {
		Error map[string]string `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode error envelope %s: %v", payload, err)
	}
	return envelope.Error["code"]
}

// --- Seam 10: oversized payload is rejected before any write ----------------

func TestCreateSearchRejectsOversizedPayload(t *testing.T) {
	server, pool := newCareerServer(t, nil)
	defer server.Close()
	defer pool.Close()

	// The request body cap is 512 KiB; a larger profile must fail browser-safe
	// without creating a row.
	big := []byte(`{"profile":{"pad":"`)
	big = append(big, bytes.Repeat([]byte("x"), 600<<10)...)
	big = append(big, []byte(`"}}`)...)
	response := send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", big, "idem_big")
	payload := readBody(t, response)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized create = %d, want 400: %s", response.StatusCode, payload)
	}
	if code := decodeErrorCode(t, payload); code != "INVALID_REQUEST" {
		t.Fatalf("oversized create error = %v, want INVALID_REQUEST", code)
	}
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_searches`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("oversized create wrote %d rows, want 0", count)
	}
}

// --- Smoke: one named journey covering the whole lifetime loop --------------

// TestSmokeLifetimeSearchJourney walks the #404 acceptance loop in one test so
// scripts/dev/career-smoke.sh can point -run Smoke at a single named journey:
// configure profile -> create search -> worker completes -> results persist ->
// digest enqueued -> status/history readable.
func TestSmokeLifetimeSearchJourney(t *testing.T) {
	recorder := &digestRecorder{}
	server, pool := newCareerServerWithDigest(t, digestWork(), recorder, "https://portal.henukit.cn/career")
	defer server.Close()
	defer pool.Close()

	put := send(t, server.URL, actorA, http.MethodPut, "/api/v1/career/profile", []byte(`{"target_roles":"后端开发","tech_stack":"go","email_notification_enabled":true}`), "")
	if put.StatusCode != http.StatusOK {
		t.Fatalf("smoke profile put = %d", put.StatusCode)
	}

	create := readBody(t, send(t, server.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_smoke"))
	if createStatus := decodeErrorCode(t, create); createStatus != "" {
		t.Fatalf("smoke create failed: %s", create)
	}
	id := decodeData(t, create)["search"].(map[string]any)["id"].(string)

	worker := server.Config.Handler.(*career.Service).Claims()
	if _, err := worker.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	var status string
	var resultCount int
	if err := pool.QueryRow(context.Background(), `SELECT status FROM career_searches WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("smoke status = %s, want completed", status)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_search_results WHERE search_id=$1`, id).Scan(&resultCount); err != nil {
		t.Fatal(err)
	}
	if resultCount != 1 {
		t.Fatalf("smoke result rows = %d, want 1", resultCount)
	}
	if recorder.count() != 1 || recorder.last().SearchID != id {
		t.Fatalf("smoke digest enqueues = %d, want 1 for search %s", recorder.count(), id)
	}
	if statusPayload := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches/"+id, nil, "")))["search"].(map[string]any); statusPayload["status"] != "completed" {
		t.Fatalf("smoke status read = %v", statusPayload["status"])
	}
	if history := decodeData(t, readBody(t, send(t, server.URL, actorA, http.MethodGet, "/api/v1/career/searches", nil, "")))["searches"].([]any); len(history) != 1 {
		t.Fatalf("smoke history len = %d, want 1", len(history))
	}
}

func readStatus(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM career_searches WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

var _ = pgx.ErrNoRows
