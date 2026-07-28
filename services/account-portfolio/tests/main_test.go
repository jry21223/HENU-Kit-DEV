package tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	accountportfolio "henukit.dev/account-portfolio"
)

func TestMain(m *testing.M) {
	if os.Getenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL") == "" {
		ctx := context.Background()
		container, err := testcontainers.Run(ctx, "postgres:16-alpine",
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
		defer func() { _ = testcontainers.TerminateContainer(container) }()
		host, err := container.Host(ctx)
		if err != nil {
			panic(err)
		}
		port, err := container.MappedPort(ctx, "5432/tcp")
		if err != nil {
			panic(err)
		}
		if err := os.Setenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL", fmt.Sprintf("postgres://account_portfolio:account_portfolio_test@%s:%s/account_portfolio_test?sslmode=disable", host, port.Port())); err != nil {
			panic(err)
		}
	}

	pool, err := pgxpool.New(context.Background(), os.Getenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	if err := accountportfolio.ApplyMigrations(context.Background(), pool); err != nil {
		pool.Close()
		panic(fmt.Errorf("apply Account Portfolio migrations: %w", err))
	}
	pool.Close()
	os.Exit(m.Run())
}

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	value := os.Getenv("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL")
	if value == "" {
		t.Fatal("ACCOUNT_PORTFOLIO_TEST_DATABASE_URL is not configured")
	}
	return value
}

func clearAccountPortfolio(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		TRUNCATE TABLE
			account_portfolio_service_nonces,
			account_portfolio_ticket_messages,
			account_portfolio_tickets,
			account_portfolio_notifications,
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
