package authretention

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/platform-core/internal/store"
)

const verificationRetention = 24 * time.Hour

type Result struct {
	VerificationRecordsScrubbed int64
	ExchangeIdempotencyDeleted  int64
}

func Cleanup(ctx context.Context, database *pgxpool.Pool, now time.Time) (Result, error) {
	if database == nil || now.IsZero() {
		return Result{}, errors.New("database and cleanup time are required")
	}
	tx, err := database.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := store.New(tx)
	verificationRows, err := queries.ScrubExpiredVerificationSecrets(ctx, pgtype.Timestamptz{Time: now.UTC().Add(-verificationRetention), Valid: true})
	if err != nil {
		return Result{}, err
	}
	idempotencyRows, err := queries.DeleteExpiredOAuthExchangeIdempotency(ctx, pgtype.Timestamptz{Time: now.UTC(), Valid: true})
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{VerificationRecordsScrubbed: verificationRows, ExchangeIdempotencyDeleted: idempotencyRows}, nil
}
