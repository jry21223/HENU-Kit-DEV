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
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	library "henukit.dev/library"
)

const serviceSecret = "library-gateway-secret-at-least-32-bytes"

func TestBoundedWorkspaceDegradesWithoutLeakingLegacyDomains(t *testing.T) {
	legacy := newLegacyServer(t)
	defer legacy.Close()
	server, pool := newLibraryServer(t, legacy.URL)
	defer server.Close()
	defer pool.Close()

	response := send(t, server.URL, "library.read", http.MethodGet, "/api/v1/workspace", nil, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("workspace status = %d", response.StatusCode)
	}
	body := readBody(t, response)
	for _, expected := range []string{`"courses"`, `"materials"`, `"downloads"`, `"submissions"`, `"corrections"`, `"status":"partial"`, `"degraded":true`} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("workspace omitted %s: %s", expected, body)
		}
	}
	for _, forbidden := range []string{"payment", "membership", "member_only", "paid", "points", "forum", "quiz", "userAgent", "127.0.0.1"} {
		if bytes.Contains(bytes.ToLower(body), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("workspace leaked %q: %s", forbidden, body)
		}
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE library_adapter_audit_events"); err == nil {
		t.Fatal("append-only audit accepted TRUNCATE")
	}
}

func TestScopedIdempotentReviewUsesOptimisticVersionAndAudit(t *testing.T) {
	legacy := newLegacyServer(t)
	defer legacy.Close()
	server, pool := newLibraryServer(t, legacy.URL)
	defer server.Close()
	defer pool.Close()

	body := []byte(`{"kind":"submission_approve","resource_id":"22222222-2222-4222-8222-222222222222","expected_version":"2026-07-19T00:00:00.123Z","payload":{"reviewReason":"checked"}}`)
	reviewKey := "idem_library_review_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	responses := make([][]byte, 2)
	var wait sync.WaitGroup
	for index := range responses {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := send(t, server.URL, "library.review", http.MethodPost, "/api/v1/commands", body, reviewKey)
			responses[index] = readBody(t, response)
		}()
	}
	wait.Wait()
	if fmt.Sprint(decodeData(t, responses[0])) != fmt.Sprint(decodeData(t, responses[1])) {
		t.Fatalf("idempotent result data differs: %s != %s", responses[0], responses[1])
	}
	if !bytes.Contains(responses[0], []byte(`"state":"succeeded"`)) {
		t.Fatalf("review did not succeed: %s", responses[0])
	}

	conflictKey := "idem_library_conflict_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	conflict := send(t, server.URL, "library.review", http.MethodPost, "/api/v1/commands", []byte(strings.ReplaceAll(string(body), "2026-07-19T00:00:00.123Z", "2026-07-18T00:00:00Z")), conflictKey)
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("stale review status = %d", conflict.StatusCode)
	}

	operation := send(t, server.URL, "library.review", http.MethodGet, "/api/v1/operations/submission_approve", nil, reviewKey)
	if payload := readBody(t, operation); !bytes.Contains(payload, []byte(`"state":"succeeded"`)) {
		t.Fatalf("operation lookup = %s", payload)
	}
	otherActor := sendAs(t, server.URL, "88888888-8888-4888-8888-888888888888", "library.review", http.MethodGet, "/api/v1/operations/submission_approve", nil, reviewKey)
	if payload := readBody(t, otherActor); !bytes.Contains(payload, []byte(`"state":"unknown"`)) {
		t.Fatalf("cross-actor operation lookup leaked result: %s", payload)
	}
	var auditCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM library_adapter_audit_events a JOIN library_adapter_operations o ON o.id=a.operation_id WHERE a.action='submission_approve' AND a.outcome='succeeded' AND o.idempotency_key=$1`, reviewKey).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit count = %d, %v", auditCount, err)
	}
}

func TestScopeAndCommandAllowlistDefaultDeny(t *testing.T) {
	legacy := newLegacyServer(t)
	defer legacy.Close()
	server, pool := newLibraryServer(t, legacy.URL)
	defer server.Close()
	defer pool.Close()

	denied := send(t, server.URL, "notice.read", http.MethodGet, "/api/v1/workspace", nil, "")
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign scope status = %d", denied.StatusCode)
	}
	command := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"payment_refund","resource_id":"x","expected_version":"v1","payload":{}}`), "idem_forbidden_command")
	if command.StatusCode != http.StatusBadRequest {
		t.Fatalf("forbidden command status = %d", command.StatusCode)
	}
	commercial := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"material_update","resource_id":"22222222-2222-4222-8222-222222222222","expected_version":"2026-07-19T00:00:00Z","payload":{"accessLevel":"member_only"}}`), "idem_forbidden_access")
	if commercial.StatusCode != http.StatusBadRequest {
		t.Fatalf("commercial access level status = %d", commercial.StatusCode)
	}
	missingCreate := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"course_create","payload":{}}`), "idem_invalid_create_missing")
	if missingCreate.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing create fields status = %d", missingCreate.StatusCode)
	}
	invalidCreate := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", []byte(`{"kind":"course_create","payload":{"schoolId":"not-a-uuid","collegeId":"66666666-6666-4666-8666-666666666666","majorId":"77777777-7777-4777-8777-777777777777","grade":"2025","name":"线性代数","slug":"linear-algebra"}}`), "idem_invalid_create_uuid")
	if invalidCreate.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid create UUID status = %d", invalidCreate.StatusCode)
	}
}

