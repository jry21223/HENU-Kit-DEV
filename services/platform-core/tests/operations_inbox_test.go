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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"gopkg.in/yaml.v3"

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

func TestOperationsInboxOpenAPIExamplesConformToHTTP(t *testing.T) {
	fixture := newInboxFixture(t)
	examples := loadInboxContractExamples(t)
	createBody, err := json.Marshal(examples.Create.Example)
	if err != nil {
		t.Fatalf("marshal OpenAPI create example: %v", err)
	}
	createKey := "idem_" + uuid.NewString()
	created := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", string(createBody), createKey, "nonce_"+uuid.NewString(), "req_inbox_contract_create")
	item := decodeInboxItem(t, created, http.StatusCreated)

	statusPath := operationStatusPath("create", item)
	resolved := sendInboxRequest(t, fixture, http.MethodGet, statusPath, "", createKey, "nonce_"+uuid.NewString(), "req_inbox_operation_resolved")
	resolvedStatus := decodeOperationStatus(t, resolved)
	if resolvedStatus.Status != "succeeded" || resolvedStatus.Item == nil || resolvedStatus.Item.ID != item.ID {
		t.Fatalf("resolved operation status = %+v, want succeeded item %s", resolvedStatus, item.ID)
	}
	unknownKey := "idem_" + uuid.NewString()
	unknown := sendInboxRequest(t, fixture, http.MethodGet, operationStatusPath("create", item), "", unknownKey, "nonce_"+uuid.NewString(), "req_inbox_operation_unknown")
	unknownStatus := decodeOperationStatus(t, unknown)
	if unknownStatus.Status != "unknown" || unknownStatus.Item != nil {
		t.Fatalf("unknown operation status = %+v", unknownStatus)
	}

	updateBody, err := json.Marshal(examples.Update.Example)
	if err != nil {
		t.Fatalf("marshal OpenAPI update example: %v", err)
	}
	updated := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items/"+item.ID+"/updates", string(updateBody), "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_contract_update")
	decodeInboxItem(t, updated, http.StatusOK)

	for name, example := range examples.Create.Invalid {
		body, _ := json.Marshal(example)
		response := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", string(body), "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_invalid_create_"+name)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("OpenAPI invalid create example %q = %d, want 400", name, response.StatusCode)
		}
	}
	for name, example := range examples.Update.Invalid {
		body, _ := json.Marshal(example)
		response := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items/"+item.ID+"/updates", string(body), "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_invalid_update_"+name)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("OpenAPI invalid update example %q = %d, want 400", name, response.StatusCode)
		}
	}
}

