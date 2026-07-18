package tests

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/contract"
)

type inboxFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	token  string
	userID uuid.UUID
}

func TestOperationsInboxStoresReferencesWithoutSourceContent(t *testing.T) {
	fixture := newInboxFixture(t)
	forbiddenBody := `{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-1","priority":"high","title":"copied source title"}`
	forbidden := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", forbiddenBody, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_forbidden_content")
	forbidden.Body.Close()
	if forbidden.StatusCode != http.StatusBadRequest {
		t.Fatalf("request containing copied source content = %d, want 400", forbidden.StatusCode)
	}

	body := `{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-1","source_resource_url":"https://quizcraft.henukit.test/admin/feedback/feedback-1","priority":"high","status":"open"}`
	idempotencyKey := "idem_" + uuid.NewString()
	created := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", body, idempotencyKey, "nonce_"+uuid.NewString(), "req_inbox_create_001")
	createdData := decodeInboxItem(t, created, http.StatusCreated)
	if createdData.SourceProductCode != "quizcraft" || createdData.SourceResourceID != "feedback-1" || createdData.Version != 1 {
		t.Fatalf("unexpected created item: %+v", createdData)
	}

	replay := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", body, idempotencyKey, "nonce_"+uuid.NewString(), "req_inbox_create_replay")
	replayedData := decodeInboxItem(t, replay, http.StatusCreated)
	if replayedData.ID != createdData.ID || replayedData.Version != createdData.Version {
		t.Fatalf("idempotent replay changed result: first=%+v replay=%+v", createdData, replayedData)
	}

	conflictingBody := strings.Replace(body, `"priority":"high"`, `"priority":"urgent"`, 1)
	conflict := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", conflictingBody, idempotencyKey, "nonce_"+uuid.NewString(), "req_inbox_idempotency_conflict")
	conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("idempotency key reused for another body = %d, want 409", conflict.StatusCode)
	}

	var itemColumns int
	if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_name = 'operations_inbox_items' AND column_name IN ('title', 'body', 'content', 'feedback_text')`).Scan(&itemColumns); err != nil {
		t.Fatalf("inspect Operations Inbox schema: %v", err)
	}
	if itemColumns != 0 {
		t.Fatalf("Operations Inbox exposes %d source-content columns, want none", itemColumns)
	}
	var items, idempotencyRows, audits int
	if err := fixture.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM operations_inbox_items), (SELECT count(*) FROM operations_inbox_idempotency), (SELECT count(*) FROM operations_inbox_audit_events)`).Scan(&items, &idempotencyRows, &audits); err != nil {
		t.Fatalf("count durable facts: %v", err)
	}
	if items != 1 || idempotencyRows != 1 || audits != 1 {
		t.Fatalf("durable facts = items:%d idempotency:%d audits:%d, want 1/1/1", items, idempotencyRows, audits)
	}
}

