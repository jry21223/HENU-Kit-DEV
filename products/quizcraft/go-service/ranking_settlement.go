package quizcraft

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"henukit.dev/quizcraft/internal/store"
)

// SettlePreviousUTCWeek records immutable public standings for the last complete
// UTC Monday-to-Monday period. It intentionally has no reward side effects.
func (service *Service) SettlePreviousUTCWeek(ctx context.Context, at time.Time) (int, error) {
	current := at.UTC()
	periodEnd := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(int(current.Weekday())+6)%7)
	periodStart := periodEnd.AddDate(0, 0, -7)
	tx, err := service.database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := store.New(tx)
	if err := queries.LockIdempotency(ctx, "ranking-settlement:"+periodEnd.Format(time.RFC3339)); err != nil {
		return 0, err
	}
	start := pgtype.Timestamptz{Time: periodStart, Valid: true}
	end := pgtype.Timestamptz{Time: periodEnd, Valid: true}
	overall, err := queries.ListOverallRankingWindow(ctx, store.ListOverallRankingWindowParams{SubmittedAt: start, SubmittedAt_2: end})
	if err != nil {
		return 0, err
	}
	encoded, _ := json.Marshal(overall)
	created, err := queries.CreateRankingSettlementEvent(ctx, store.CreateRankingSettlementEventParams{ID: uuid.New(), PeriodStart: start, PeriodEnd: end, Scope: "overall", Standings: encoded})
	if err != nil {
		return 0, err
	}
	count := int(created)
	banks, err := queries.ListScoredBankIDsWindow(ctx, store.ListScoredBankIDsWindowParams{SubmittedAt: start, SubmittedAt_2: end})
	if err != nil {
		return 0, err
	}
	for _, bankID := range banks {
		rows, queryErr := queries.ListBankRankingWindow(ctx, store.ListBankRankingWindowParams{BankID: bankID, SubmittedAt: start, SubmittedAt_2: end})
		if queryErr != nil {
			return 0, queryErr
		}
		standings, _ := json.Marshal(rows)
		created, err := queries.CreateRankingSettlementEvent(ctx, store.CreateRankingSettlementEventParams{ID: uuid.New(), PeriodStart: start, PeriodEnd: end, Scope: "bank", BankID: uuid.NullUUID{UUID: bankID, Valid: true}, Standings: standings})
		if err != nil {
			return 0, fmt.Errorf("settle bank %s: %w", bankID, err)
		}
		count += int(created)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}