func TestCreateReturnsLegacyResourceIDAndAuditsTarget(t *testing.T) {
	legacy := newLegacyServer(t)
	defer legacy.Close()
	server, pool := newLibraryServer(t, legacy.URL)
	defer server.Close()
	defer pool.Close()

	key := "idem_library_create_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	body := []byte(`{"kind":"course_create","payload":{"schoolId":"55555555-5555-4555-8555-555555555555","collegeId":"66666666-6666-4666-8666-666666666666","majorId":"77777777-7777-4777-8777-777777777777","name":"线性代数","slug":"linear-algebra","grade":"2025","status":"draft"}}`)
	response := send(t, server.URL, "library.manage", http.MethodPost, "/api/v1/commands", body, key)
	payload := readBody(t, response)
	if !bytes.Contains(payload, []byte(`"resource_id":"44444444-4444-4444-8444-444444444444"`)) || !bytes.Contains(payload, []byte(`"state":"succeeded"`)) {
		t.Fatalf("create result = %s", payload)
	}
	operation := send(t, server.URL, "library.manage", http.MethodGet, "/api/v1/operations/course_create", nil, key)
	if replay := readBody(t, operation); !bytes.Contains(replay, []byte(`"resource_id":"44444444-4444-4444-8444-444444444444"`)) {
		t.Fatalf("create operation lookup = %s", replay)
	}
	var targetID string
	if err := pool.QueryRow(context.Background(), `SELECT target_id FROM library_adapter_operations WHERE idempotency_key=$1`, key).Scan(&targetID); err != nil || targetID != "44444444-4444-4444-8444-444444444444" {
		t.Fatalf("create target = %q, %v", targetID, err)
	}
}

