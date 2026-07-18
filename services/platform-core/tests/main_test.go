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

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	if os.Getenv("PLATFORM_CORE_TEST_DATABASE_URL") != "" && os.Getenv("PLATFORM_CORE_TEST_REDIS_ADDR") != "" {
		return m.Run()
	}
	ctx := context.Background()
	postgres, err := testcontainers.Run(ctx, "postgres:17-alpine",
		testcontainers.WithEnv(map[string]string{
			"POSTGRES_DB": "platform_core_test", "POSTGRES_USER": "platform_core", "POSTGRES_PASSWORD": "platform_core",
		}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		panic(fmt.Errorf("start PostgreSQL testcontainer: %w", err))
	}
	defer func() { _ = testcontainers.TerminateContainer(postgres) }()
	postgresHost, err := postgres.Host(ctx)
	if err != nil {
		panic(err)
	}
	postgresPort, err := postgres.MappedPort(ctx, "5432/tcp")
	if err != nil {
		panic(err)
	}
	databaseURL := fmt.Sprintf("postgres://platform_core:platform_core@%s:%s/platform_core_test?sslmode=disable", postgresHost, postgresPort.Port())
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		panic(fmt.Errorf("connect PostgreSQL testcontainer: %w", err))
	}
	migrations, err := filepath.Glob("../db/migrations/*.up.sql")
	if err != nil {
		panic(err)
	}
	sort.Strings(migrations)
	for _, migrationPath := range migrations {
		migration, readErr := os.ReadFile(migrationPath)
		if readErr != nil {
			panic(readErr)
		}
		if _, execErr := connection.Exec(ctx, string(migration)); execErr != nil {
			panic(fmt.Errorf("apply test migration %s: %w", migrationPath, execErr))
		}
	}
	_ = connection.Close(ctx)

	redisContainer, err := testcontainers.Run(ctx, "redis:7-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("Ready to accept connections").WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		panic(fmt.Errorf("start Redis testcontainer: %w", err))
	}
	defer func() { _ = testcontainers.TerminateContainer(redisContainer) }()
	redisHost, err := redisContainer.Host(ctx)
	if err != nil {
		panic(err)
	}
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("PLATFORM_CORE_TEST_DATABASE_URL", databaseURL)
	_ = os.Setenv("PLATFORM_CORE_TEST_REDIS_ADDR", fmt.Sprintf("%s:%s", redisHost, redisPort.Port()))
	return m.Run()
}
