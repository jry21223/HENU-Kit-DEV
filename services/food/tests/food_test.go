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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	food "henukit.dev/food"
)

const serviceSecret = "food-gateway-secret-at-least-32-bytes"

var (
	submissionID = "11111111-1111-4111-8111-111111111111"
	anomalyID    = "22222222-2222-4222-8222-222222222222"
	tierID       = "33333333-3333-4333-8333-333333333333"
)

func TestWorkspaceIsBoundedAndRepresentsStaleOwnerData(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	seedWorkspace(t, pool)

	response := send(t, server.URL, "food.read", http.MethodGet, "/api/v1/workspace", nil, "")
	payload := readBody(t, response)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("workspace status = %d: %s", response.StatusCode, payload)
	}
	for _, expected := range []string{`"status":"ok"`, `"submissions"`, `"anomaly_tickets"`, `"tier_adjustments"`, `"venue_name":"北苑餐厅"`} {
		if !bytes.Contains(payload, []byte(expected)) {
			t.Fatalf("workspace omitted %s: %s", expected, payload)
		}
	}
	for _, forbidden := range []string{"actor_user_id", "review_note", "request_hash", "idempotency_key", "payment", "credential"} {
		if bytes.Contains(bytes.ToLower(payload), []byte(forbidden)) {
			t.Fatalf("workspace leaked %q: %s", forbidden, payload)
		}
	}
	if _, err := pool.Exec(context.Background(), `ALTER TABLE food_submissions RENAME TO food_submissions_unavailable`); err != nil {
		t.Fatal(err)
	}
	stale := readBody(t, send(t, server.URL, "food.read", http.MethodGet, "/api/v1/workspace", nil, ""))
	if _, err := pool.Exec(context.Background(), `ALTER TABLE food_submissions_unavailable RENAME TO food_submissions`); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stale, []byte(`"status":"stale"`)) || !bytes.Contains(stale, []byte(`"stale":true`)) {
		t.Fatalf("stale state not represented: %s", stale)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE food_audit_events"); err == nil {
		t.Fatal("append-only audit accepted TRUNCATE")
	}
}

func TestSummaryCountsAllPendingRowsBeyondWorkspaceLimit(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	seedWorkspace(t, pool)
	if _, err := pool.Exec(context.Background(), `DELETE FROM food_submissions WHERE description='summary-count-fixture'`); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 205; index++ {
		if _, err := pool.Exec(context.Background(), `INSERT INTO food_submissions(id,venue_name,item_name,description,status,version,submitted_at) VALUES($1,'南苑餐厅','测试菜品','summary-count-fixture','pending',1,now()-interval '1 day')`, uuid.New()); err != nil {
			t.Fatal(err)
		}
	}
	payload := readBody(t, send(t, server.URL, "food.read", http.MethodGet, "/api/v1/console-summary", nil, ""))
	if !bytes.Contains(payload, []byte(`"label":"待审核投稿","value":"206"`)) {
		t.Fatalf("summary truncated pending count: %s", payload)
	}
}

func TestThreeOperationClassesAreScopedVersionedIdempotentAndAudited(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	seedWorkspace(t, pool)

	tests := []struct {
		kind, resourceID, permission, table, finalStatus string
	}{
		{"submission_approve", submissionID, "food.review", "food_submissions", "approved"},
		{"anomaly_resolve", anomalyID, "food.anomaly", "food_anomaly_tickets", "resolved"},
		{"tier_adjustment_confirm", tierID, "food.tier_adjust", "food_tier_adjustments", "confirmed"},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"kind":%q,"resource_id":%q,"expected_version":1,"payload":{"note":"已核验"}}`, test.kind, test.resourceID))
			key := "idem_food_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			responses := make([][]byte, 2)
			statuses := make([]int, 2)
			var wait sync.WaitGroup
			for index := range responses {
				wait.Add(1)
				go func() {
					defer wait.Done()
					response := send(t, server.URL, test.permission, http.MethodPost, "/api/v1/commands", body, key)
					statuses[index] = response.StatusCode
					responses[index] = readBody(t, response)
				}()
			}
			wait.Wait()
			if statuses[0] != http.StatusOK || statuses[1] != http.StatusOK || fmt.Sprint(decodeData(t, responses[0])) != fmt.Sprint(decodeData(t, responses[1])) {
				t.Fatalf("idempotent results differ: %d %s / %d %s", statuses[0], responses[0], statuses[1], responses[1])
			}
			if !bytes.Contains(responses[0], []byte(`"state":"succeeded"`)) || !bytes.Contains(responses[0], []byte(`"version":2`)) {
				t.Fatalf("command result = %s", responses[0])
			}
			var status string
			var version, successAudits int
			if err := pool.QueryRow(context.Background(), fmt.Sprintf(`SELECT status,version FROM %s WHERE id=$1`, test.table), test.resourceID).Scan(&status, &version); err != nil {
				t.Fatal(err)
			}
			if status != test.finalStatus || version != 2 {
				t.Fatalf("resource = %s v%d", status, version)
			}
			if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_audit_events a JOIN food_operations o ON o.id=a.operation_id WHERE o.idempotency_key=$1 AND a.outcome='succeeded'`, key).Scan(&successAudits); err != nil || successAudits != 1 {
				t.Fatalf("success audits = %d, %v", successAudits, err)
			}
			lookup := readBody(t, send(t, server.URL, test.permission, http.MethodGet, "/api/v1/operations/"+test.kind, nil, key))
			if !bytes.Contains(lookup, []byte(`"state":"succeeded"`)) {
				t.Fatalf("operation lookup = %s", lookup)
			}
			otherActor := readBody(t, sendAs(t, server.URL, "88888888-8888-4888-8888-888888888888", test.permission, http.MethodGet, "/api/v1/operations/"+test.kind, nil, key))
			if !bytes.Contains(otherActor, []byte(`"state":"unknown"`)) {
				t.Fatalf("cross-actor lookup leaked result: %s", otherActor)
			}
		})
	}
}

