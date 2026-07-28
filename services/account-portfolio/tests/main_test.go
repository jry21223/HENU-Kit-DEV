package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	accountportfolio "henukit.dev/account-portfolio"
)

func TestMain(m *testing.M) {
	var container testcontainers.Container
	cleanupContainer := func() {
		if container != nil {
			_ = testcontainers.TerminateContainer(container)
		}
	}
	if os.Getenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL") == "" {
		ctx := context.Background()
		started, err := testcontainers.Run(ctx, "postgres:16-alpine",
			testcontainers.WithEnv(map[string]string{
				"POSTGRES_DB":       "account_portfolio_test",
				"POSTGRES_USER":     "account_portfolio",
				"POSTGRES_PASSWORD": "account_portfolio_test",
			}),
			testcontainers.WithExposedPorts("5432/tcp"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
		)
		if err != nil {
			panic(fmt.Errorf("start Account Portfolio PostgreSQL: %w", err))
		}
		container = started
		host, err := container.Host(ctx)
		if err != nil {
			cleanupContainer()
			panic(err)
		}
		port, err := container.MappedPort(ctx, "5432/tcp")
		if err != nil {
			cleanupContainer()
			panic(err)
		}
		if err := os.Setenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL", fmt.Sprintf("postgres://account_portfolio:account_portfolio_test@%s:%s/account_portfolio_test?sslmode=disable", host, port.Port())); err != nil {
			cleanupContainer()
			panic(err)
		}
	}

	pool, err := pgxpool.New(context.Background(), os.Getenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	if err := accountportfolio.ApplyMigrations(context.Background(), pool); err != nil {
		pool.Close()
		cleanupContainer()
		panic(fmt.Errorf("apply Account Portfolio migrations: %w", err))
	}
	pool.Close()
	status := m.Run()
	cleanupContainer()
	os.Exit(status)
}

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	value := os.Getenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL")
	if value == "" {
		t.Fatal("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL is not configured")
	}
	return value
}

