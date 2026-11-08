package food

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const foodFactWindowLimit = 200

type foodFactReference struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type foodFactWindow struct {
	Filter   string              `json:"filter"`
	Limit    int                 `json:"limit"`
	Returned int                 `json:"returned"`
	HasMore  bool                `json:"has_more"`
	Items    []foodFactReference `json:"items"`
}

type foodFacts struct {
	Scope  string    `json:"scope"`
	AsOf   time.Time `json:"as_of"`
	Stale  bool      `json:"stale"`
	Counts struct {
		PendingSubmissions     int64 `json:"pending_submissions"`
		OpenAnomalies          int64 `json:"open_anomalies"`
		PendingTierAdjustments int64 `json:"pending_tier_adjustments"`
	} `json:"counts"`
	Windows struct {
		Submissions     foodFactWindow `json:"submissions"`
		Anomalies       foodFactWindow `json:"anomalies"`
		TierAdjustments foodFactWindow `json:"tier_adjustments"`
	} `json:"windows"`
}

func (h *service) facts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if _, ok := h.require(w, r, "food.read"); !ok {
		return
	}
	// This read describes the whole Food operating scope. Campus-filtered
	// Workspace responses retain their existing, different contract.
	if r.URL.RawQuery != "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Food facts use the global scope")
		return
	}
	data, err := h.readFacts(r.Context())
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Food facts are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) readFacts(ctx context.Context) (foodFacts, error) {
	var data foodFacts
	tx, err := h.database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return data, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	data.Scope = "global"
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp(),
		(SELECT count(*) FROM food_submissions WHERE status='pending'),
		(SELECT count(*) FROM food_anomaly_tickets WHERE status='open'),
		(SELECT count(*) FROM food_tier_adjustments WHERE status='pending')`).Scan(&data.AsOf, &data.Counts.PendingSubmissions, &data.Counts.OpenAnomalies, &data.Counts.PendingTierAdjustments); err != nil {
		return foodFacts{}, err
	}
	data.AsOf = data.AsOf.UTC()
	for _, read := range []struct {
		target        *foodFactWindow
		query, filter string
		total         int64
	}{
		{&data.Windows.Submissions, `SELECT id,status,version,updated_at FROM food_submissions WHERE status='pending' ORDER BY updated_at DESC,id DESC LIMIT 200`, "pending", data.Counts.PendingSubmissions},
		{&data.Windows.Anomalies, `SELECT id,status,version,updated_at FROM food_anomaly_tickets WHERE status='open' ORDER BY updated_at DESC,id DESC LIMIT 200`, "open", data.Counts.OpenAnomalies},
		{&data.Windows.TierAdjustments, `SELECT id,status,version,updated_at FROM food_tier_adjustments WHERE status='pending' ORDER BY updated_at DESC,id DESC LIMIT 200`, "pending", data.Counts.PendingTierAdjustments},
	} {
		window, err := readFoodFactWindow(ctx, tx, read.query, read.filter, read.total)
		if err != nil {
			return foodFacts{}, err
		}
		*read.target = window
	}
	if err := tx.Commit(ctx); err != nil {
		return foodFacts{}, err
	}
	return data, nil
}

func readFoodFactWindow(ctx context.Context, tx pgx.Tx, query, filter string, total int64) (foodFactWindow, error) {
	window := foodFactWindow{Filter: filter, Limit: foodFactWindowLimit, Items: []foodFactReference{}}
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return window, err
	}
	defer rows.Close()
	for rows.Next() {
		var item foodFactReference
		if err := rows.Scan(&item.ID, &item.Status, &item.Version, &item.UpdatedAt); err != nil {
			return window, err
		}
		window.Items = append(window.Items, item)
	}
	if err := rows.Err(); err != nil {
		return window, err
	}
	window.Returned = len(window.Items)
	window.HasMore = total > int64(window.Returned)
	return window, nil
}
