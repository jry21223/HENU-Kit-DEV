package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	platformcore "henukit.dev/platform-core"
	"henukit.dev/platform-core/internal/contract"
)

const testExchangeToken = "exchange_test_session_token_with_enough_entropy"

func TestAuthorizationDefaultsToDenyAndIgnoresForgedRole(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)

	tokenHash := sha256.Sum256([]byte(testExchangeToken))
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at)
		SELECT user_id, 'client_exchange', $1, $2, id, now() + interval '5 minutes'
		FROM sessions WHERE kind = 'core'`, tokenHash[:], testClientID); err != nil {
		t.Fatalf("seed exchange Session: %v", err)
	}

	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	body := `{"session_exchange_token":"exchange_test_session_token_with_enough_entropy","permission_code":"quizcraft.bank.manage","scope":{"kind":"product","product_code":"quizcraft"}}`
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/authorization/check", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create authorization request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(testClientID, testClientSecret)
	request.Header.Set("X-Role", "platform-admin")
	request.Header.Set(contract.ServiceIDHeader, testClientID)
	request.Header.Set(contract.KeyIDHeader, testKeyID)
	request.Header.Set(contract.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
	request.Header.Set(contract.NonceHeader, "nonce_authorization_default_deny")
	signExchangeRequest(t, request)

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("check authorization: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("authorization without a grant = %d, want 403", response.StatusCode)
	}
}

func TestAuthorizationAllowsMatchingPermissionAndProductScope(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)

	token := "exchange_authorized_session_token_with_enough_entropy"
	tokenHash := sha256.Sum256([]byte(token))
	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at)
		SELECT user_id, 'client_exchange', $1, $2, id, now() + interval '5 minutes'
		FROM sessions WHERE kind = 'core'
		RETURNING user_id`, tokenHash[:], testClientID).Scan(&userID); err != nil {
		t.Fatalf("seed exchange Session: %v", err)
	}
	roleID, grantID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO permission_codes (code, description) VALUES ('quizcraft.bank.manage', 'Manage QuizCraft banks')`); err != nil {
		t.Fatalf("seed permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO authorization_roles (id, code, display_name) VALUES ($1, 'quizcraft-editor', 'QuizCraft editor')`, roleID); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_code) VALUES ($1, 'quizcraft.bank.manage')`, roleID); err != nil {
		t.Fatalf("seed role permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_role_grants (id, user_id, role_id, scope_kind, product_code) VALUES ($1, $2, $3, 'product', 'quizcraft')`, grantID, userID, roleID); err != nil {
		t.Fatalf("seed scoped role grant: %v", err)
	}

	handler, err := platformcore.New(platformcore.Config{
		Database: pool, Redis: redisClient, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		IdempotencyEncryptionKey: testIdempotencyEncryptionKey,
	})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	body := fmt.Sprintf(`{"session_exchange_token":%q,"permission_code":"quizcraft.bank.manage","scope":{"kind":"product","product_code":"quizcraft"}}`, token)
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/authorization/check", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(testClientID, testClientSecret)
	request.Header.Set(contract.ServiceIDHeader, testClientID)
	request.Header.Set(contract.KeyIDHeader, testKeyID)
	request.Header.Set(contract.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
	request.Header.Set(contract.NonceHeader, "nonce_authorization_matching_scope")
	signExchangeRequest(t, request)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("check authorization: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("matching authorization = %d, want 200", response.StatusCode)
	}
	var envelope contract.SuccessEnvelope[contract.AuthorizationDecision]
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode authorization decision: %v", err)
	}
	if !envelope.Data.Allowed || envelope.Data.ActorUserID != userID.String() || envelope.Data.GrantID != grantID.String() || envelope.Data.PermissionCode != "quizcraft.bank.manage" {
		t.Fatalf("unexpected authorization decision: %+v", envelope.Data)
	}
}

