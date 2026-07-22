package operatorbootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Input struct {
	Email, ActorUnixUser, RequestID, Reason string
}

type Result struct {
	UserID  string
	Changed bool
}

func Grant(ctx context.Context, database *pgxpool.Pool, verificationKey []byte, input Input) (Result, error) {
	if database == nil || len(verificationKey) != 32 || input.ActorUnixUser == "" || !strings.HasPrefix(input.RequestID, "req_") || len(input.Reason) < 8 {
		return Result{}, errors.New("database, verification key, actor, request ID, and reason are required")
	}
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return Result{}, err
	}
	emailHash := emailLookupHash(verificationKey, email)
	tx, err := database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var userID, status string
	if err := tx.QueryRow(ctx, `SELECT users.id::text, users.status FROM email_identities JOIN users ON users.id=email_identities.user_id WHERE email_identities.email_lookup_hash=$1 FOR UPDATE OF users, email_identities`, emailHash).Scan(&userID, &status); err != nil {
		return Result{}, err
	}
	if status != "active" {
		return Result{}, errors.New("target account is not active")
	}
	command, err := tx.Exec(ctx, `
WITH roles AS (
    INSERT INTO authorization_roles(code, display_name, status)
    VALUES ('platform-operator', 'Platform Operator', 'active'),
           ('quizcraft-workshop-operator', 'QuizCraft Workshop Operator', 'active')
    ON CONFLICT (code) DO UPDATE SET display_name=EXCLUDED.display_name, status='active', revision=authorization_roles.revision+1, updated_at=now()
    WHERE authorization_roles.display_name IS DISTINCT FROM EXCLUDED.display_name OR authorization_roles.status IS DISTINCT FROM 'active'
    RETURNING id, code
), all_roles AS (
    SELECT id, code FROM roles
    UNION ALL
    SELECT id, code FROM authorization_roles WHERE code IN ('platform-operator','quizcraft-workshop-operator') AND NOT EXISTS (SELECT 1 FROM roles WHERE roles.id=authorization_roles.id)
), desired_permissions(role_code, permission_code) AS (
    VALUES
      ('platform-operator','platform.operations.read'),
      ('platform-operator','platform.operations.write'),
      ('quizcraft-workshop-operator','quizcraft.workshop.read'),
      ('quizcraft-workshop-operator','quizcraft.workshop.write'),
      ('quizcraft-workshop-operator','quizcraft.workshop.publish')
), inserted_permissions AS (
    INSERT INTO role_permissions(role_id, permission_code)
    SELECT all_roles.id, desired_permissions.permission_code FROM all_roles JOIN desired_permissions ON desired_permissions.role_code=all_roles.code
    ON CONFLICT DO NOTHING
), desired_grants AS (
    SELECT id AS role_id, CASE code WHEN 'platform-operator' THEN 'platform' ELSE 'product' END AS scope_kind,
           CASE code WHEN 'quizcraft-workshop-operator' THEN 'quizcraft' END AS product_code
    FROM all_roles
)
INSERT INTO user_role_grants(user_id, role_id, scope_kind, product_code)
SELECT $1::uuid, role_id, scope_kind, product_code FROM desired_grants
WHERE NOT EXISTS (
    SELECT 1 FROM user_role_grants existing
    WHERE existing.user_id=$1::uuid AND existing.role_id=desired_grants.role_id AND existing.scope_kind=desired_grants.scope_kind
      AND COALESCE(existing.product_code,'')=COALESCE(desired_grants.product_code,'') AND existing.status='active'
)`, userID)
	if err != nil {
		return Result{}, err
	}
	changed := command.RowsAffected() > 0
	if changed {
		if _, err := tx.Exec(ctx, `UPDATE users SET authorization_revision=authorization_revision+1 WHERE id=$1`, userID); err != nil {
			return Result{}, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO operator_bootstrap_audit_events(target_user_id, actor_unix_user, request_id, reason, permission_codes, scope_summary, changed) VALUES ($1,$2,$3,$4,$5,'[{"kind":"platform"},{"kind":"product","product_code":"quizcraft"}]'::jsonb,$6)`, userID, input.ActorUnixUser, input.RequestID, input.Reason, []string{"platform.operations.read", "platform.operations.write", "quizcraft.workshop.read", "quizcraft.workshop.write", "quizcraft.workshop.publish"}, changed); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{UserID: userID, Changed: changed}, nil
}

func normalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed.Address != normalized || !strings.HasSuffix(normalized, "@henu.edu.cn") {
		return "", errors.New("a henu.edu.cn account email is required")
	}
	return normalized, nil
}

func emailLookupHash(key []byte, email string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("henukit-verification:email"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(email))
	return mac.Sum(nil)
}
