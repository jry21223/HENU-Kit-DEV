package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	platformcore "henukit.dev/platform-core"
)

func TestAccountLookupFindsAccountByExactEmailAndKeepsOneShapeForMisses(t *testing.T) {
	fixture := newLookupFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	grantPlatformOperations(t, fixture)
	seedLookupIdentity(t, fixture, "operator.target@henu.edu.cn", "目标运营员")

	// Normalization must match the login path: mixed-case input still resolves.
	hit := sendLookupRequest(t, fixture, "Operator.Target@HENU.EDU.CN")
	hitEnvelope := decodeLookupEnvelope(t, hit, http.StatusOK)
	if hitEnvelope.Account == nil {
		t.Fatalf("exact-email lookup returned no account: %s", string(hitEnvelope.Raw))
	}
	if hitEnvelope.Account.ID == "" || hitEnvelope.Account.Status != "active" {
		t.Fatalf("lookup account = %+v, want id and active status", hitEnvelope.Account)
	}
	if hitEnvelope.Account.DisplayName == nil || *hitEnvelope.Account.DisplayName != "目标运营员" {
		t.Fatalf("lookup display_name = %v, want 目标运营员", hitEnvelope.Account.DisplayName)
	}
	if hitEnvelope.Account.Email != "operator.target@henu.edu.cn" {
		t.Fatalf("lookup email = %q, want normalized owner email", hitEnvelope.Account.Email)
	}
	miss := sendLookupRequest(t, fixture, "nobody@henu.edu.cn")
	missEnvelope := decodeLookupEnvelope(t, miss, http.StatusOK)
	if missEnvelope.Account != nil {
		t.Fatalf("unknown-email lookup returned an account: %s", missEnvelope.Raw)
	}
	if !sameEnvelopeShape(hitEnvelope.Raw, missEnvelope.Raw) {
		t.Fatalf("hit/miss response shapes differ:\nhit:  %s\nmiss: %s", hitEnvelope.Raw, missEnvelope.Raw)
	}
	if missEnvelope.RequestID == "" {
		t.Fatal("lookup response omitted request_id")
	}
}

func TestConsoleIdentityBatchUsesTicketPermissionAndOmitsMissingUsers(t *testing.T) {
	fixture := newLookupFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	targetID := seedLookupIdentity(t, fixture, "ticket.owner@henu.edu.cn", "工单用户")
	grantAccountTicketRead(t, fixture)
	missingID := uuid.NewString()
	body := `{"user_ids":["` + targetID + `","` + missingID + `"],"permission_code":"account.tickets.read"}`
	response := sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/console-user-identities/resolutions", body, "", "nonce_"+uuid.NewString(), "req_ticket_identity_batch")
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ticket identity batch = %d: %s, want 200", response.StatusCode, payload)
	}
	if !bytes.Contains(payload, []byte(`"email":"ticket.owner@henu.edu.cn"`)) || !bytes.Contains(payload, []byte(`"display_name":"工单用户"`)) || bytes.Contains(payload, []byte(missingID)) {
		t.Fatalf("ticket identity batch did not return the owner identity or leaked a missing identity: %s", payload)
	}
	platformSnapshot := sendInboxRequest(t, fixture, http.MethodGet, "/api/v1/platform-operations", "", "", "nonce_"+uuid.NewString(), "req_ticket_identity_no_platform_read")
	platformSnapshot.Body.Close()
	if platformSnapshot.StatusCode != http.StatusForbidden {
		t.Fatalf("ticket-only operator unexpectedly gained platform.operations.read: %d", platformSnapshot.StatusCode)
	}
}

func TestAccountLookupNeverLeaksEmailIntoLogsOrAudit(t *testing.T) {
	var logs bytes.Buffer
	fixture := newLookupFixture(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	grantPlatformOperations(t, fixture)
	seedLookupIdentity(t, fixture, "operator.target@henu.edu.cn", "目标运营员")

	response := sendLookupRequest(t, fixture, "operator.target@henu.edu.cn")
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("lookup = %d, want 200", response.StatusCode)
	}
	if bytes.Contains(logs.Bytes(), []byte("operator.target@henu.edu.cn")) {
		t.Fatalf("service logs leaked the email: %s", logs.String())
	}
	var auditText string
	if err := fixture.pool.QueryRow(context.Background(), `SELECT coalesce(string_agg(value::text, ','), '') FROM (SELECT to_jsonb(t)::text AS value FROM authorization_audit_events AS t) AS rows`).Scan(&auditText); err != nil {
		t.Fatalf("read authorization audit: %v", err)
	}
	if strings.Contains(auditText, "operator.target@henu.edu.cn") {
		t.Fatalf("persisted authorization audit leaked the email: %s", auditText)
	}
}

