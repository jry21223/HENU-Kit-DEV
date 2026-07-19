package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMain(m *testing.M) { os.Exit(runTests(m)) }

func runTests(m *testing.M) int {
	if os.Getenv("LIBRARY_TEST_DATABASE_URL") != "" {
		return m.Run()
	}
	ctx := context.Background()
	postgres, err := testcontainers.Run(ctx, "postgres:16-alpine",
		testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "library_test", "POSTGRES_USER": "library", "POSTGRES_PASSWORD": "library"}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		panic(fmt.Errorf("start Library PostgreSQL: %w", err))
	}
	defer func() { _ = testcontainers.TerminateContainer(postgres) }()
	host, err := postgres.Host(ctx)
	if err != nil {
		panic(err)
	}
	port, err := postgres.MappedPort(ctx, "5432/tcp")
	if err != nil {
		panic(err)
	}
	databaseURL := fmt.Sprintf("postgres://library:library@%s:%s/library_test?sslmode=disable", host, port.Port())
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		panic(err)
	}
	migrations, err := filepath.Glob("../db/migrations/*.up.sql")
	if err != nil {
		panic(err)
	}
	sort.Strings(migrations)
	for _, path := range migrations {
		migration, readErr := os.ReadFile(path)
		if readErr != nil {
			panic(readErr)
		}
		if _, execErr := connection.Exec(ctx, string(migration)); execErr != nil {
			panic(fmt.Errorf("apply %s: %w", path, execErr))
		}
	}
	_ = connection.Close(ctx)
	_ = os.Setenv("LIBRARY_TEST_DATABASE_URL", databaseURL)
	return m.Run()
}