func TestOperationsInboxEnforcesProductScopeOnReadsAndWrites(t *testing.T) {
	fixture := newInboxFixture(t)
	body := `{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-scope","priority":"normal"}`
	created := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", body, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_scope_create")
	decodeInboxItem(t, created, http.StatusCreated)

	allowed := sendInboxRequest(t, fixture, http.MethodGet, "/api/v1/operations-inbox/items?product_code=quizcraft", "", "", "nonce_"+uuid.NewString(), "req_inbox_scope_read")
	defer allowed.Body.Close()
	if allowed.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(allowed.Body)
		t.Fatalf("in-scope list = %d: %s", allowed.StatusCode, payload)
	}
	var list struct {
		Data struct {
			Items []contract.OperationsInboxItem `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(allowed.Body).Decode(&list); err != nil || len(list.Data.Items) != 1 {
		t.Fatalf("decode in-scope list: items=%d err=%v", len(list.Data.Items), err)
	}

	denied := sendInboxRequest(t, fixture, http.MethodGet, "/api/v1/operations-inbox/items?product_code=food", "", "", "nonce_"+uuid.NewString(), "req_inbox_scope_escape")
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("out-of-scope list = %d, want 403", denied.StatusCode)
	}
	var decision, reason, product string
	if err := fixture.pool.QueryRow(context.Background(), `SELECT decision, reason_code, target_product_code FROM authorization_audit_events WHERE request_id = 'req_inbox_scope_escape'`).Scan(&decision, &reason, &product); err != nil {
		t.Fatalf("read scope-denial audit: %v", err)
	}
	if decision != "denied" || reason != "PERMISSION_OR_SCOPE_MISSING" || product != "food" {
		t.Fatalf("unexpected scope-denial audit: %s/%s/%s", decision, reason, product)
	}
}

func TestOperationsInboxOptimisticUpdateHasOneWinnerAndImmutableAudit(t *testing.T) {
	fixture := newInboxFixture(t)
	createBody := `{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-race","priority":"normal"}`
	created := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", createBody, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_race_create")
	item := decodeInboxItem(t, created, http.StatusCreated)

	statuses := make(chan int, 2)
	var wait sync.WaitGroup
	for _, priority := range []string{"high", "urgent"} {
		priority := priority
		wait.Add(1)
		go func() {
			defer wait.Done()
			body := fmt.Sprintf(`{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-race","expected_version":1,"priority":%q,"status":"in_progress"}`, priority)
			response := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items/"+item.ID+"/updates", body, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_race_"+priority)
			defer response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	wait.Wait()
	close(statuses)
	successes, conflicts := 0, 0
	for status := range statuses {
		if status == http.StatusOK {
			successes++
		} else if status == http.StatusConflict {
			conflicts++
		} else {
			t.Errorf("unexpected concurrent update status %d", status)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent updates = %d success/%d conflict, want 1/1", successes, conflicts)
	}
	var version int64
	var auditCount int
	if err := fixture.pool.QueryRow(context.Background(), `SELECT version, (SELECT count(*) FROM operations_inbox_audit_events WHERE item_id = operations_inbox_items.id) FROM operations_inbox_items WHERE id = $1`, item.ID).Scan(&version, &auditCount); err != nil {
		t.Fatalf("read final item state: %v", err)
	}
	if version != 2 || auditCount != 2 {
		t.Fatalf("final item version/audits = %d/%d, want 2/2", version, auditCount)
	}
	if _, err := fixture.pool.Exec(context.Background(), `UPDATE operations_inbox_audit_events SET action = 'created' WHERE item_id = $1`, item.ID); err == nil {
		t.Fatal("Operations Inbox audit accepted a mutation")
	}
}

func newInboxFixture(t *testing.T) inboxFixture {
	t.Helper()
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	token := "exchange_inbox_" + uuid.NewString()
	tokenHash := sha256.Sum256([]byte(token))
	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at) SELECT user_id, 'client_exchange', $1, $2, id, now() + interval '5 minutes' FROM sessions WHERE kind = 'core' RETURNING user_id`, tokenHash[:], testClientID).Scan(&userID); err != nil {
		t.Fatalf("seed Inbox exchange Session: %v", err)
	}
	roleID := uuid.New()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO permission_codes (code, description) VALUES ('platform.operations_inbox.read', 'Read inbox'), ('platform.operations_inbox.write', 'Write inbox')`, nil},
		{`INSERT INTO authorization_roles (id, code, display_name) VALUES ($1, 'operations-operator', 'Operations operator')`, []any{roleID}},
		{`INSERT INTO role_permissions (role_id, permission_code) VALUES ($1, 'platform.operations_inbox.read'), ($1, 'platform.operations_inbox.write')`, []any{roleID}},
		{`INSERT INTO user_role_grants (user_id, role_id, scope_kind, product_code) VALUES ($1, $2, 'product', 'quizcraft')`, []any{userID, roleID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed Inbox grants: %v", err)
		}
	}
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), IdempotencyEncryptionKey: testIdempotencyEncryptionKey})
	if err != nil {
		t.Fatalf("create Platform Core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return inboxFixture{pool: pool, server: server, token: token, userID: userID}
}

func sendInboxRequest(t *testing.T, fixture inboxFixture, method, path, body, idempotencyKey, nonce, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, fixture.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create Inbox request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(testClientID, testClientSecret)
	request.Header.Set(contract.ServiceIDHeader, testClientID)
	request.Header.Set(contract.KeyIDHeader, testKeyID)
	request.Header.Set(contract.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
	request.Header.Set(contract.NonceHeader, nonce)
	request.Header.Set(contract.SessionExchangeTokenHeader, fixture.token)
	if idempotencyKey != "" {
		request.Header.Set(contract.IdempotencyKeyHeader, idempotencyKey)
	}
	if requestID != "" {
		request.Header.Set("X-Request-Id", requestID)
	}
	signExchangeRequest(t, request)
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatalf("send Inbox request: %v", err)
	}
	return response
}

func decodeInboxItem(t *testing.T, response *http.Response, wantStatus int) contract.OperationsInboxItem {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != wantStatus {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("Inbox response = %d: %s, want %d", response.StatusCode, payload, wantStatus)
	}
	var envelope contract.SuccessEnvelope[contract.OperationsInboxItem]
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode Inbox item: %v", err)
	}
	return envelope.Data
}
