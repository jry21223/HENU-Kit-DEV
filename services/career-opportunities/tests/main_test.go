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
	if os.Getenv("CAREER_TEST_DATABASE_URL") != "" && os.Getenv("CAREER_TEST_REDIS_ADDR") != "" {
		return m.Run()
	}
	ctx := context.Background()
	postgres, err := testcontainers.Run(ctx, "postgres:16-alpine",
		testcontainers.WithEnv(map[string]string{"POSTGRES_DB": "career_test", "POSTGRES_USER": "career", "POSTGRES_PASSWORD": "career"}),
		testcontainers.WithExposedPorts("5432/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		panic(fmt.Errorf("start Career PostgreSQL: %w", err))
	}
	defer func() { _ = testcontainers.TerminateContainer(postgres) }()
	redisContainer, err := testcontainers.Run(ctx, "redis:7-alpine",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("Ready to accept connections").WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		panic(fmt.Errorf("start Career Redis: %w", err))
	}
	defer func() { _ = testcontainers.TerminateContainer(redisContainer) }()
	host, err := postgres.Host(ctx)
	if err != nil {
		panic(err)
	}
	port, err := postgres.MappedPort(ctx, "5432/tcp")
	if err != nil {
		panic(err)
	}
	databaseURL := fmt.Sprintf("postgres://career:career@%s:%s/career_test?sslmode=disable", host, port.Port())
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
	redisHost, err := redisContainer.Host(ctx)
	if err != nil {
		panic(err)
	}
	redisPort, err := redisContainer.MappedPort(ctx, "6379/tcp")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("CAREER_TEST_DATABASE_URL", databaseURL)
	_ = os.Setenv("CAREER_TEST_REDIS_ADDR", redisHost+":"+redisPort.Port())
	return m.Run()
}
