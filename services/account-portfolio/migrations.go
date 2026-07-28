package accountportfolio

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationFiles is compiled into the service so a deployed image can apply
// the exact schema it was built and reviewed with.
//
//go:embed db/migrations/*.up.sql
var migrationFiles embed.FS

// ApplyMigrations applies each numbered Account Portfolio migration exactly
// once. The migrations themselves remain idempotent so a restored database
// without metadata can be reconciled safely before the version is recorded.
func ApplyMigrations(ctx context.Context, database *pgxpool.Pool) error {
	if database == nil {
		return fmt.Errorf("account portfolio database is required")
	}
	if _, err := database.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS account_portfolio_schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create migration metadata: %w", err)
	}

	paths, err := fs.Glob(migrationFiles, "db/migrations/*.up.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		version, ok := strings.CutSuffix(strings.TrimPrefix(path, "db/migrations/"), ".up.sql")
		if !ok || version == "" {
			return fmt.Errorf("invalid migration path %q", path)
		}
		var applied bool
		if err := database.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_portfolio_schema_migrations WHERE version=$1)`, version).Scan(&applied); err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		if applied {
			continue
		}
		contents, err := migrationFiles.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		tx, err := database.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO account_portfolio_schema_migrations(version, applied_at) VALUES($1, $2)`, version, time.Now().UTC()); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}
	return nil
}
