package accountportfolio

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func consoleReadLimit(query url.Values, fallback, maximum int) (int, bool) {
	values, present := query["limit"]
	if !present {
		return fallback, true
	}
	if len(values) != 1 {
		return 0, false
	}
	limit, err := strconv.Atoi(values[0])
	return limit, err == nil && limit >= 1 && limit <= maximum
}

func (h *service) consolePoints(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	userID, failure := membershipTargetID(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	limit, ok := consoleReadLimit(query, 20, 50)
	if err != nil || !ok || len(query) > 0 && !query.Has("limit") || len(query) > 1 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Points limit is invalid")
		return
	}
	ctx := r.Context()
	tx, err := h.database.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err == nil {
		defer tx.Rollback(ctx)
		var data struct {
			Balance int64                  `json:"balance"`
			Entries []pointLedgerEntryView `json:"entries"`
		}
		err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(amount), 0) FROM account_portfolio_point_ledger WHERE user_id = $1`, userID).Scan(&data.Balance)
		if err == nil {
			var rows pgx.Rows
			rows, err = tx.Query(ctx, `SELECT id::text, amount, reason, created_at FROM account_portfolio_point_ledger WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, limit)
			if err == nil {
				data.Entries, err = pgx.CollectRows(rows, pgx.RowToStructByPos[pointLedgerEntryView])
				if data.Entries == nil {
					data.Entries = []pointLedgerEntryView{}
				}
			}
		}
		if err == nil && tx.Commit(ctx) == nil {
			writeData(w, r, http.StatusOK, data)
			return
		}
	}
	writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
}

type consoleOrderView struct {
	ID           string          `json:"id"`
	UserID       string          `json:"user_id"`
	Plan         string          `json:"plan"`
	AmountCents  int64           `json:"amount_cents"`
	Status       string          `json:"status"`
	Version      int             `json:"version"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	PaidAt       *time.Time      `json:"paid_at,omitempty"`
	ClosedAt     *time.Time      `json:"closed_at,omitempty"`
	RefundedAt   *time.Time      `json:"refunded_at,omitempty"`
	LatestRefund json.RawMessage `json:"latest_refund,omitempty"`
}

func consoleOrderFilters(raw string) (string, string, int, bool) {
	query, err := url.ParseQuery(raw)
	limit, ok := consoleReadLimit(query, 50, 100)
	if err != nil || !ok {
		return "", "", 0, false
	}
	for key, values := range query {
		if (key != "status" && key != "user_id" && key != "limit") || len(values) != 1 || values[0] == "" {
			return "", "", 0, false
		}
	}
	status, userID := query.Get("status"), query.Get("user_id")
	switch status {
	case "all":
		status = ""
	case "", "created", "pending_payment", "paid", "closed", "refunded":
	default:
		return "", "", 0, false
	}
	if userID != "" {
		parsed, err := uuid.Parse(userID)
		if err != nil {
			return "", "", 0, false
		}
		userID = parsed.String()
	}
	return status, userID, limit, true
}

func (h *service) consoleMembershipOrders(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	status, userID, limit, ok := consoleOrderFilters(r.URL.RawQuery)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Membership order filters are invalid")
		return
	}
	rows, err := h.database.Query(r.Context(), `
		SELECT orders.id::text, orders.user_id::text, orders.plan, orders.amount_cents, orders.status,
		       orders.version, orders.created_at, orders.updated_at,
		       facts.paid_at, facts.closed_at, facts.refunded_at, refund.summary
		FROM (
			SELECT id, user_id, plan, amount_cents, status, version, created_at, updated_at
			FROM account_portfolio_membership_orders
			WHERE ($1 = '' OR status = $1) AND (NULLIF($2, '') IS NULL OR user_id = NULLIF($2, '')::uuid)
			ORDER BY created_at DESC, id DESC LIMIT $3
		) AS orders
		LEFT JOIN LATERAL (
			SELECT min(occurred_at) FILTER (WHERE status = 'paid') AS paid_at,
			       min(occurred_at) FILTER (WHERE status = 'closed') AS closed_at,
			       min(occurred_at) FILTER (WHERE status = 'refunded') AS refunded_at
			FROM account_portfolio_payment_facts WHERE order_id = orders.id
		) AS facts ON true
		LEFT JOIN LATERAL (
			SELECT json_build_object('id', id, 'status', status, 'amount_cents', amount_cents) AS summary
			FROM account_portfolio_membership_order_refunds WHERE order_id = orders.id
			ORDER BY created_at DESC, id DESC LIMIT 1
		) AS refund ON true
		ORDER BY orders.created_at DESC, orders.id DESC`, status, userID, limit)
	if err == nil {
		orders, collectErr := pgx.CollectRows(rows, pgx.RowToStructByPos[consoleOrderView])
		if collectErr == nil {
			if orders == nil {
				orders = []consoleOrderView{}
			}
			writeData(w, r, http.StatusOK, map[string]any{"orders": orders})
			return
		}
	}
	writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio membership orders are unavailable")
}
