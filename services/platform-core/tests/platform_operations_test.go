package tests

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPlatformOperationsSessionRevocationIsIdempotentAuditedAndQueryable(t *testing.T) {
	fixture := newInboxFixture(t)
	grantPlatformOperations(t, fixture)
	targetUserID := uuid.New()
	targetSessionID := uuid.New()
	tokenHash := sha256.Sum256([]byte("target-session-" + targetSessionID.String()))
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO users (id, email_verified, status) VALUES ($1, true, 'active');
	`, targetUserID); err != nil {
		t.Fatalf("seed revocation target user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO sessions (id, user_id, kind, token_hash, expires_at)
		VALUES ($1, $2, 'core', $3, now() + interval '1 hour')
	`, targetSessionID, targetUserID, tokenHash[:]); err != nil {
		t.Fatalf("seed revocation target Session: %v", err)
	}

	path := "/api/v1/platform-operations/sessions/" + targetSessionID.String() + "/revocations"
	key := "idem_" + uuid.NewString()
	first := sendInboxRequest(t, fixture, http.MethodPost, path, `{"expected_active":true}`, key, "nonce_"+uuid.NewString(), "req_platform_session_revoke")
	firstPayload := decodePlatformOperation(t, first, http.StatusOK)
	if firstPayload.Status != "succeeded" || firstPayload.Operation != "session_revoke" || firstPayload.ResourceID != targetSessionID.String() {
		t.Fatalf("unexpected Session revocation result: %+v", firstPayload)
	}
	replay := decodePlatformOperation(t, sendInboxRequest(t, fixture, http.MethodPost, path, `{"expected_active":true}`, key, "nonce_"+uuid.NewString(), "req_platform_session_replay"), http.StatusOK)
	if replay != firstPayload {
		t.Fatalf("idempotent Session replay changed result: first=%+v replay=%+v", firstPayload, replay)
	}

	statusPath := "/api/v1/platform-operations/operations/session_revoke"
	resolved := decodePlatformOperation(t, sendInboxRequest(t, fixture, http.MethodGet, statusPath, "", key, "nonce_"+uuid.NewString(), "req_platform_session_status"), http.StatusOK)
	if resolved != firstPayload {
		t.Fatalf("resolved operation status = %+v, want %+v", resolved, firstPayload)
	}
	unknown := decodePlatformOperation(t, sendInboxRequest(t, fixture, http.MethodGet, statusPath, "", "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_platform_session_unknown"), http.StatusOK)
	if unknown.Status != "unknown" || unknown.Operation != "session_revoke" || unknown.ResourceID != "" {
		t.Fatalf("unknown operation status = %+v", unknown)
	}

	var revoked bool
	var audits int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT revoked_at IS NOT NULL,
		       (SELECT count(*) FROM platform_operations_audit_events WHERE resource_id = $1)
		FROM sessions WHERE id = $1
	`, targetSessionID).Scan(&revoked, &audits); err != nil {
		t.Fatalf("read Session revocation facts: %v", err)
	}
	if !revoked || audits != 1 {
		t.Fatalf("Session revoked/audits = %t/%d, want true/1", revoked, audits)
	}
}

func TestPlatformOperationsAccessUpdateUsesRevisionIdempotencyAndAudit(t *testing.T) {
	fixture := newInboxFixture(t)
	grantPlatformOperations(t, fixture)
	targetUserID := uuid.New()
	if _, err := fixture.pool.Exec(context.Background(), `INSERT INTO users (id, email_verified, status) VALUES ($1, true, 'active')`, targetUserID); err != nil {
		t.Fatalf("seed access-update target: %v", err)
	}

	path := "/api/v1/platform-operations/users/" + targetUserID.String() + "/access-updates"
	body := `{"expected_revision":1,"status":"suspended","grants":[{"role_code":"operations-operator","scope":{"kind":"product","product_code":"quizcraft"}}]}`
	key := "idem_" + uuid.NewString()
	first := decodePlatformOperation(t, sendInboxRequest(t, fixture, http.MethodPost, path, body, key, "nonce_"+uuid.NewString(), "req_platform_access_update"), http.StatusOK)
	if first.Status != "succeeded" || first.Operation != "access_update" || first.ResourceID != targetUserID.String() || first.ResourceVersion != 2 {
		t.Fatalf("unexpected access-update result: %+v", first)
	}
	replay := decodePlatformOperation(t, sendInboxRequest(t, fixture, http.MethodPost, path, body, key, "nonce_"+uuid.NewString(), "req_platform_access_replay"), http.StatusOK)
	if replay != first {
		t.Fatalf("idempotent access replay changed result: first=%+v replay=%+v", first, replay)
	}
	changedBody := `{"expected_revision":1,"status":"deleted","grants":[]}`
	changedReplay := sendInboxRequest(t, fixture, http.MethodPost, path, changedBody, key, "nonce_"+uuid.NewString(), "req_platform_access_changed_replay")
	changedReplay.Body.Close()
	if changedReplay.StatusCode != http.StatusConflict {
		t.Fatalf("changed idempotent replay = %d, want 409", changedReplay.StatusCode)
	}

	stale := sendInboxRequest(t, fixture, http.MethodPost, path, body, "idem_"+uuid.NewString(), "nonce_"+uuid.NewString(), "req_platform_access_stale")
	defer stale.Body.Close()
	if stale.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(stale.Body)
		t.Fatalf("stale access update = %d: %s, want 409", stale.StatusCode, payload)
	}

	var status, scopeKind, productCode string
	var revision int64
	var audits int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT users.status, users.authorization_revision, grants.scope_kind, grants.product_code,
		       (SELECT count(*) FROM platform_operations_audit_events WHERE resource_id = users.id)
		FROM users
		JOIN user_role_grants AS grants ON grants.user_id = users.id AND grants.status = 'active'
		WHERE users.id = $1
	`, targetUserID).Scan(&status, &revision, &scopeKind, &productCode, &audits); err != nil {
		t.Fatalf("read updated access facts: %v", err)
	}
	if status != "suspended" || revision != 2 || scopeKind != "product" || productCode != "quizcraft" || audits != 1 {
		t.Fatalf("updated access = status:%s revision:%d scope:%s/%s audits:%d", status, revision, scopeKind, productCode, audits)
	}
	if _, err := fixture.pool.Exec(context.Background(), `UPDATE platform_operations_audit_events SET request_id = 'req_mutated' WHERE resource_id = $1`, targetUserID); err == nil {
		t.Fatal("append-only Platform Operations audit accepted UPDATE")
	}
	if _, err := fixture.pool.Exec(context.Background(), `DELETE FROM platform_operations_audit_events WHERE resource_id = $1`, targetUserID); err == nil {
		t.Fatal("append-only Platform Operations audit accepted DELETE")
	}
}