func TestAccountLookupRateLimitsByCallerAndKeepsEmailOutOfRedisKeys(t *testing.T) {
	fixture := newLookupFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	grantPlatformOperations(t, fixture)
	seedLookupIdentity(t, fixture, "operator.target@henu.edu.cn", "目标运营员")

	var last int
	for last = 0; last < 30; last++ {
		response := sendLookupRequest(t, fixture, "operator.target@henu.edu.cn")
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("lookup %d = %d, want 200", last+1, response.StatusCode)
		}
	}
	limited := sendLookupRequest(t, fixture, "operator.target@henu.edu.cn")
	defer limited.Body.Close()
	if limited.StatusCode != http.StatusTooManyRequests {
		payload, _ := io.ReadAll(limited.Body)
		t.Fatalf("rate-limited lookup = %d: %s, want 429", limited.StatusCode, payload)
	}

	// A different caller keeps its own budget: the limit key must be per caller.
	altToken := seedSecondCaller(t, fixture)
	other := sendInboxRequestAs(t, fixture, "console-gateway-alt", "primary", "alt-client-secret-with-enough-entropy", altToken, http.MethodPost, "/api/v1/platform-operations/account-lookups", `{"email":"operator.target@henu.edu.cn"}`, "", "nonce_"+uuid.NewString(), "req_lookup_alt_caller")
	otherEnvelope := decodeLookupEnvelope(t, other, http.StatusOK)
	if otherEnvelope.Account == nil {
		t.Fatalf("second caller lookup was blocked: %s", otherEnvelope.Raw)
	}

	keys, err := fixture.redisClient.Keys(context.Background(), "*").Result()
	if err != nil {
		t.Fatalf("list Redis keys: %v", err)
	}
	for _, key := range keys {
		if strings.Contains(key, "operator.target@henu.edu.cn") {
			t.Fatalf("Redis rate-limit key leaked the email: %q", key)
		}
		if !strings.Contains(key, "account-lookup") {
			continue
		}
		if !strings.Contains(key, "console-gateway") {
			t.Fatalf("rate-limit key is not caller-scoped: %q", key)
		}
	}
}

func TestAccountLookupRejectsNonHenuEmailWithSameErrorPath(t *testing.T) {
	fixture := newLookupFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	grantPlatformOperations(t, fixture)

	for _, email := range []string{"someone@gmail.com", "not-an-email", ""} {
		response := sendLookupRequest(t, fixture, email)
		payload, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("lookup %q = %d: %s, want 400", email, response.StatusCode, payload)
		}
	}
}

func TestAccountLookupRequiresPlatformOperationsRead(t *testing.T) {
	fixture := newLookupFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	// No platform.operations.read grant: the fixture only holds the Inbox product Scope.
	response := sendLookupRequest(t, fixture, "operator.target@henu.edu.cn")
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("ungranted lookup = %d: %s, want 403", response.StatusCode, payload)
	}
}

type lookupEnvelope struct {
	RequestID string `json:"request_id"`
	Raw       []byte
	Account   *struct {
		ID          string  `json:"id"`
		DisplayName *string `json:"display_name"`
		Email       string  `json:"email"`
		Status      string  `json:"status"`
	} `json:"account"`
}