func TestAuthorizationAuditCorrelatesActorRequestAndTarget(t *testing.T) {
	var logs bytes.Buffer
	fixture := newAuthorizationFixture(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	requestID := "req_authorization_audit_001"
	response := sendAuthorizationCheck(t, fixture, "quizcraft", "nonce_authorization_audit", requestID)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("matching authorization = %d, want 200", response.StatusCode)
	}

	var actorID uuid.UUID
	var storedRequestID, permissionCode, targetKind, targetProduct, decision string
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT actor_user_id, request_id, permission_code, target_kind, target_product_code, decision
		FROM authorization_audit_events WHERE request_id = $1`, requestID).Scan(
		&actorID, &storedRequestID, &permissionCode, &targetKind, &targetProduct, &decision,
	); err != nil {
		t.Fatalf("read authorization audit: %v", err)
	}
	if actorID != fixture.userID || storedRequestID != requestID || permissionCode != "quizcraft.bank.manage" || targetKind != "product" || targetProduct != "quizcraft" || decision != "allowed" {
		t.Fatalf("unexpected authorization audit: actor=%s request=%s permission=%s target=%s/%s decision=%s", actorID, storedRequestID, permissionCode, targetKind, targetProduct, decision)
	}
}

func TestAuthorizationRejectsScopeEscapeAndAuditsDenial(t *testing.T) {
	fixture := newAuthorizationFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	requestID := "req_scope_escape_001"
	response := sendAuthorizationCheck(t, fixture, "food", "nonce_scope_escape", requestID)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("out-of-scope authorization = %d, want 403", response.StatusCode)
	}
	var decision, reason, product string
	var grantID *uuid.UUID
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT decision, reason_code, target_product_code, grant_id
		FROM authorization_audit_events WHERE request_id = $1`, requestID).Scan(&decision, &reason, &product, &grantID); err != nil {
		t.Fatalf("read denied authorization audit: %v", err)
	}
	if decision != "denied" || reason != "PERMISSION_OR_SCOPE_MISSING" || product != "food" || grantID != nil {
		t.Fatalf("unexpected denied audit: decision=%s reason=%s product=%s grant=%v", decision, reason, product, grantID)
	}
}

func TestAuthorizationRejectsMalformedScopeAsBadRequest(t *testing.T) {
	fixture := newAuthorizationFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	response := sendAuthorizationCheck(t, fixture, "QUIZCRAFT!", "nonce_malformed_scope", "")
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed Scope = %d, want 400", response.StatusCode)
	}
}

func TestAuthorizationRevocationsTakeEffectOnNextRequest(t *testing.T) {
	tests := []struct {
		name       string
		revoke     func(context.Context, authorizationFixture) error
		wantStatus int
		wantReason string
	}{
		{
			name: "account suspended",
			revoke: func(ctx context.Context, fixture authorizationFixture) error {
				_, err := fixture.pool.Exec(ctx, `UPDATE users SET status = 'suspended', authorization_revision = authorization_revision + 1 WHERE id = $1`, fixture.userID)
				return err
			},
			wantStatus: http.StatusUnauthorized,
			wantReason: "ACCOUNT_NOT_ACTIVE",
		},
		{
			name: "role revoked",
			revoke: func(ctx context.Context, fixture authorizationFixture) error {
				_, err := fixture.pool.Exec(ctx, `UPDATE authorization_roles SET status = 'revoked', revision = revision + 1, updated_at = now() WHERE id = $1`, fixture.roleID)
				return err
			},
			wantStatus: http.StatusForbidden,
			wantReason: "PERMISSION_OR_SCOPE_MISSING",
		},
		{
			name: "role grant revoked",
			revoke: func(ctx context.Context, fixture authorizationFixture) error {
				_, err := fixture.pool.Exec(ctx, `UPDATE user_role_grants SET status = 'revoked', revision = revision + 1, revoked_at = now(), updated_at = now() WHERE id = $1`, fixture.grantID)
				return err
			},
			wantStatus: http.StatusForbidden,
			wantReason: "PERMISSION_OR_SCOPE_MISSING",
		},
		{
			name: "exchange Session revoked",
			revoke: func(ctx context.Context, fixture authorizationFixture) error {
				hash := sha256.Sum256([]byte(fixture.token))
				_, err := fixture.pool.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1`, hash[:])
				return err
			},
			wantStatus: http.StatusUnauthorized,
			wantReason: "SESSION_REVOKED",
		},
		{
			name: "parent Core Session revoked",
			revoke: func(ctx context.Context, fixture authorizationFixture) error {
				_, err := fixture.pool.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE kind = 'core' AND user_id = $1`, fixture.userID)
				return err
			},
			wantStatus: http.StatusUnauthorized,
			wantReason: "PARENT_SESSION_REVOKED",
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAuthorizationFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
			before := sendAuthorizationCheck(t, fixture, "quizcraft", fmt.Sprintf("nonce_before_revoke_%d", index), "")
			before.Body.Close()
			if before.StatusCode != http.StatusOK {
				t.Fatalf("authorization before revocation = %d, want 200", before.StatusCode)
			}
			if err := test.revoke(context.Background(), fixture); err != nil {
				t.Fatalf("apply revocation: %v", err)
			}
			requestID := fmt.Sprintf("req_after_revoke_%d", index)
			after := sendAuthorizationCheck(t, fixture, "quizcraft", fmt.Sprintf("nonce_after_revoke_%d", index), requestID)
			after.Body.Close()
			if after.StatusCode != test.wantStatus {
				t.Fatalf("authorization immediately after revocation = %d, want %d", after.StatusCode, test.wantStatus)
			}
			var decision, reason string
			if err := fixture.pool.QueryRow(context.Background(), `
				SELECT decision, reason_code FROM authorization_audit_events WHERE request_id = $1`, requestID).Scan(&decision, &reason); err != nil {
				t.Fatalf("read revocation audit: %v", err)
			}
			if decision != "denied" || reason != test.wantReason {
				t.Fatalf("unexpected revocation audit: decision=%s reason=%s, want denied/%s", decision, reason, test.wantReason)
			}
		})
	}
}

