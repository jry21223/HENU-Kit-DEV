package quizcraft

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"henukit.dev/quizcraft/internal/store"
)

// LearningStateReconciliation describes whether the derived learning-state
// projection can be reconstructed exactly from immutable practice attempts.
// It deliberately contains counts only: callers must not expose another
// learner's state while auditing a projection.
type LearningStateReconciliation struct {
	MissingRows    int64
	ExtraRows      int64
	MismatchedRows int64
}

// Clean reports whether the derived projection matches its immutable source.
func (result LearningStateReconciliation) Clean() bool {
	return result.MissingRows == 0 && result.ExtraRows == 0 && result.MismatchedRows == 0
}

// ReconcileLearningState compares the derived learning-state projection with
// the immutable answer facts. It does not mutate either data set.
func ReconcileLearningState(ctx context.Context, database *pgxpool.Pool) (LearningStateReconciliation, error) {
	if database == nil {
		return LearningStateReconciliation{}, errors.New("QuizCraft database is required")
	}
	return reconcileLearningState(ctx, store.New(database))
}

// RebuildLearningState restores the derived learning-state projection from
// immutable practice attempts. It intentionally takes table locks, so the
// caller must run it only during an approved technical write freeze. There is
// no public HTTP route for this operator repair; it never changes attempts.
func RebuildLearningState(ctx context.Context, database *pgxpool.Pool) (LearningStateReconciliation, error) {
	if database == nil {
		return LearningStateReconciliation{}, errors.New("QuizCraft database is required")
	}
	tx, err := database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return LearningStateReconciliation{}, fmt.Errorf("begin learning-state rebuild: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Practice submissions write attempts before the projection. Acquiring this
	// lock first establishes the same order and prevents a partial rebuild.
	if _, err := tx.Exec(ctx, "LOCK TABLE quizcraft_practice_attempts IN SHARE MODE"); err != nil {
		return LearningStateReconciliation{}, fmt.Errorf("lock immutable practice attempts: %w", err)
	}
	if _, err := tx.Exec(ctx, "LOCK TABLE quizcraft_learning_state IN ACCESS EXCLUSIVE MODE"); err != nil {
		return LearningStateReconciliation{}, fmt.Errorf("lock learning-state projection: %w", err)
	}

	queries := store.New(tx)
	if err := queries.ClearLearningState(ctx); err != nil {
		return LearningStateReconciliation{}, fmt.Errorf("clear learning-state projection: %w", err)
	}
	if err := queries.RebuildLearningStateFromAttempts(ctx); err != nil {
		return LearningStateReconciliation{}, fmt.Errorf("rebuild learning-state projection: %w", err)
	}
	reconciliation, err := reconcileLearningState(ctx, queries)
	if err != nil {
		return LearningStateReconciliation{}, err
	}
	if !reconciliation.Clean() {
		return reconciliation, errors.New("rebuilt learning-state projection failed reconciliation")
	}
	if err := tx.Commit(ctx); err != nil {
		return LearningStateReconciliation{}, fmt.Errorf("commit learning-state rebuild: %w", err)
	}
	return reconciliation, nil
}

type learningStateQuerier interface {
	GetLearningStateReconciliation(context.Context) (store.GetLearningStateReconciliationRow, error)
}

func reconcileLearningState(ctx context.Context, queries learningStateQuerier) (LearningStateReconciliation, error) {
	row, err := queries.GetLearningStateReconciliation(ctx)
	if err != nil {
		return LearningStateReconciliation{}, fmt.Errorf("reconcile learning-state projection: %w", err)
	}
	return LearningStateReconciliation{
		MissingRows:    row.MissingRows,
		ExtraRows:      row.ExtraRows,
		MismatchedRows: row.MismatchedRows,
	}, nil
}
