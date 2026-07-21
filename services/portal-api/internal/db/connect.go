package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Connect opens a PostgreSQL connection using the given env var.
// Returns nil if the env var is empty (graceful degradation to mock mode).
func Connect(envKey string) (*sql.DB, error) {
	dsn := os.Getenv(envKey)
	if dsn == "" {
		return nil, nil
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open(%s): %w", envKey, err)
	}
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping(%s): %w", envKey, err)
	}
	return conn, nil
}