func decodeLookupEnvelope(t *testing.T, response *http.Response, wantStatus int) lookupEnvelope {
	t.Helper()
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != wantStatus {
		t.Fatalf("account lookup = %d: %s, want %d", response.StatusCode, payload, wantStatus)
	}
	var envelope struct {
		Data      lookupEnvelope `json:"data"`
		RequestID string         `json:"request_id"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode account lookup: %v", err)
	}
	envelope.Data.RequestID = envelope.RequestID
	envelope.Data.Raw = payload
	return envelope.Data
}

func sameEnvelopeShape(first, second []byte) bool {
	firstKeys := jsonKeys(first)
	secondKeys := jsonKeys(second)
	if len(firstKeys) != len(secondKeys) {
		return false
	}
	for key := range firstKeys {
		if !secondKeys[key] {
			return false
		}
	}
	return true
}

func jsonKeys(payload []byte) map[string]bool {
	var parsed map[string]any
	_ = json.Unmarshal(payload, &parsed)
	keys := make(map[string]bool, len(parsed))
	for key := range parsed {
		keys[key] = true
	}
	if data, ok := parsed["data"].(map[string]any); ok {
		for key := range data {
			keys["data."+key] = true
		}
	}
	return keys
}

func sendLookupRequest(t *testing.T, fixture inboxFixture, email string) *http.Response {
	t.Helper()
	body := `{"email":` + strconvQuote(email) + `}`
	return sendInboxRequest(t, fixture, http.MethodPost, "/api/v1/platform-operations/account-lookups", body, "", "nonce_"+uuid.NewString(), "req_lookup_"+strings.ReplaceAll(uuid.NewString(), "-", "")[:16])
}

func strconvQuote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func seedLookupIdentity(t *testing.T, fixture inboxFixture, email, displayName string) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New()
	hash := lookupHash(email)
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO users (id, email_verified, status, display_name) VALUES ($1, true, 'active', $2)
	`, userID, displayName); err != nil {
		t.Fatalf("seed lookup user: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO email_identities (user_id, email_lookup_hash, email_ciphertext, verified_at)
		VALUES ($1, $2, $3, now())
	`, userID, hash, sealTestEmail(t, email)); err != nil {
		t.Fatalf("seed lookup identity: %v", err)
	}
	return userID.String()
}

func grantAccountTicketRead(t *testing.T, fixture inboxFixture) {
	t.Helper()
	ctx := context.Background()
	roleID := uuid.New()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO permission_codes (code, description) VALUES ('account.tickets.read', 'Read Account tickets') ON CONFLICT DO NOTHING`, nil},
		{`INSERT INTO authorization_roles (id, code, display_name) VALUES ($1, 'account-ticket-operator', 'Account ticket operator')`, []any{roleID}},
		{`INSERT INTO role_permissions (role_id, permission_code) VALUES ($1, 'account.tickets.read')`, []any{roleID}},
		{`INSERT INTO user_role_grants (user_id, role_id, scope_kind, product_code) VALUES ($1, $2, 'product', 'account-portfolio')`, []any{fixture.userID, roleID}},
	}
	for _, statement := range statements {
		if _, err := fixture.pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("grant Account ticket read: %v", err)
		}
	}
}

func seedSecondCaller(t *testing.T, fixture inboxFixture) string {
	t.Helper()
	ctx := context.Background()
	altSecretHash := sha256.Sum256([]byte("alt-client-secret-with-enough-entropy"))
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ('console-gateway-alt', ARRAY['https://console.henukit.test/auth/callback'])`); err != nil {
		t.Fatalf("seed second caller client: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO oauth_client_keys (client_id, key_id, secret_hash, status) VALUES ('console-gateway-alt', 'primary', $1, 'active')`, altSecretHash[:]); err != nil {
		t.Fatalf("seed second caller key: %v", err)
	}
	token := "exchange_lookup_alt_" + uuid.NewString()
	tokenHash := sha256.Sum256([]byte(token))
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at)
		SELECT user_id, 'client_exchange', $1, 'console-gateway-alt', id, now() + interval '5 minutes'
		FROM sessions WHERE kind = 'core'
	`, tokenHash[:]); err != nil {
		t.Fatalf("seed second caller exchange Session: %v", err)
	}
	return token
}

func lookupHash(email string) []byte {
	mac := hmac.New(sha256.New, testVerificationEncryptionKey)
	_, _ = mac.Write([]byte("henukit-verification:email"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(email))
	return mac.Sum(nil)
}

func newLookupFixture(t *testing.T, logger *slog.Logger) inboxFixture {
	t.Helper()
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	token := "exchange_lookup_" + uuid.NewString()
	tokenHash := sha256.Sum256([]byte(token))
	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at) SELECT user_id, 'client_exchange', $1, $2, id, now() + interval '5 minutes' FROM sessions WHERE kind = 'core' RETURNING user_id`, tokenHash[:], testClientID).Scan(&userID); err != nil {
		t.Fatalf("seed lookup exchange Session: %v", err)
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
			t.Fatalf("seed lookup grants: %v", err)
		}
	}
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient, Logger: logger, IdempotencyEncryptionKey: testIdempotencyEncryptionKey, VerificationEncryptionKey: testVerificationEncryptionKey})
	if err != nil {
		t.Fatalf("create Platform Core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return inboxFixture{pool: pool, server: server, token: token, userID: userID, redisClient: redisClient}
}