func TestConcurrentMigrationStartupSerializesVersionRecording(t *testing.T) {
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	schema := fmt.Sprintf("account_portfolio_migration_race_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	}()

	newPool := func() *pgxpool.Pool {
		config, err := pgxpool.ParseConfig(testDatabaseURL(t))
		if err != nil {
			t.Fatal(err)
		}
		config.ConnConfig.RuntimeParams["search_path"] = schema
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		return pool
	}
	first, second := newPool(), newPool()
	defer first.Close()
	defer second.Close()

	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, pool := range []*pgxpool.Pool{first, second} {
		group.Add(1)
		go func(pool *pgxpool.Pool) {
			defer group.Done()
			errs <- accountportfolio.ApplyMigrations(ctx, pool)
		}(pool)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ApplyMigrations() = %v", err)
		}
	}

	var applied int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=$1 AND table_name IN ('account_portfolio_accounts', 'account_portfolio_schema_migrations')`, schema).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Fatalf("migration tables in %s = %d, want 2", schema, applied)
	}

}

func TestRollbackClearsVersionRecordSoServiceCanReconcileSchema(t *testing.T) {
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	schema := fmt.Sprintf("account_portfolio_migration_rollback_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	}()

	config, err := pgxpool.ParseConfig(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := accountportfolio.ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("initial ApplyMigrations() = %v", err)
	}

	for _, migration := range []string{"000004_membership_order_payment_kernel.down.sql", "000003_membership_entitlements.down.sql", "000002_support_ticket_commands.down.sql", "000001_account_portfolio.down.sql"} {
		down, err := os.ReadFile(filepath.Join("..", "db", "migrations", migration))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(down)); err != nil {
			t.Fatalf("apply rollback %s = %v", migration, err)
		}
	}
	if err := accountportfolio.ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("reconcile after rollback = %v", err)
	}

	var accountTable, commandsTable, membershipEventsTable, paymentIntentsTable, paymentFactsTable, initialVersionRecorded, supportCommandsVersionRecorded, membershipEntitlementsVersionRecorded, paymentKernelVersionRecorded bool
	if err := pool.QueryRow(ctx, `
		SELECT
			to_regclass('account_portfolio_accounts') IS NOT NULL,
			to_regclass('account_portfolio_command_idempotency') IS NOT NULL,
			to_regclass('account_portfolio_membership_events') IS NOT NULL,
			to_regclass('account_portfolio_payment_order_intents') IS NOT NULL,
			to_regclass('account_portfolio_payment_facts') IS NOT NULL,
			EXISTS(SELECT 1 FROM account_portfolio_schema_migrations WHERE version='000001_account_portfolio'),
			EXISTS(SELECT 1 FROM account_portfolio_schema_migrations WHERE version='000002_support_ticket_commands'),
			EXISTS(SELECT 1 FROM account_portfolio_schema_migrations WHERE version='000003_membership_entitlements'),
			EXISTS(SELECT 1 FROM account_portfolio_schema_migrations WHERE version='000004_membership_order_payment_kernel')
	`).Scan(&accountTable, &commandsTable, &membershipEventsTable, &paymentIntentsTable, &paymentFactsTable, &initialVersionRecorded, &supportCommandsVersionRecorded, &membershipEntitlementsVersionRecorded, &paymentKernelVersionRecorded); err != nil {
		t.Fatal(err)
	}
	if !accountTable || !commandsTable || !membershipEventsTable || !paymentIntentsTable || !paymentFactsTable || !initialVersionRecorded || !supportCommandsVersionRecorded || !membershipEntitlementsVersionRecorded || !paymentKernelVersionRecorded {
		t.Fatalf("reconciled schema account_table=%t commands_table=%t membership_events_table=%t payment_intents_table=%t payment_facts_table=%t initial_version=%t support_commands_version=%t membership_entitlements_version=%t payment_kernel_version=%t, want all true", accountTable, commandsTable, membershipEventsTable, paymentIntentsTable, paymentFactsTable, initialVersionRecorded, supportCommandsVersionRecorded, membershipEntitlementsVersionRecorded, paymentKernelVersionRecorded)
	}
}

func TestMembershipEntitlementMigrationPreservesPreexistingMembership(t *testing.T) {
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	schema := fmt.Sprintf("account_portfolio_membership_migration_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	}()

	config, err := pgxpool.ParseConfig(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := accountportfolio.ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("initial ApplyMigrations() = %v", err)
	}

	down, err := os.ReadFile(filepath.Join("..", "db", "migrations", "000003_membership_entitlements.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback membership entitlement migration = %v", err)
	}

	const ownerID = "abababab-abab-4bab-8bab-abababababab"
	if _, err := pool.Exec(ctx, `INSERT INTO account_portfolio_accounts(user_id) VALUES($1)`, ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO account_portfolio_memberships(user_id, plan, source) VALUES($1, 'lifetime', 'legacy_operator')`, ownerID); err != nil {
		t.Fatal(err)
	}
	if err := accountportfolio.ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("ApplyMigrations() after pre-entitlement seed = %v", err)
	}

	var plan string
	var version, eventCount int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT version FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID).Scan(&plan, &version, &eventCount); err != nil {
		t.Fatal(err)
	}
	if plan != "lifetime" || version != 1 || eventCount != 0 {
		t.Fatalf("migrated preexisting membership plan/version/events = %q/%d/%d, want lifetime/1/0", plan, version, eventCount)
	}
}

func clearAccountPortfolio(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE
			account_portfolio_payment_audits,
			account_portfolio_command_idempotency,
			account_portfolio_service_nonces,
			account_portfolio_ticket_events,
			account_portfolio_ticket_messages,
			account_portfolio_tickets,
			account_portfolio_notifications,
			account_portfolio_membership_events,
			account_portfolio_payment_facts,
			account_portfolio_payment_order_intents,
			account_portfolio_membership_orders,
			account_portfolio_point_ledger,
			account_portfolio_memberships,
			account_portfolio_points,
			account_portfolio_accounts
	`)
	if err != nil {
		t.Fatal(err)
	}
}
