package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"henukit.dev/platform-core/internal/accountoperatorgrant"
)

func TestAccountOperatorRoleGrantIsOwnedAuditedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	pool, redisClient := openDependencies(t, ctx)
	auditMigration, err := os.ReadFile("../db/migrations/000018_account_operator_role_grant_audit.up.sql")
	if err != nil {
		t.Fatalf("read audit migration: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := pool.Exec(ctx, string(auditMigration)); err != nil {
			t.Fatalf("reapply audit migration attempt %d: %v", attempt+1, err)
		}
	}
	resetIdentityTables(t, ctx, pool, redisClient)
	for migration := 14; migration <= 17; migration++ {
		matches, err := filepath.Glob(fmt.Sprintf("../db/migrations/%06d_*.up.sql", migration))
		if err != nil {
			t.Fatalf("find migration %d: %v", migration, err)
		}
		if len(matches) != 1 {
			t.Fatalf("migration %d matches %d files", migration, len(matches))
		}
		contents, readErr := os.ReadFile(matches[0])
		if readErr != nil {
			t.Fatalf("read migration %s: %v", matches[0], readErr)
		}
		if _, execErr := pool.Exec(ctx, string(contents)); execErr != nil {
			t.Fatalf("apply migration %s: %v", matches[0], execErr)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO authorization_roles(code, display_name) VALUES ('operations-operator','Operations operator')`); err != nil {
		t.Fatalf("create operator role: %v", err)
	}

	for attempt, requestID := range []string{"req_release_aaaaaaaa_first", "req_release_aaaaaaaa_second"} {
		result, err := accountoperatorgrant.Grant(ctx, pool, accountoperatorgrant.Input{
			RoleCode: "operations-operator", Actor: "henukit-release", RequestID: requestID,
			Reason: "Account Portfolio production release aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		})
		if err != nil {
			t.Fatalf("grant attempt %d: %v", attempt+1, err)
		}
		if result.Changed != (attempt == 0) {
			t.Fatalf("grant attempt %d changed=%t", attempt+1, result.Changed)
		}
	}
	replay, err := accountoperatorgrant.Grant(ctx, pool, accountoperatorgrant.Input{
		RoleCode: "operations-operator", Actor: "henukit-release", RequestID: "req_release_aaaaaaaa_first",
		Reason: "Account Portfolio production release aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	})
	if err != nil || replay.Changed {
		t.Fatalf("audited replay changed=%t err=%v", replay.Changed, err)
	}

	var grantCount int
	var revision int64
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions rp JOIN authorization_roles r ON r.id=rp.role_id WHERE r.code='operations-operator' AND rp.permission_code LIKE 'account.%'`).Scan(&grantCount); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT revision FROM authorization_roles WHERE code='operations-operator'`).Scan(&revision); err != nil {
		t.Fatalf("read revision: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM account_operator_role_grant_audit_events WHERE role_code='operations-operator'`).Scan(&auditCount); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if grantCount != 8 || revision != 2 || auditCount != 2 {
		t.Fatalf("grant facts count=%d revision=%d audits=%d", grantCount, revision, auditCount)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO authorization_roles(code, display_name) VALUES ('concurrent-operator','Concurrent operator')`); err != nil {
		t.Fatalf("create concurrent operator role: %v", err)
	}
	type grantOutcome struct {
		result accountoperatorgrant.Result
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan grantOutcome, 2)
	for attempt := 0; attempt < 2; attempt++ {
		go func() {
			<-start
			result, grantErr := accountoperatorgrant.Grant(ctx, pool, accountoperatorgrant.Input{
				RoleCode: "concurrent-operator", Actor: "henukit-release", RequestID: "req_release_aaaaaaaa_concurrent",
				Reason: "Account Portfolio production release aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			})
			outcomes <- grantOutcome{result: result, err: grantErr}
		}()
	}
	close(start)
	changedCount := 0
	for attempt := 0; attempt < 2; attempt++ {
		outcome := <-outcomes
		if outcome.err != nil {
			t.Fatalf("concurrent grant attempt %d: %v", attempt+1, outcome.err)
		}
		if outcome.result.Changed {
			changedCount++
		}
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM role_permissions rp JOIN authorization_roles r ON r.id=rp.role_id WHERE r.code='concurrent-operator' AND rp.permission_code LIKE 'account.%'`).Scan(&grantCount); err != nil {
		t.Fatalf("count concurrent grants: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT revision FROM authorization_roles WHERE code='concurrent-operator'`).Scan(&revision); err != nil {
		t.Fatalf("read concurrent revision: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM account_operator_role_grant_audit_events WHERE role_code='concurrent-operator'`).Scan(&auditCount); err != nil {
		t.Fatalf("count concurrent audits: %v", err)
	}
	if changedCount != 1 || grantCount != 8 || revision != 2 || auditCount != 1 {
		t.Fatalf("concurrent grant facts changed=%d count=%d revision=%d audits=%d", changedCount, grantCount, revision, auditCount)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM account_operator_role_grant_audit_events`); err == nil {
		t.Fatal("account operator grant audit accepted deletion")
	}
}