func TestConcurrentDuplicateRoleGrantsHaveOneWinner(t *testing.T) {
	fixture := newAuthorizationFixture(t, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx := context.Background()
	if _, err := fixture.pool.Exec(ctx, `UPDATE user_role_grants SET status = 'revoked', revoked_at = now(), revision = revision + 1 WHERE id = $1`, fixture.grantID); err != nil {
		t.Fatalf("retire fixture grant: %v", err)
	}
	const attempts = 10
	var successes atomic.Int32
	errorsFound := make(chan error, attempts)
	var wait sync.WaitGroup
	for range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := fixture.pool.Exec(ctx, `
				INSERT INTO user_role_grants (user_id, role_id, scope_kind, product_code)
				VALUES ($1, $2, 'product', 'quizcraft')`, fixture.userID, fixture.roleID)
			if err == nil {
				successes.Add(1)
				return
			}
			var postgresError *pgconn.PgError
			if !errors.As(err, &postgresError) || postgresError.Code != "23505" {
				errorsFound <- err
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("unexpected concurrent grant error: %v", err)
	}
	if successes.Load() != 1 {
		t.Fatalf("concurrent active grants = %d, want exactly one", successes.Load())
	}
}

type authorizationFixture struct {
	pool    *pgxpool.Pool
	server  *httptest.Server
	token   string
	userID  uuid.UUID
	roleID  uuid.UUID
	grantID uuid.UUID
}

func newAuthorizationFixture(t *testing.T, logger *slog.Logger) authorizationFixture {
	t.Helper()
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)
	seedIdentity(t, ctx, pool)
	token := "exchange_fixture_" + uuid.NewString()
	tokenHash := sha256.Sum256([]byte(token))
	var userID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (user_id, kind, token_hash, client_id, parent_session_id, expires_at)
		SELECT user_id, 'client_exchange', $1, $2, id, now() + interval '5 minutes'
		FROM sessions WHERE kind = 'core' RETURNING user_id`, tokenHash[:], testClientID).Scan(&userID); err != nil {
		t.Fatalf("seed exchange Session: %v", err)
	}
	roleID, grantID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO permission_codes (code, description) VALUES ('quizcraft.bank.manage', 'Manage QuizCraft banks')`); err != nil {
		t.Fatalf("seed permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO authorization_roles (id, code, display_name) VALUES ($1, 'quizcraft-editor', 'QuizCraft editor')`, roleID); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_code) VALUES ($1, 'quizcraft.bank.manage')`, roleID); err != nil {
		t.Fatalf("seed role permission: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_role_grants (id, user_id, role_id, scope_kind, product_code) VALUES ($1, $2, $3, 'product', 'quizcraft')`, grantID, userID, roleID); err != nil {
		t.Fatalf("seed scoped role grant: %v", err)
	}
	handler, err := platformcore.New(platformcore.Config{Database: pool, Redis: redisClient, Logger: logger, IdempotencyEncryptionKey: testIdempotencyEncryptionKey})
	if err != nil {
		t.Fatalf("create platform core: %v", err)
	}
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return authorizationFixture{pool: pool, server: server, token: token, userID: userID, roleID: roleID, grantID: grantID}
}

func sendAuthorizationCheck(t *testing.T, fixture authorizationFixture, productCode, nonce, requestID string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(`{"session_exchange_token":%q,"permission_code":"quizcraft.bank.manage","scope":{"kind":"product","product_code":%q}}`, fixture.token, productCode)
	request, _ := http.NewRequest(http.MethodPost, fixture.server.URL+"/api/v1/authorization/check", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(testClientID, testClientSecret)
	request.Header.Set(contract.ServiceIDHeader, testClientID)
	request.Header.Set(contract.KeyIDHeader, testKeyID)
	request.Header.Set(contract.TimestampHeader, fmt.Sprintf("%d", time.Now().Unix()))
	request.Header.Set(contract.NonceHeader, nonce)
	if requestID != "" {
		request.Header.Set("X-Request-Id", requestID)
	}
	signExchangeRequest(t, request)
	response, err := fixture.server.Client().Do(request)
	if err != nil {
		t.Fatalf("check authorization: %v", err)
	}
	return response
}