func TestPlatformOperationsDeniesProductScopedGrant(t *testing.T) {
	fixture := newInboxFixture(t)
	ctx := context.Background()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO permission_codes (code, description) VALUES ('platform.operations.read', 'Read Platform Operations') ON CONFLICT DO NOTHING`, nil},
		{`INSERT INTO role_permissions (role_id, permission_code) SELECT id, 'platform.operations.read' FROM authorization_roles WHERE code = 'operations-operator' ON CONFLICT DO NOTHING`, nil},
	}
	for _, statement := range statements {
		if _, err := fixture.pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed product-scoped grant: %v", err)
		}
	}
	response := sendInboxRequest(t, fixture, http.MethodGet, "/api/v1/platform-operations", "", "", "nonce_"+uuid.NewString(), "req_platform_operations_product_scope")
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("product-scoped Platform Operations read = %d: %s, want 403", response.StatusCode, payload)
	}
}

type platformOperationResult struct {
	Operation       string `json:"operation"`
	Status          string `json:"status"`
	ResourceID      string `json:"resource_id,omitempty"`
	ResourceVersion int64  `json:"resource_version,omitempty"`
}

func decodePlatformOperation(t *testing.T, response *http.Response, wantStatus int) platformOperationResult {
	t.Helper()
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != wantStatus {
		t.Fatalf("Platform Operation = %d: %s, want %d", response.StatusCode, payload, wantStatus)
	}
	var envelope struct {
		Data platformOperationResult `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode Platform Operation: %v", err)
	}
	return envelope.Data
}

