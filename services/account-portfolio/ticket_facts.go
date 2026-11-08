package accountportfolio

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
)

type ticketFactReference struct {
	ID        string    `json:"id"`
	Reference string    `json:"reference"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}
type ticketFacts struct {
	Scope  string    `json:"scope"`
	AsOf   time.Time `json:"as_of"`
	Stale  bool      `json:"stale"`
	Counts struct {
		Open       int64 `json:"open"`
		InProgress int64 `json:"in_progress"`
		Resolved   int64 `json:"resolved"`
		Unresolved int64 `json:"unresolved"`
	} `json:"counts"`
	Window struct {
		Filter       string                `json:"filter"`
		MatchedTotal int64                 `json:"matched_total"`
		Limit        int                   `json:"limit"`
		Returned     int                   `json:"returned"`
		HasMore      bool                  `json:"has_more"`
		Items        []ticketFactReference `json:"items"`
	} `json:"window"`
}

func (h *service) consoleTicketFacts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	filter, invalid := parseTicketFactsFilter(r)
	if invalid {
		writeError(w, r, 400, "INVALID_REQUEST", "ticket facts filter is invalid")
		return
	}
	data, readErr := h.readTicketFacts(ctx, filter)
	if readErr != nil {
		writeError(w, r, 503, "DEPENDENCY_UNAVAILABLE", "Account Portfolio ticket facts are unavailable")
		return
	}
	writeData(w, r, 200, data)
}

func parseTicketFactsFilter(r *http.Request) (string, bool) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) > 1 {
		return "", true
	}
	for key := range query {
		if key != "status" {
			return "", true
		}
	}
	status := "unresolved"
	if values, ok := query["status"]; ok {
		if len(values) != 1 {
			return "", true
		}
		status = values[0]
	}
	switch status {
	case "unresolved", "all", "open", "in_progress", "resolved":
		return status, false
	}
	return "", true
}

func (h *service) readTicketFacts(ctx context.Context, filter string) (ticketFacts, error) {
	var data ticketFacts
	tx, err := h.database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return data, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	data.Scope = "global"
	data.Window.Filter = filter
	data.Window.Limit = 100
	data.Window.Items = []ticketFactReference{}
	err = tx.QueryRow(ctx, `SELECT transaction_timestamp(),count(*) FILTER (WHERE status='open'),count(*) FILTER (WHERE status='in_progress'),count(*) FILTER (WHERE status='resolved') FROM account_portfolio_tickets`).Scan(&data.AsOf, &data.Counts.Open, &data.Counts.InProgress, &data.Counts.Resolved)
	if err != nil {
		return ticketFacts{}, err
	}
	data.AsOf = data.AsOf.UTC()
	data.Counts.Unresolved = data.Counts.Open + data.Counts.InProgress
	switch filter {
	case "all":
		data.Window.MatchedTotal = data.Counts.Unresolved + data.Counts.Resolved
	case "unresolved":
		data.Window.MatchedTotal = data.Counts.Unresolved
	case "open":
		data.Window.MatchedTotal = data.Counts.Open
	case "in_progress":
		data.Window.MatchedTotal = data.Counts.InProgress
	case "resolved":
		data.Window.MatchedTotal = data.Counts.Resolved
	}
	rows, err := tx.Query(ctx, `SELECT id,title,status,version,updated_at FROM account_portfolio_tickets WHERE $1='all' OR ($1='unresolved' AND status IN ('open','in_progress')) OR status=$1 ORDER BY updated_at DESC,id DESC LIMIT 100`, filter)
	if err != nil {
		return ticketFacts{}, err
	}
	for rows.Next() {
		var item ticketFactReference
		if err := rows.Scan(&item.ID, &item.Title, &item.Status, &item.Version, &item.UpdatedAt); err != nil {
			rows.Close()
			return ticketFacts{}, err
		}
		item.Reference = ticketReference(item.ID)
		data.Window.Items = append(data.Window.Items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ticketFacts{}, err
	}
	data.Window.Returned = len(data.Window.Items)
	data.Window.HasMore = data.Window.MatchedTotal > int64(data.Window.Returned)
	if err := tx.Commit(ctx); err != nil {
		return ticketFacts{}, err
	}
	return data, nil
}
