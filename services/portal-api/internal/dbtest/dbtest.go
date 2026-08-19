// Package dbtest opens the PostgreSQL database the portal-api tests run
// against and applies this service's own migrations to it.
//
// Tests skip when PORTAL_API_TEST_DATABASE_URL is unset so `go test ./...`
// stays runnable without a database; CI always sets it, so the tests are never
// silently skipped there.
package dbtest

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// EnvKey names the DSN the suite connects to.
const EnvKey = "PORTAL_API_TEST_DATABASE_URL"

var schemaCounter atomic.Uint64

// Open returns a migrated, empty database.
//
// Every test gets its own PostgreSQL schema, so tests are isolated from each
// other and — importantly — from the other package test binaries `go test ./...`
// runs in parallel against the same database. The schema is dropped when the
// test ends.
func Open(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv(EnvKey)
	if dsn == "" {
		t.Skipf("set %s to run the portal-api database tests", EnvKey)
	}

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open %s: %v", EnvKey, err)
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		t.Fatalf("ping %s: %v", EnvKey, err)
	}

	schema := fmt.Sprintf("portal_api_test_%d_%d", os.Getpid(), schemaCounter.Add(1))
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", dsn)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
	})

	conn, err := sql.Open("pgx", withSearchPath(t, dsn, schema))
	if err != nil {
		t.Fatalf("open %s in schema %s: %v", EnvKey, schema, err)
	}
	if err := conn.Ping(); err != nil {
		conn.Close()
		t.Fatalf("ping schema %s: %v", schema, err)
	}
	t.Cleanup(func() { conn.Close() })

	migrate(t, conn)
	return conn
}

// withSearchPath pins every connection in the pool to the test's own schema, so
// the unqualified table names in the migrations resolve there.
func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", EnvKey, err)
	}
	query := parsed.Query()
	query.Set("options", "-c search_path="+schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func migrate(t *testing.T, conn *sql.DB) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the migrations directory")
	}
	pattern := filepath.Join(filepath.Dir(thisFile), "..", "..", "db", "migrations", "postgres", "*.up.sql")
	migrations, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Fatalf("no migrations matched %s", pattern)
	}
	sort.Strings(migrations)

	for _, path := range migrations {
		statements, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if _, err := conn.Exec(string(statements)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(path), err)
		}
	}
}
