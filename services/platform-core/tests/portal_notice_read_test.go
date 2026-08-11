package tests

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/contract"
)

const (
	portalNoticeClientID     = "portal-gateway"
	portalNoticeClientSecret = "portal-gateway-test-secret-with-enough-entropy"
	portalNoticeKeyID        = "portal-notice-primary"
)

func TestRegistrationGrantsPortalNoticeReadToItsPortalActor(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedPortalNoticeClient(t, ctx, pool)
	server := newVerificationServer(t, pool, redisClient)

	client, csrfToken, code := prepareRegistrationCode(t, ctx, pool, server, "portal-notice-registration-device")
	registered, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token":   {csrfToken},
		"display_name": {"通知阅读测试用户"},
		"email":        {testStudentEmail},
		"code":         {code},
		"password":     {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("submit registration: %v", err)
	}
	registered.Body.Close()
	if registered.StatusCode != http.StatusSeeOther {
		t.Fatalf("registration = %d, want 303", registered.StatusCode)
	}

	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE display_name = '通知阅读测试用户'`).Scan(&userID); err != nil {
		t.Fatalf("read registered user: %v", err)
	}
	exchangeToken := "portal_notice_exchange_" + uuid.NewString()
	tokenHash := sha256.Sum256([]byte(exchangeToken))
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at)
		SELECT user_id, 'client_exchange', $1, $2, id, now() + interval '5 minutes'
		FROM sessions
		WHERE user_id = $3 AND kind = 'core'`, tokenHash[:], portalNoticeClientID, userID); err != nil {
		t.Fatalf("seed Portal exchange Session: %v", err)
	}

	allowed := sendPortalNoticeAuthorizationCheck(t, server, exchangeToken, "nonce_portal_notice_allowed", "req_portal_notice_allowed")
	defer allowed.Body.Close()
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("registered Portal actor authorization = %d, want 200", allowed.StatusCode)
	}
	var envelope contract.SuccessEnvelope[contract.AuthorizationDecision]
	if err := json.NewDecoder(allowed.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode authorization decision: %v", err)
	}
	if !envelope.Data.Allowed || envelope.Data.ActorUserID != userID.String() || envelope.Data.PermissionCode != "portal.notice.read" || envelope.Data.Scope.Kind != "product" || envelope.Data.Scope.ProductCode == nil || *envelope.Data.Scope.ProductCode != "portal" {
		t.Fatalf("unexpected Portal authorization decision: %+v", envelope.Data)
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET status = 'suspended', authorization_revision = authorization_revision + 1 WHERE id = $1`, userID); err != nil {
		t.Fatalf("suspend registered user: %v", err)
	}
	suspended := sendPortalNoticeAuthorizationCheck(t, server, exchangeToken, "nonce_portal_notice_suspended", "req_portal_notice_suspended")
	suspended.Body.Close()
	if suspended.StatusCode != http.StatusUnauthorized {
		t.Fatalf("suspended Portal actor authorization = %d, want 401", suspended.StatusCode)
	}

	if _, err := pool.Exec(ctx, `UPDATE users SET status = 'active', authorization_revision = authorization_revision + 1 WHERE id = $1`, userID); err != nil {
		t.Fatalf("restore registered user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE user_role_grants
		SET status = 'revoked', revoked_at = now(), revision = revision + 1, updated_at = now()
		WHERE user_id = $1
		  AND role_id = (SELECT id FROM authorization_roles WHERE code = 'portal-notice-reader')
		  AND scope_kind = 'product'
		  AND product_code = 'portal'`, userID); err != nil {
		t.Fatalf("revoke Portal notice grant: %v", err)
	}
	revoked := sendPortalNoticeAuthorizationCheck(t, server, exchangeToken, "nonce_portal_notice_revoked", "req_portal_notice_revoked")
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked Portal notice grant authorization = %d, want 403", revoked.StatusCode)
	}
}

