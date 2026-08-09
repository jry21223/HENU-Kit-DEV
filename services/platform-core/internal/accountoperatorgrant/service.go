package accountoperatorgrant

import (
	"context"
	"errors"
	"regexp"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	roleCodePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
	requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)
)

var permissionCodes = []string{
	"account.tickets.read",
	"account.tickets.reply",
	"account.tickets.transition",
	"account.membership.write",
	"account.points.adjust",
	"account.orders.read",
	"account.orders.close",
	"account.orders.refund",
}

// portalNoticeReaderRoleCode is the narrowly permissioned baseline role that
// registration and migration assign to eligible Portal users. This release
// helper mutates a role's permissions, so it must never target that role.
const portalNoticeReaderRoleCode = "portal-notice-reader"

type Input struct {
	RoleCode  string
	Actor     string
	RequestID string
	Reason    string
}

type Result struct {
	Changed bool
}

func Grant(ctx context.Context, database *pgxpool.Pool, input Input) (Result, error) {
	if database == nil || !roleCodePattern.MatchString(input.RoleCode) ||
		input.RoleCode == portalNoticeReaderRoleCode ||
		!requestIDPattern.MatchString(input.RequestID) || len(input.Actor) < 1 || len(input.Actor) > 120 ||
		len(input.Reason) < 8 || len(input.Reason) > 500 {
		return Result{}, errors.New("invalid account operator role grant input")
	}
	tx, err := database.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var roleID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM authorization_roles WHERE code=$1 AND status='active' FOR UPDATE`, input.RoleCode).Scan(&roleID); err != nil {
		return Result{}, err
	}
	var existingRoleCode, existingActor, existingReason string
	var existingPermissions []string
	err = tx.QueryRow(ctx, `SELECT role_code, actor, reason, permission_codes FROM account_operator_role_grant_audit_events WHERE request_id=$1`, input.RequestID).Scan(&existingRoleCode, &existingActor, &existingReason, &existingPermissions)
	if err == nil {
		if existingRoleCode == input.RoleCode && existingActor == input.Actor && existingReason == input.Reason && slices.Equal(existingPermissions, permissionCodes) {
			return Result{Changed: false}, nil
		}
		return Result{}, errors.New("account operator role grant request conflicts with its audit")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	var permissionCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM permission_codes WHERE status='active' AND code = ANY($1::text[])`, permissionCodes).Scan(&permissionCount); err != nil {
		return Result{}, err
	}
	if permissionCount != len(permissionCodes) {
		return Result{}, errors.New("account operator permissions are incomplete")
	}

	command, err := tx.Exec(ctx, `
INSERT INTO role_permissions(role_id, permission_code)
SELECT $1::uuid, unnest($2::text[])
ON CONFLICT DO NOTHING`, roleID, permissionCodes)
	if err != nil {
		return Result{}, err
	}
	changed := command.RowsAffected() > 0
	if changed {
		if _, err := tx.Exec(ctx, `UPDATE authorization_roles SET revision=revision+1, updated_at=now() WHERE id=$1::uuid`, roleID); err != nil {
			return Result{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO account_operator_role_grant_audit_events(role_id, role_code, actor, request_id, reason, permission_codes, changed)
VALUES ($1::uuid,$2,$3,$4,$5,$6,$7)`, roleID, input.RoleCode, input.Actor, input.RequestID, input.Reason, permissionCodes, changed); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{Changed: changed}, nil
}