func TestPlatformOperationsSnapshotIsScopedAndContainsNoSecrets(t *testing.T) {
	fixture := newInboxFixture(t)
	grantPlatformOperations(t, fixture)
	seedPlatformOperationsFacts(t, fixture)

	response := sendInboxRequest(t, fixture, http.MethodGet, "/api/v1/platform-operations", "", "", "nonce_"+uuid.NewString(), "req_platform_operations_read")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("Platform Operations snapshot = %d: %s, want 200", response.StatusCode, payload)
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"token_hash", "secret_hash", "provider_message_id", "code_hash", "recipient_ciphertext", "source body must stay private"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("Platform Operations response leaked %q: %s", secret, payload)
		}
	}

	var envelope struct {
		Data struct {
			Accounts []struct {
				ID                    string  `json:"id"`
				DisplayName           *string `json:"display_name"`
				Email                 string  `json:"email"`
				Status                string  `json:"status"`
				AuthorizationRevision int64   `json:"authorization_revision"`
				Grants                []struct {
					RoleCode string `json:"role_code"`
				} `json:"grants"`
			} `json:"accounts"`
			Sessions []struct {
				ID          string  `json:"id"`
				UserID      string  `json:"user_id"`
				DisplayName *string `json:"display_name"`
				Email       string  `json:"email"`
			} `json:"sessions"`
			Mail struct {
				Pending   int64 `json:"pending"`
				Delivered int64 `json:"delivered"`
				Failed    int64 `json:"failed"`
			} `json:"mail"`
			InboxItems []struct {
				SourceProductCode string `json:"source_product_code"`
				SourceResourceID  string `json:"source_resource_id"`
			} `json:"inbox_items"`
			Audit []struct {
				RequestID      string  `json:"request_id"`
				PermissionCode string  `json:"permission_code"`
				DisplayName    *string `json:"display_name"`
				Email          string  `json:"email"`
			} `json:"audit"`
			Dependencies struct {
				Postgres string `json:"postgres"`
				Redis    string `json:"redis"`
			} `json:"dependencies"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode Platform Operations snapshot: %v", err)
	}
	if len(envelope.Data.Accounts) == 0 || len(envelope.Data.Sessions) == 0 || len(envelope.Data.InboxItems) == 0 || len(envelope.Data.Audit) == 0 {
		t.Fatalf("snapshot omitted an operational area: %+v", envelope.Data)
	}
	if len(envelope.Data.Accounts[0].Grants) == 0 {
		t.Fatalf("snapshot omitted account permission grants: %+v", envelope.Data.Accounts[0])
	}
	if envelope.Data.Accounts[0].DisplayName == nil || *envelope.Data.Accounts[0].DisplayName == "" {
		t.Fatalf("snapshot account omitted display_name: %+v", envelope.Data.Accounts[0])
	}
	if envelope.Data.Accounts[0].Email == "" || envelope.Data.Sessions[0].Email == "" || envelope.Data.Audit[0].Email == "" {
		t.Fatalf("snapshot omitted Platform Core-owned email: account=%+v session=%+v audit=%+v", envelope.Data.Accounts[0], envelope.Data.Sessions[0], envelope.Data.Audit[0])
	}
	if envelope.Data.Sessions[0].DisplayName == nil || *envelope.Data.Sessions[0].DisplayName == "" {
		t.Fatalf("snapshot Session omitted display_name: %+v", envelope.Data.Sessions[0])
	}
	if envelope.Data.Audit[0].DisplayName == nil || *envelope.Data.Audit[0].DisplayName == "" {
		t.Fatalf("snapshot audit event omitted display_name: %+v", envelope.Data.Audit[0])
	}
	if envelope.Data.Mail.Pending != 1 || envelope.Data.Mail.Delivered != 1 || envelope.Data.Mail.Failed != 1 {
		t.Fatalf("mail status counts = %+v, want pending/delivered/failed = 1/1/1", envelope.Data.Mail)
	}
	if envelope.Data.Dependencies.Postgres != "ready" || envelope.Data.Dependencies.Redis != "ready" {
		t.Fatalf("dependency status = %+v, want ready/ready", envelope.Data.Dependencies)
	}

}

func TestPlatformOperationsSnapshotRetainsAuditRowsAfterActorHardDelete(t *testing.T) {
	fixture := newInboxFixture(t)
	grantPlatformOperations(t, fixture)
	ctx := context.Background()
	actorID := uuid.New()
	sessionID := uuid.New()
	tokenHash := sha256.Sum256([]byte("audit-actor-session-" + actorID.String()))
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO users (id, email_verified, status, display_name) VALUES ($1, true, 'active', '已删除审计员')`, actorID); err != nil {
		t.Fatalf("seed audit actor: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `INSERT INTO sessions (id, user_id, kind, token_hash, expires_at) VALUES ($1, $2, 'core', $3, now() + interval '1 hour')`, sessionID, actorID, tokenHash[:]); err != nil {
		t.Fatalf("seed audit actor Session: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO authorization_audit_events (
			session_id, actor_user_id, request_id, service_id, permission_code,
			target_kind, decision, reason_code
		) VALUES ($1, $2, 'req_audit_actor_deleted', $3, 'platform.operations.read', 'platform', 'denied', 'NO_GRANT')
	`, sessionID, actorID, testClientID); err != nil {
		t.Fatalf("seed authorization audit event: %v", err)
	}
	before := snapshotAuditEvent(t, fixture, "req_audit_actor_deleted")
	if before.DisplayName == nil || *before.DisplayName != "已删除审计员" {
		t.Fatalf("audit event display_name = %v, want 已删除审计员", before.DisplayName)
	}
	if before.ActorUserID != actorID.String() || before.PermissionCode != "platform.operations.read" || before.TargetKind != "platform" || before.Decision != "denied" || before.ReasonCode != "NO_GRANT" || before.CreatedAt == "" || before.RequestID != "req_audit_actor_deleted" {
		t.Fatalf("audit event missing auditable facts: %+v", before)
	}

	// Simulate an out-of-band hard delete that future FK policy could permit:
	// the audit row must survive via LEFT JOIN and keep every other fact.
	if _, err := fixture.pool.Exec(ctx, `ALTER TABLE authorization_audit_events DROP CONSTRAINT authorization_audit_events_actor_user_id_fkey, DROP CONSTRAINT authorization_audit_events_session_id_fkey`); err != nil {
		t.Fatalf("drop audit actor foreign keys: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = fixture.pool.Exec(cleanupCtx, `DELETE FROM authorization_audit_events WHERE actor_user_id = $1`, actorID)
		_, _ = fixture.pool.Exec(cleanupCtx, `ALTER TABLE authorization_audit_events ADD CONSTRAINT authorization_audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE RESTRICT, ADD CONSTRAINT authorization_audit_events_session_id_fkey FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE RESTRICT`)
	})
	if _, err := fixture.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, actorID); err != nil {
		t.Fatalf("hard delete audit actor: %v", err)
	}
	after := snapshotAuditEvent(t, fixture, "req_audit_actor_deleted")
	if after.RequestID != before.RequestID || after.ActorUserID != before.ActorUserID || after.PermissionCode != before.PermissionCode || after.TargetKind != before.TargetKind || after.Decision != before.Decision || after.ReasonCode != before.ReasonCode {
		t.Fatalf("audit row lost facts after actor hard delete: before=%+v after=%+v", before, after)
	}
	if after.DisplayName != nil {
		t.Fatalf("audit row retained display_name of a deleted actor: %+v", after)
	}
}

type snapshotAuditFact struct {
	RequestID      string  `json:"request_id"`
	ActorUserID    string  `json:"actor_user_id"`
	DisplayName    *string `json:"display_name"`
	PermissionCode string  `json:"permission_code"`
	TargetKind     string  `json:"target_kind"`
	Decision       string  `json:"decision"`
	ReasonCode     string  `json:"reason_code"`
	CreatedAt      string  `json:"created_at"`
}

func snapshotAuditEvent(t *testing.T, fixture inboxFixture, requestID string) snapshotAuditFact {
	t.Helper()
	response := sendInboxRequest(t, fixture, http.MethodGet, "/api/v1/platform-operations", "", "", "nonce_"+uuid.NewString(), "req_platform_snapshot_audit")
	defer response.Body.Close()
	payload, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("Platform Operations snapshot = %d: %s, want 200", response.StatusCode, payload)
	}
	var envelope struct {
		Data struct {
			Audit []snapshotAuditFact `json:"audit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode audit snapshot: %v", err)
	}
	for _, event := range envelope.Data.Audit {
		if event.RequestID == requestID {
			return event
		}
	}
	t.Fatalf("snapshot omitted audit event %s: %s", requestID, payload)
	return snapshotAuditFact{}
}