func TestCommandsDefaultDenyInvalidScopePayloadAndStaleVersion(t *testing.T) {
	server, pool := newFoodServer(t)
	defer server.Close()
	defer pool.Close()
	seedWorkspace(t, pool)

	body := []byte(fmt.Sprintf(`{"kind":"submission_reject","resource_id":%q,"expected_version":1,"payload":{"note":"信息不足"}}`, submissionID))
	wrongPermission := send(t, server.URL, "food.anomaly", http.MethodPost, "/api/v1/commands", body, "idem_wrong_permission")
	if wrongPermission.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong permission status = %d", wrongPermission.StatusCode)
	}
	invalid := send(t, server.URL, "food.review", http.MethodPost, "/api/v1/commands", []byte(fmt.Sprintf(`{"kind":"submission_reject","resource_id":%q,"expected_version":1,"payload":{"paymentId":"hidden"}}`, submissionID)), "idem_invalid_payload")
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid payload status = %d", invalid.StatusCode)
	}
	stale := send(t, server.URL, "food.review", http.MethodPost, "/api/v1/commands", []byte(fmt.Sprintf(`{"kind":"submission_reject","resource_id":%q,"expected_version":99,"payload":{"note":"信息不足"}}`, submissionID)), "idem_stale_version")
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale version status = %d", stale.StatusCode)
	}
	var failedAudits int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM food_audit_events a JOIN food_operations o ON o.id=a.operation_id WHERE o.idempotency_key='idem_stale_version' AND a.outcome='failed'`).Scan(&failedAudits); err != nil || failedAudits != 1 {
		t.Fatalf("stale audit count = %d, %v", failedAudits, err)
	}
	unknown := readBody(t, send(t, server.URL, "food.review", http.MethodGet, "/api/v1/operations/submission_approve", nil, "idem_missing_operation"))
	if !bytes.Contains(unknown, []byte(`"state":"unknown"`)) {
		t.Fatalf("missing operation = %s", unknown)
	}
}

func newFoodServer(t *testing.T) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("FOOD_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("FOOD_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := food.New(food.Config{Database: pool, Redis: redisClient, ClientID: "console-gateway", Keys: map[string]string{"active": serviceSecret}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })
	return httptest.NewServer(handler), pool
}

func seedWorkspace(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`INSERT INTO food_submissions(id,venue_name,item_name,description,status,version,submitted_at) VALUES('11111111-1111-4111-8111-111111111111','北苑餐厅','胡辣汤','早餐窗口','pending',1,now()) ON CONFLICT(id) DO UPDATE SET status='pending',version=1,updated_at=now()`,
		`INSERT INTO food_anomaly_tickets(id,venue_name,kind,details,severity,status,version) VALUES('22222222-2222-4222-8222-222222222222','北苑餐厅','duplicate','重复地点','medium','open',1) ON CONFLICT(id) DO UPDATE SET status='open',version=1,updated_at=now()`,
		`INSERT INTO food_tier_adjustments(id,venue_name,current_tier,proposed_tier,reason,status,version) VALUES('33333333-3333-4333-8333-333333333333','北苑餐厅','standard','recommended','近期评分稳定','pending',1) ON CONFLICT(id) DO UPDATE SET status='pending',version=1,updated_at=now()`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
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
	request.Header.Set("X-Product-Code", "food")
	request.Header.Set("X-Request-Id", "req_food_test_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
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
