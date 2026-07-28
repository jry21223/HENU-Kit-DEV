package tests

import (
	"context"
	"os"
	"testing"
)

func TestMembershipPermissionMigrationIsIdempotentAndReversible(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)

	up, err := os.ReadFile("../db/migrations/000015_account_portfolio_membership_access.up.sql")
	if err != nil {
		t.Fatalf("read membership access migration up: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("apply membership access migration up attempt %d: %v", attempt+1, err)
		}
	}

	var description, status string
	if err := pool.QueryRow(ctx, `SELECT description, status FROM permission_codes WHERE code = 'account.membership.write'`).Scan(&description, &status); err != nil {
		t.Fatalf("read membership permission: %v", err)
	}
	if description != "Grant or revoke Account Portfolio lifetime membership within Account Portfolio product Scope" || status != "active" {
		t.Fatalf("membership permission = %q/%q", description, status)
	}
	var automaticGrants int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE permission_code = 'account.membership.write'`).Scan(&automaticGrants); err != nil {
		t.Fatalf("count membership role grants: %v", err)
	}
	if automaticGrants != 0 {
		t.Fatalf("membership migration granted %d roles, want zero", automaticGrants)
	}

	down, err := os.ReadFile("../db/migrations/000015_account_portfolio_membership_access.down.sql")
	if err != nil {
		t.Fatalf("read membership access migration down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("apply membership access migration down: %v", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permission_codes WHERE code = 'account.membership.write'`).Scan(&remaining); err != nil {
		t.Fatalf("count membership permission after down: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("membership permission remains after down: %d", remaining)
	}
}