func grantPlatformOperations(t *testing.T, fixture inboxFixture) {
	t.Helper()
	ctx := context.Background()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO permission_codes (code, description)
		  VALUES ('platform.operations.read', 'Read Platform Operations'),
		         ('platform.operations.write', 'Write Platform Operations')
		  ON CONFLICT (code) DO NOTHING`, nil},
		{`INSERT INTO role_permissions (role_id, permission_code)
		  SELECT id, permission
		  FROM authorization_roles
		  CROSS JOIN unnest(ARRAY['platform.operations.read', 'platform.operations.write']) AS permission
		  WHERE code = 'operations-operator'
		  ON CONFLICT DO NOTHING`, nil},
		{`INSERT INTO user_role_grants (user_id, role_id, scope_kind)
		  SELECT $1, id, 'platform' FROM authorization_roles WHERE code = 'operations-operator'`, []any{fixture.userID}},
	}
	for _, statement := range statements {
		if _, err := fixture.pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("grant Platform Operations access: %v", err)
		}
	}
}

func seedPlatformOperationsFacts(t *testing.T, fixture inboxFixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := fixture.pool.Exec(ctx, `
		WITH seeded_codes AS (
			INSERT INTO verification_codes (
				email_lookup_hash, purpose, request_key, request_fingerprint,
				code_nonce, code_hash, expires_at
			)
			SELECT decode(repeat('00', 32), 'hex'), 'login', 'request-key-' || value,
			       decode(repeat('01', 32), 'hex'), decode(repeat('02', 16), 'hex'),
			       decode(repeat('03', 32), 'hex'), now() + interval '10 minutes'
			FROM generate_series(1, 3) AS value
			RETURNING id, request_key
		)
		INSERT INTO mail_outbox (
			verification_code_id, dedupe_key, request_id, kind, priority,
			recipient_ciphertext, payload_ciphertext, status,
			accepted_at, delivered_at, failed_at
		)
		SELECT id, 'dedupe-' || request_key, 'req_mail_' || right(request_key, 1),
		       'verification_code', 'critical', decode('00', 'hex'), decode('00', 'hex'),
		       CASE right(request_key, 1) WHEN '1' THEN 'pending' WHEN '2' THEN 'delivered' ELSE 'failed' END,
		       CASE WHEN right(request_key, 1) = '2' THEN now() END,
		       CASE WHEN right(request_key, 1) = '2' THEN now() END,
		       CASE WHEN right(request_key, 1) = '3' THEN now() END
		FROM seeded_codes
	`); err != nil {
		t.Fatalf("seed Platform Operations mail facts: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO operations_inbox_items (
			source_product_code, source_resource_type, source_resource_id,
			priority, created_by, updated_by
		) VALUES ('quizcraft', 'feedback', 'feedback-private-reference', 'high', $1, $1)
	`, fixture.userID); err != nil {
		t.Fatalf("seed Platform Operations Inbox facts: %v", err)
	}
}
