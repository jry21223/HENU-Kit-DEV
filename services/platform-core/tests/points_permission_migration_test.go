package tests

import (
	"context"
	"os"
	"testing"
)

func TestPointAdjustmentPermissionMigrationIsIdempotentAndDoesNotGrantRoles(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	resetIdentityTables(t, ctx, pool, redisClient)

	up, err := os.ReadFile("../db/migrations/000016_account_portfolio_points_access.up.sql")
	if err != nil {
		t.Fatalf("read points access migration up: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := pool.Exec(ctx, string(up)); err != nil {
			t.Fatalf("apply points access migration up attempt %d: %v", attempt+1, err)
		}
	}

	var description, status string
	if err := pool.QueryRow(ctx, `SELECT description, status FROM permission_codes WHERE code = 'account.points.adjust'`).Scan(&description, &status); err != nil {
		t.Fatalf("read point adjustment permission: %v", err)
	}
	if description != "Adjust Account Portfolio points within Account Portfolio product Scope" || status != "active" {
		t.Fatalf("point adjustment permission = %q/%q", description, status)
	}
	var automaticGrants int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions WHERE permission_code = 'account.points.adjust'`).Scan(&automaticGrants); err != nil {
		t.Fatalf("count point adjustment role grants: %v", err)
	}
	if automaticGrants != 0 {
		t.Fatalf("points migration granted %d roles, want zero", automaticGrants)
	}

	down, err := os.ReadFile("../db/migrations/000016_account_portfolio_points_access.down.sql")
	if err != nil {
		t.Fatalf("read points access migration down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("apply points access migration down: %v", err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM permission_codes WHERE code = 'account.points.adjust'`).Scan(&remaining); err != nil {
		t.Fatalf("count point adjustment permission after down: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("point adjustment permission remains after down: %d", remaining)
	}
}
