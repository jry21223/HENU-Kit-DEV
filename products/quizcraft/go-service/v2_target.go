package quizcraft

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// QuizcraftV2DatabaseName is the independently owned target database for the
// recoverable migration tooling. The operator commands deliberately do not
// accept a generic DATABASE_URL here: accepting a legacy URL would make the
// schema runner capable of mutating the source of truth it is meant to read.
const QuizcraftV2DatabaseName = "quizcraft_v2"

// databaseQueryRower is the smallest common database seam used by the
// command-boundary guards. Both a pooled runtime target and a one-off restore
// connection can prove their selected PostgreSQL database without duplicating
// the identity query.
type databaseQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// RequireQuizcraftV2DatabaseURL rejects a connection string that does not
// explicitly name the V2 target. It is a command-boundary check; callers still
// verify the connected database with RequireQuizcraftV2Target before writing.
func RequireQuizcraftV2DatabaseURL(databaseURL string) error {
	if strings.TrimSpace(databaseURL) == "" {
		return errors.New("QUIZCRAFT_V2_DATABASE_URL is required")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil || config.Database != QuizcraftV2DatabaseName {
		return errors.New("QUIZCRAFT_V2_DATABASE_URL must target the independently owned quizcraft_v2 database")
	}
	return nil
}

// RequireQuizcraftV2Target verifies the server-selected database identity
// after connecting. It protects against a URL/proxy mismatch without exposing
// connection details in command errors or migration evidence.
func RequireQuizcraftV2Target(ctx context.Context, database *pgxpool.Pool) error {
	if database == nil {
		return errors.New("quizcraft_v2 target database is required")
	}
	return requireQuizcraftV2Target(ctx, database)
}

func requireQuizcraftV2Target(ctx context.Context, database databaseQueryRower) error {
	var name string
	if err := database.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		return err
	}
	if name != QuizcraftV2DatabaseName {
		return errors.New("connected database is not the independently owned quizcraft_v2 target")
	}
	return nil
}