func TestUnknownLegacyWriteIsQueryableAndNeverBlindlyRetried(t *testing.T) {
	var commandCalls atomic.Int32
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			commandCalls.Add(1)
			panic(http.ErrAbortHandler)
		}
		switch r.URL.Path {
		case "/api/v1/admin/courses":
			_, _ = io.WriteString(w, `{"data":{"courses":[]}}`)
		case "/api/v1/admin/materials", "/api/v1/admin/material-reviews":
			_, _ = io.WriteString(w, `{"data":{"materials":[{"id":"22222222-2222-4222-8222-222222222222","courseId":"11111111-1111-4111-8111-111111111111","title":"期末复习提纲","type":"quick_review","fileName":"review.pdf","fileSize":2048,"accessLevel":"login_required","status":"pending","updatedAt":"2026-07-19T00:00:00Z"}]}}`)
		case "/api/v1/admin/downloads":
			_, _ = io.WriteString(w, `{"data":{"downloads":[]}}`)
		case "/api/v1/admin/reports":
			_, _ = io.WriteString(w, `{"data":{"reports":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer legacy.Close()
	server, pool := newLibraryServer(t, legacy.URL)
	defer server.Close()
	defer pool.Close()
	body := []byte(`{"kind":"submission_approve","resource_id":"22222222-2222-4222-8222-222222222222","expected_version":"2026-07-19T00:00:00Z","payload":{}}`)
	unknownKey := "idem_library_unknown_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	first := send(t, server.URL, "library.review", http.MethodPost, "/api/v1/commands", body, unknownKey)
	firstPayload, _ := io.ReadAll(first.Body)
	first.Body.Close()
	if first.StatusCode != http.StatusServiceUnavailable || !bytes.Contains(firstPayload, []byte("OPERATION_RESULT_UNKNOWN")) {
		t.Fatalf("unknown first result = %d %s", first.StatusCode, firstPayload)
	}
	second := send(t, server.URL, "library.review", http.MethodPost, "/api/v1/commands", body, unknownKey)
	if payload := readBody(t, second); !bytes.Contains(payload, []byte(`"state":"unknown"`)) {
		t.Fatalf("replayed unknown = %s", payload)
	}
	if commandCalls.Load() != 1 {
		t.Fatalf("unknown write called legacy %d times", commandCalls.Load())
	}
	operation := send(t, server.URL, "library.review", http.MethodGet, "/api/v1/operations/submission_approve", nil, unknownKey)
	if payload := readBody(t, operation); !bytes.Contains(payload, []byte(`"state":"unknown"`)) {
		t.Fatalf("unknown lookup = %s", payload)
	}
	var targetType, targetID, outcome string
	if err := pool.QueryRow(context.Background(), `SELECT o.target_type,o.target_id,a.outcome FROM library_adapter_operations o JOIN library_adapter_audit_events a ON a.operation_id=o.id WHERE o.idempotency_key=$1 ORDER BY a.created_at DESC LIMIT 1`, unknownKey).Scan(&targetType, &targetID, &outcome); err != nil {
		t.Fatal(err)
	}
	if targetType != "material" || targetID != "22222222-2222-4222-8222-222222222222" || outcome != "unknown" {
		t.Fatalf("unknown audit = %s %s %s", targetType, targetID, outcome)
	}
}

func newLibraryServer(t *testing.T, legacyURL string) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("LIBRARY_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("LIBRARY_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := library.New(library.Config{Database: pool, Redis: redisClient, ClientID: "console-gateway", Keys: map[string]string{"active": serviceSecret}, LegacyBaseURL: legacyURL, LegacyToken: "legacy-admin-token", HTTPClient: http.DefaultClient})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() { _ = redisClient.Close() })
	return server, pool
}

func newLegacyServer(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	approved := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer legacy-admin-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/courses":
			_, _ = io.WriteString(w, `{"data":{"courses":[{"id":"11111111-1111-4111-8111-111111111111","name":"高等数学","slug":"math","grade":"2025","status":"published","updatedAt":"2026-07-19T00:00:00Z"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/materials":
			_, _ = io.WriteString(w, `{"data":{"materials":[]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/material-reviews":
			status, updatedAt := "pending", "2026-07-19T00:00:00.123Z"
			if approved {
				status, updatedAt = "published", "2026-07-19T00:01:00Z"
			}
			_, _ = fmt.Fprintf(w, `{"data":{"materials":[{"id":"22222222-2222-4222-8222-222222222222","courseId":"11111111-1111-4111-8111-111111111111","title":"期末复习提纲","type":"quick_review","fileName":"review.pdf","fileSize":2048,"accessLevel":"login_required","status":%q,"createdBy":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","updatedAt":%q}]}}`, status, updatedAt)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/downloads":
			_, _ = io.WriteString(w, `{"data":{"downloads":[{"id":"33333333-3333-4333-8333-333333333333","userId":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","materialId":"22222222-2222-4222-8222-222222222222","accessLevel":"member_only","ip":"127.0.0.1","userAgent":"secret-agent","downloadedAt":"2026-07-19T01:00:00Z","material":{"title":"期末复习提纲"}}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/reports":
			http.Error(w, "legacy unavailable", http.StatusServiceUnavailable)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/materials/22222222-2222-4222-8222-222222222222/approve":
			approved = true
			_, _ = io.WriteString(w, `{"data":{"reviewed":true,"status":"published"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/courses":
			_, _ = io.WriteString(w, `{"data":{"course":{"id":"44444444-4444-4444-8444-444444444444"}}}`)
		default:
			t.Fatalf("unexpected legacy request: %s %s", r.Method, r.URL.RequestURI())
		}
	}))
}

func send(t *testing.T, baseURL, permission, method, path string, body []byte, key string) *http.Response {
	return sendAs(t, baseURL, "99999999-9999-4999-8999-999999999999", permission, method, path, body, key)
}

func sendAs(t *testing.T, baseURL, actorID, permission, method, path string, body []byte, key string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Id", "console-gateway")
	request.Header.Set("X-Key-Id", "active")
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Permission-Code", permission)
	request.Header.Set("X-Scope-Kind", "product")
	request.Header.Set("X-Product-Code", "library")
	request.Header.Set("X-Request-Id", "req_library_test_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(serviceSecret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.SetBasicAuth("console-gateway", serviceSecret)
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
	if response.StatusCode >= 500 {
		t.Fatalf("server error %d: %s", response.StatusCode, body)
	}
	return body
}

func decodeData(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(fmt.Errorf("decode %s: %w", payload, err))
	}
	return envelope.Data
}
