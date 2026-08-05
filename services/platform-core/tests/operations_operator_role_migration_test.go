package tests

import (
	"context"
	"os"
	"testing"
)

func TestOperationsOperatorRoleMigrationIsIdempotentAndDoesNotGrantAccess(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)

	up, err := os.ReadFile("../db/migrations/000019_operations_operator_role.up.sql")
	if err != nil {
		t.Fatalf("read operations operator role migration up: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("apply operations operator role migration up attempt %d: %v", attempt+1, err)
		}
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM authorization_roles WHERE code='operations-operator'`).Scan(&status); err != nil {
		t.Fatalf("read operations operator role: %v", err)
	}
	if status != "active" {
		t.Fatalf("operations operator status = %q", status)
	}
	var permissionGrants, userGrants int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions rp JOIN authorization_roles r ON r.id=rp.role_id WHERE r.code='operations-operator'`).Scan(&permissionGrants); err != nil {
		t.Fatalf("count role permissions: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_role_grants g JOIN authorization_roles r ON r.id=g.role_id WHERE r.code='operations-operator'`).Scan(&userGrants); err != nil {
		t.Fatalf("count user grants: %v", err)
	}
	if permissionGrants != 0 || userGrants != 0 {
		t.Fatalf("role migration granted access: permissions=%d users=%d", permissionGrants, userGrants)
	}

	down, err := os.ReadFile("../db/migrations/000019_operations_operator_role.down.sql")
	if err != nil {
		t.Fatalf("read operations operator role migration down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("apply operations operator role migration down: %v", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM authorization_roles WHERE code='operations-operator'`).Scan(&remaining); err != nil {
		t.Fatalf("count role after down: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("ownership-safe down changed operations operator role count: %d", remaining)
	}
}