func TestOperationsInboxEnforcesProductScopeOnReadsAndWrites(t *testing.T) {
	fixture := newInboxFixture(t)
	body := `{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-scope","priority":"normal"}`
	created := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", body, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_scope_create")
	item := decodeInboxItem(t, created, http.StatusCreated)

	deniedCreateBody := `{"source_product_code":"food","source_resource_type":"feedback","source_resource_id":"feedback-out-of-scope","priority":"normal"}`
	deniedCreate := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", deniedCreateBody, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_scope_create_escape")
	deniedCreate.Body.Close()
	if deniedCreate.StatusCode != http.StatusForbidden {
		t.Fatalf("out-of-scope create = %d, want 403", deniedCreate.StatusCode)
	}
	deniedUpdateBody := `{"source_product_code":"food","source_resource_type":"feedback","source_resource_id":"feedback-out-of-scope","expected_version":1,"priority":"high"}`
	deniedUpdate := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items/"+item.ID+"/updates", deniedUpdateBody, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_inbox_scope_update_escape")
	deniedUpdate.Body.Close()
	if deniedUpdate.StatusCode != http.StatusForbidden {
		t.Fatalf("out-of-scope update = %d, want 403", deniedUpdate.StatusCode)
	}
	var itemCount int
	var version int64
	if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*), max(version) FROM operations_inbox_items`).Scan(&itemCount, &version); err != nil || itemCount != 1 || version != 1 {
		t.Fatalf("denied writes changed durable state: items=%d version=%d err=%v", itemCount, version, err)
	}

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
	if _, err := fixture.pool.Exec(context.Background(), `
		UPDATE user_role_grants
		SET scope_kind = 'resource', resource_type = 'feedback', resource_id = 'feedback-scope'
		WHERE user_id = $1`, fixture.userID); err != nil {
		t.Fatalf("narrow operator grant to one resource: %v", err)
	}
	resourceRead := sendInboxRequest(t, fixture, http.MethodGet, itemReadPath(item, "feedback-scope"), "", "", "nonce_"+uuid.NewString(), "req_inbox_resource_read")
	readItem := decodeInboxItem(t, resourceRead, http.StatusOK)
	if readItem.ID != item.ID {
		t.Fatalf("resource-scoped read returned %s, want %s", readItem.ID, item.ID)
	}
	adjacentRead := sendInboxRequest(t, fixture, http.MethodGet, itemReadPath(item, "feedback-neighbor"), "", "", "nonce_"+uuid.NewString(), "req_inbox_resource_read_escape")
	adjacentRead.Body.Close()
	if adjacentRead.StatusCode != http.StatusForbidden {
		t.Fatalf("adjacent resource read = %d, want 403", adjacentRead.StatusCode)
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

func TestOperationsInboxListPaginatesWithoutDroppingItems(t *testing.T) {
	fixture := newInboxFixture(t)
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO operations_inbox_items (source_product_code, source_resource_type, source_resource_id, priority, created_by, updated_by)
		SELECT 'quizcraft', 'feedback', 'feedback-page-' || value, 'normal', $1, $1
		FROM generate_series(1, 25) AS value`, fixture.userID); err != nil {
		t.Fatalf("seed paginated inbox: %v", err)
	}
	decodePage := func(path string) (int, bool) {
		response := sendInboxRequest(t, fixture, http.MethodGet, path, "", "", "nonce_"+uuid.NewString(), "req_inbox_page_"+uuid.NewString())
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(response.Body)
			t.Fatalf("list page = %d: %s", response.StatusCode, payload)
		}
		var envelope struct {
			Data struct {
				Items    []contract.OperationsInboxItem `json:"items"`
				Page     int                            `json:"page"`
				PageSize int                            `json:"page_size"`
				HasMore  bool                           `json:"has_more"`
			} `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode list page: %v", err)
		}
		return len(envelope.Data.Items), envelope.Data.HasMore
	}
	if count, hasMore := decodePage("/api/v1/operations-inbox/items?product_code=quizcraft"); count != 20 || !hasMore {
		t.Fatalf("default page = %d items has_more=%t, want 20/true", count, hasMore)
	}
	if count, hasMore := decodePage("/api/v1/operations-inbox/items?product_code=quizcraft&page=2&page_size=20"); count != 5 || hasMore {
		t.Fatalf("second page = %d items has_more=%t, want 5/false", count, hasMore)
	}
}

func TestOperationsInboxIdempotencyIsScopedByCallingService(t *testing.T) {
	fixture := newInboxFixture(t)
	const secondClientID = "second-gateway"
	const secondKeyID = "second-key"
	const secondSecret = "second-test-client-secret-with-enough-entropy"
	secondToken := "exchange_second_" + uuid.NewString()
	secretHash := sha256.Sum256([]byte(secondSecret))
	tokenHash := sha256.Sum256([]byte(secondToken))
	ctx := context.Background()
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ($1, ARRAY['https://second.example/callback'])`, secondClientID); err != nil {
		t.Fatalf("seed second OAuth client: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO oauth_client_keys (client_id, key_id, secret_hash, status) VALUES ($1, $2, $3, 'active')`, secondClientID, secondKeyID, secretHash[:]); err != nil {
		t.Fatalf("seed second client key: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at)
		SELECT $1, 'client_exchange', $2, $3, id, now() + interval '5 minutes'
		FROM sessions WHERE kind = 'core' AND user_id = $1`, fixture.userID, tokenHash[:], secondClientID); err != nil {
		t.Fatalf("seed second exchange session: %v", err)
	}
	key := "idem_" + uuid.NewString()
	firstBody := `{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-service-a","priority":"normal"}`
	secondBody := `{"source_product_code":"quizcraft","source_resource_type":"feedback","source_resource_id":"feedback-service-b","priority":"normal"}`
	decodeInboxItem(t, sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/operations-inbox/items", firstBody, key, "nonce_"+uuid.NewString(), "req_inbox_service_a"), http.StatusCreated)
	decodeInboxItem(t, sendInboxRequestAs(t, fixture, secondClientID, secondKeyID, secondSecret, secondToken, http.MethodPost, "/api/v1/operations-inbox/items", secondBody, key, "nonce_"+uuid.NewString(), "req_inbox_service_b"), http.StatusCreated)
	var rows int
	if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM operations_inbox_idempotency WHERE idempotency_key = $1`, key).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("service-scoped idempotency rows = %d, want 2 (err=%v)", rows, err)
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
	rows, err := fixture.pool.Query(context.Background(), `SELECT item_snapshot FROM operations_inbox_audit_events WHERE item_id = $1 ORDER BY to_version`, item.ID)
	if err != nil {
		t.Fatalf("read reconstructable audits: %v", err)
	}
	defer rows.Close()
	var snapshots []map[string]any
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan audit snapshot: %v", err)
		}
		var snapshot map[string]any
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			t.Fatalf("decode audit snapshot: %v", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if len(snapshots) != 2 || snapshots[0]["priority"] != "normal" || snapshots[0]["status"] != "open" || snapshots[0]["version"] != float64(1) || snapshots[1]["status"] != "in_progress" || snapshots[1]["version"] != float64(2) {
		t.Fatalf("audit snapshots do not reconstruct state changes: %#v", snapshots)
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
	return sendInboxRequestAs(t, fixture, testClientID, testKeyID, testClientSecret, fixture.token, method, path, body, idempotencyKey, nonce, requestID)
}

func sendInboxRequestAs(t *testing.T, fixture inboxFixture, clientID, keyID, secret, token, method, path, body, idempotencyKey, nonce, requestID string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, fixture.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create Inbox request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(clientID, secret)
	request.Header.Set(contract.ServiceIDHeader, clientID)
	request.Header.Set(contract.KeyIDHeader, keyID)
	request.Header.Set(contract.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
	request.Header.Set(contract.NonceHeader, nonce)
	request.Header.Set(contract.SessionExchangeTokenHeader, token)
	if idempotencyKey != "" {
		request.Header.Set(contract.IdempotencyKeyHeader, idempotencyKey)
	}
	if requestID != "" {
		request.Header.Set("X-Request-Id", requestID)
	}
	signExchangeRequestWithSecret(t, request, secret)
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

func decodeOperationStatus(t *testing.T, response *http.Response) contract.OperationsInboxOperationStatus {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("operation status = %d: %s, want 200", response.StatusCode, payload)
	}
	var envelope contract.SuccessEnvelope[contract.OperationsInboxOperationStatus]
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode operation status: %v", err)
	}
	return envelope.Data
}

func operationStatusPath(operation string, item contract.OperationsInboxItem) string {
	query := url.Values{
		"source_product_code":  {item.SourceProductCode},
		"source_resource_type": {item.SourceResourceType},
		"source_resource_id":   {item.SourceResourceID},
	}
	return "/api/v1/operations-inbox/operations/" + operation + "?" + query.Encode()
}

func itemReadPath(item contract.OperationsInboxItem, resourceID string) string {
	query := url.Values{
		"source_product_code":  {item.SourceProductCode},
		"source_resource_type": {item.SourceResourceType},
		"source_resource_id":   {resourceID},
	}
	return "/api/v1/operations-inbox/items/" + item.ID + "?" + query.Encode()
}

type inboxContractExamples struct {
	Create inboxSchemaExamples
	Update inboxSchemaExamples
}

type inboxSchemaExamples struct {
	Example map[string]any            `yaml:"example"`
	Invalid map[string]map[string]any `yaml:"x-invalid-examples"`
}

func loadInboxContractExamples(t *testing.T) inboxContractExamples {
	t.Helper()
	path := filepath.Join("..", "..", "..", "packages", "api-contracts", "openapi", "platform-core.yaml")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var document struct {
		Components struct {
			Schemas map[string]inboxSchemaExamples `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(source, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	result := inboxContractExamples{Create: document.Components.Schemas["CreateOperationsInboxItemRequest"], Update: document.Components.Schemas["UpdateOperationsInboxItemRequest"]}
	if len(result.Create.Example) == 0 || len(result.Create.Invalid) == 0 || len(result.Update.Example) == 0 || len(result.Update.Invalid) == 0 {
		t.Fatal("Operations Inbox OpenAPI examples are incomplete")
	}
	return result
}
