package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect opens a database connection using the given env var.
// Supports PostgreSQL (postgres://) and MySQL (mysql://) DSN formats.
//
// In mock mode, an empty DSN returns (nil, nil) so handlers may use fixtures.
// In live mode, an empty DSN or ping failure always returns a non-nil error.
func Connect(envKey string) (*sql.DB, error) {
	dsn := strings.TrimSpace(os.Getenv(envKey))
	if dsn == "" {
		if IsLive() {
			return nil, fmt.Errorf("%s is required in live mode", envKey)
		}
		return nil, nil
	}

	var driver, dsnClean string
	if strings.HasPrefix(dsn, "mysql://") {
		driver = "mysql"
		dsnClean = strings.TrimPrefix(dsn, "mysql://")
	} else {
		driver = "pgx"
		dsnClean = dsn
	}

	conn, err := sql.Open(driver, dsnClean)
	if err != nil {
		return nil, fmt.Errorf("sql.Open(%s, %s): %w", driver, envKey, err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ping(%s): %w", envKey, err)
	}
	return conn, nil
}