func TestRegistrationRollsBackWhenPortalNoticeGrantIsUnavailable(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	if _, err := pool.Exec(ctx, `
		UPDATE authorization_roles
		SET status = 'revoked', revision = revision + 1, updated_at = now()
		WHERE code = 'portal-notice-reader'`); err != nil {
		t.Fatalf("revoke Portal notice role: %v", err)
	}
	server := newVerificationServer(t, pool, redisClient)

	client, csrfToken, code := prepareRegistrationCode(t, ctx, pool, server, "portal-notice-missing-grant-device")
	registered, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token":   {csrfToken},
		"display_name": {"授权缺失测试用户"},
		"email":        {testStudentEmail},
		"code":         {code},
		"password":     {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("submit registration without Portal notice grant: %v", err)
	}
	registered.Body.Close()
	if registered.StatusCode != http.StatusOK {
		t.Fatalf("registration without Portal notice grant = %d, want failure form", registered.StatusCode)
	}

	var users, identities, credentials, sessions, consumed int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM email_identities),
			(SELECT count(*) FROM password_credentials),
			(SELECT count(*) FROM sessions WHERE kind = 'core'),
			(SELECT count(*) FROM verification_codes WHERE purpose = 'register' AND used_at IS NOT NULL)`).Scan(
		&users, &identities, &credentials, &sessions, &consumed,
	); err != nil {
		t.Fatalf("read failed registration facts: %v", err)
	}
	if users != 0 || identities != 0 || credentials != 0 || sessions != 0 || consumed != 0 {
		t.Fatalf("registration without Portal notice grant committed users=%d identities=%d credentials=%d sessions=%d consumed=%d", users, identities, credentials, sessions, consumed)
	}
}

func TestPortalNoticeRoleWithExtraPermissionCannotBackfillOrRegister(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)

	var roleID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM authorization_roles WHERE code = 'portal-notice-reader'`).Scan(&roleID); err != nil {
		t.Fatalf("read Portal notice role: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO permission_codes (code, description) VALUES ('portal.notice.extra', 'Unexpected Portal notice capability')`); err != nil {
		t.Fatalf("seed extra Portal notice permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_code) VALUES ($1, 'portal.notice.extra')`, roleID); err != nil {
		t.Fatalf("attach extra Portal notice role permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE permission_codes SET description = 'Portal notice preflight permission sentinel' WHERE code = 'portal.notice.read'`); err != nil {
		t.Fatalf("seed Portal notice permission preflight sentinel: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE authorization_roles SET display_name = 'Portal notice preflight role sentinel' WHERE id = $1`, roleID); err != nil {
		t.Fatalf("seed Portal notice role preflight sentinel: %v", err)
	}

	backfillCandidate := createPortalNoticeMigrationUser(t, ctx, pool, true, "active")
	var beforePermissionDescription, beforeRoleDisplayName string
	var beforeRolePermissions, beforeGrants int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT description FROM permission_codes WHERE code = 'portal.notice.read'),
			(SELECT display_name FROM authorization_roles WHERE id = $1),
			(SELECT count(*) FROM role_permissions WHERE role_id = $1),
			(SELECT count(*) FROM user_role_grants WHERE role_id = $1)`, roleID,
	).Scan(&beforePermissionDescription, &beforeRoleDisplayName, &beforeRolePermissions, &beforeGrants); err != nil {
		t.Fatalf("read preflight state: %v", err)
	}
	up, err := os.ReadFile("../db/migrations/000019_portal_notice_read.up.sql")
	if err != nil {
		t.Fatalf("read Portal notice migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(up)); err == nil {
		t.Fatal("Portal notice migration accepted an active role with an extra permission")
	} else if !strings.Contains(err.Error(), "portal-notice-reader") {
		t.Fatalf("Portal notice migration rejected extra role permission without a safe diagnostic: %v", err)
	}
	var afterPermissionDescription, afterRoleDisplayName string
	var afterRolePermissions, afterGrants int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT description FROM permission_codes WHERE code = 'portal.notice.read'),
			(SELECT display_name FROM authorization_roles WHERE id = $1),
			(SELECT count(*) FROM role_permissions WHERE role_id = $1),
			(SELECT count(*) FROM user_role_grants WHERE role_id = $1)`, roleID,
	).Scan(&afterPermissionDescription, &afterRoleDisplayName, &afterRolePermissions, &afterGrants); err != nil {
		t.Fatalf("read rejected-migration state: %v", err)
	}
	if beforePermissionDescription != afterPermissionDescription || beforeRoleDisplayName != afterRoleDisplayName || beforeRolePermissions != afterRolePermissions || beforeGrants != afterGrants {
		t.Fatalf("Portal notice migration changed state after rejecting extra role permission: before permission=%q role=%q permissions=%d grants=%d; after permission=%q role=%q permissions=%d grants=%d", beforePermissionDescription, beforeRoleDisplayName, beforeRolePermissions, beforeGrants, afterPermissionDescription, afterRoleDisplayName, afterRolePermissions, afterGrants)
	}
	assertPortalNoticeGrantState(t, ctx, pool, backfillCandidate, 0, 0)

	server := newVerificationServer(t, pool, redisClient)
	client, csrfToken, code := prepareRegistrationCode(t, ctx, pool, server, "portal-notice-extra-role-device")
	registered, err := client.PostForm(server.URL+"/register", url.Values{
		"csrf_token":   {csrfToken},
		"display_name": {"额外权限角色测试用户"},
		"email":        {testStudentEmail},
		"code":         {code},
		"password":     {"correct horse 电池 staple"},
	})
	if err != nil {
		t.Fatalf("submit registration with extra Portal notice role permission: %v", err)
	}
	registered.Body.Close()
	if registered.StatusCode != http.StatusOK {
		t.Fatalf("registration with extra Portal notice role permission = %d, want failure form", registered.StatusCode)
	}

	var users, identities, credentials, sessions, grants, consumed int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM email_identities),
			(SELECT count(*) FROM password_credentials),
			(SELECT count(*) FROM sessions WHERE kind = 'core'),
			(SELECT count(*) FROM user_role_grants WHERE role_id = $1),
			(SELECT count(*) FROM verification_codes WHERE purpose = 'register' AND used_at IS NOT NULL)`, roleID).Scan(
		&users, &identities, &credentials, &sessions, &grants, &consumed,
	); err != nil {
		t.Fatalf("read failed extra-role registration facts: %v", err)
	}
	if users != 1 || identities != 0 || credentials != 0 || sessions != 0 || grants != 0 || consumed != 0 {
		t.Fatalf("extra-role registration committed users=%d identities=%d credentials=%d sessions=%d grants=%d consumed=%d", users, identities, credentials, sessions, grants, consumed)
	}
}

func TestPortalNoticeReadMigrationBackfillsOnlyActiveVerifiedUsersAndPreservesRevocation(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)

	activeVerified := createPortalNoticeMigrationUser(t, ctx, pool, true, "active")
	suspendedVerified := createPortalNoticeMigrationUser(t, ctx, pool, true, "suspended")
	activeUnverified := createPortalNoticeMigrationUser(t, ctx, pool, false, "active")
	revokedGrantUser := createPortalNoticeMigrationUser(t, ctx, pool, true, "active")
	var roleID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM authorization_roles WHERE code = 'portal-notice-reader'`).Scan(&roleID); err != nil {
		t.Fatalf("read Portal notice role: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_role_grants (user_id, role_id, scope_kind, product_code, status, revoked_at)
		VALUES ($1, $2, 'product', 'portal', 'revoked', now())`, revokedGrantUser, roleID); err != nil {
		t.Fatalf("seed revoked Portal notice grant: %v", err)
	}

	up, err := os.ReadFile("../db/migrations/000019_portal_notice_read.up.sql")
	if err != nil {
		t.Fatalf("read Portal notice migration: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("apply Portal notice migration attempt %d: %v", attempt+1, err)
		}
	}

	var permissionStatus, roleStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM permission_codes WHERE code = 'portal.notice.read'`).Scan(&permissionStatus); err != nil {
		t.Fatalf("read Portal notice permission: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM authorization_roles WHERE id = $1`, roleID).Scan(&roleStatus); err != nil {
		t.Fatalf("read Portal notice role status: %v", err)
	}
	if permissionStatus != "active" || roleStatus != "active" {
		t.Fatalf("Portal notice baseline statuses permission=%q role=%q, want active/active", permissionStatus, roleStatus)
	}

	assertPortalNoticeGrantState(t, ctx, pool, activeVerified, 1, 0)
	assertPortalNoticeGrantState(t, ctx, pool, suspendedVerified, 0, 0)
	assertPortalNoticeGrantState(t, ctx, pool, activeUnverified, 0, 0)
	assertPortalNoticeGrantState(t, ctx, pool, revokedGrantUser, 0, 1)

	down, err := os.ReadFile("../db/migrations/000019_portal_notice_read.down.sql")
	if err != nil {
		t.Fatalf("read Portal notice migration down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("apply Portal notice migration down: %v", err)
	}
	assertPortalNoticeGrantState(t, ctx, pool, activeVerified, 1, 0)
	assertPortalNoticeGrantState(t, ctx, pool, revokedGrantUser, 0, 1)

	if _, err := pool.Exec(ctx, `UPDATE authorization_roles SET status = 'revoked', updated_at = now() WHERE id = $1`, roleID); err != nil {
		t.Fatalf("revoke Portal notice role before reapply: %v", err)
	}
	if _, err := pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply Portal notice migration after role revocation: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM authorization_roles WHERE id = $1`, roleID).Scan(&roleStatus); err != nil {
		t.Fatalf("read revoked Portal notice role: %v", err)
	}
	if roleStatus != "revoked" {
		t.Fatalf("Portal notice migration reactivated role status %q", roleStatus)
	}
}

func createPortalNoticeMigrationUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, emailVerified bool, status string) uuid.UUID {
	t.Helper()
	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (email_verified, status, display_name)
		VALUES ($1, $2, 'Portal notice migration user')
		RETURNING id`, emailVerified, status).Scan(&userID); err != nil {
		t.Fatalf("create Portal notice migration user: %v", err)
	}
	return userID
}

func assertPortalNoticeGrantState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, wantActive, wantRevoked int) {
	t.Helper()
	var active, revoked int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE grants.status = 'active'),
			count(*) FILTER (WHERE grants.status = 'revoked')
		FROM user_role_grants AS grants
		JOIN authorization_roles AS roles ON roles.id = grants.role_id
		WHERE grants.user_id = $1
		  AND roles.code = 'portal-notice-reader'
		  AND grants.scope_kind = 'product'
		  AND grants.product_code = 'portal'`, userID).Scan(&active, &revoked); err != nil {
		t.Fatalf("read Portal notice grant state: %v", err)
	}
	if active != wantActive || revoked != wantRevoked {
		t.Fatalf("Portal notice grants active=%d revoked=%d, want %d/%d", active, revoked, wantActive, wantRevoked)
	}
}

func seedPortalNoticeClient(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	secretHash := sha256.Sum256([]byte(portalNoticeClientSecret))
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_clients (id, redirect_uris) VALUES ($1, $2)`, portalNoticeClientID, []string{"https://portal.henukit.test/api/v1/auth/callback"}); err != nil {
		t.Fatalf("seed Portal OAuth client: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO oauth_client_keys (client_id, key_id, secret_hash, status) VALUES ($1, $2, $3, 'active')`, portalNoticeClientID, portalNoticeKeyID, secretHash[:]); err != nil {
		t.Fatalf("seed Portal OAuth client key: %v", err)
	}
}

func sendPortalNoticeAuthorizationCheck(t *testing.T, server *httptest.Server, exchangeToken, nonce, requestID string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"session_exchange_token":%q,"permission_code":"portal.notice.read","scope":{"kind":"product","product_code":"portal"}}`, exchangeToken)
	request, err := http.NewRequest(http.MethodPost, server.URL+contract.AuthorizationCheckRoute, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create Portal authorization request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(portalNoticeClientID, portalNoticeClientSecret)
	request.Header.Set(contract.ServiceIDHeader, portalNoticeClientID)
	request.Header.Set(contract.KeyIDHeader, portalNoticeKeyID)
	request.Header.Set(contract.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
	request.Header.Set(contract.NonceHeader, nonce)
	request.Header.Set("X-Request-Id", requestID)
	signExchangeRequestWithSecret(t, request, portalNoticeClientSecret)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("check Portal notice authorization: %v", err)
	}
	return response
}
