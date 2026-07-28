// Package accountportfolio owns persistent user points, membership,
// membership orders, notifications, and support tickets. It deliberately does
// not read or translate legacy Study account state.
package accountportfolio

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/account-portfolio/internal/contract"
)

var requestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)

// Config contains only private service-to-service configuration. Browser
// clients always go through Portal Gateway and never receive these values.
type Config struct {
	Database *pgxpool.Pool
	ClientID string
	Keys     map[string]string
	Now      func() time.Time
}

type service struct {
	database *pgxpool.Pool
	clientID string
	keys     map[string]string
	now      func() time.Time
}

type actor struct{ userID string }
type contextKey string

const (
	requestIDKey contextKey = "request-id"
	actorKey     contextKey = "actor"
)

// New creates an independently deployable Account Portfolio HTTP handler.
func New(config Config) (http.Handler, error) {
	if config.Database == nil || strings.TrimSpace(config.ClientID) == "" || len(config.Keys) == 0 || len(config.Keys) > 2 {
		return nil, errors.New("account portfolio database and service credentials are required")
	}
	for keyID, secret := range config.Keys {
		if strings.TrimSpace(keyID) == "" || len(secret) < 32 {
			return nil, errors.New("account portfolio service key ring is invalid")
		}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	h := &service{database: config.Database, clientID: config.ClientID, keys: config.Keys, now: now}
	router := chi.NewRouter()
	router.Use(h.requestContext)
	router.Get(contract.HealthRoute, h.health)
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticate)
		protected.Get(contract.SummaryRoute, h.summary)
		protected.Get(contract.PointsRoute, h.points)
		protected.Get(contract.MembershipRoute, h.membership)
		protected.Get(contract.NotificationsRoute, h.notifications)
		protected.Get(contract.TicketsRoute, h.tickets)
		protected.Get(contract.MembershipOrdersRoute, h.membershipOrders)
	})
	return router, nil
}

func (h *service) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.database.Ping(ctx); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio database is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]bool{"ok": true})
}

func (h *service) summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	var data struct {
		PointsBalance           int64  `json:"points_balance"`
		Plan                    string `json:"plan"`
		Lifetime                bool   `json:"lifetime"`
		UnreadNotificationCount int    `json:"unread_notification_count"`
		OpenTicketCount         int    `json:"open_ticket_count"`
	}
	err := h.database.QueryRow(r.Context(), `
		SELECT p.balance, m.plan,
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1 AND read_at IS NULL),
			(SELECT count(*) FROM account_portfolio_tickets WHERE user_id=$1 AND status <> 'resolved')
		FROM account_portfolio_points p
		JOIN account_portfolio_memberships m ON m.user_id = p.user_id
		WHERE p.user_id=$1
	`, userID).Scan(&data.PointsBalance, &data.Plan, &data.UnreadNotificationCount, &data.OpenTicketCount)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio summary is unavailable")
		return
	}
	data.Lifetime = data.Plan == "lifetime"
	writeData(w, r, http.StatusOK, data)
}

func (h *service) points(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	type entry struct {
		ID        string    `json:"id"`
		Amount    int64     `json:"amount"`
		Reason    string    `json:"reason"`
		CreatedAt time.Time `json:"created_at"`
	}
	data := struct {
		Balance int64   `json:"balance"`
		Entries []entry `json:"entries"`
	}{Entries: make([]entry, 0)}
	if err := h.database.QueryRow(r.Context(), `SELECT balance FROM account_portfolio_points WHERE user_id=$1`, userID).Scan(&data.Balance); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
		return
	}
	rows, err := h.database.Query(r.Context(), `SELECT id, amount, reason, created_at FROM account_portfolio_point_ledger WHERE user_id=$1 ORDER BY created_at DESC, id DESC LIMIT 100`, userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value entry
		if err := rows.Scan(&value.ID, &value.Amount, &value.Reason, &value.CreatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
			return
		}
		data.Entries = append(data.Entries, value)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) membership(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	data := struct {
		Plan     string `json:"plan"`
		Lifetime bool   `json:"lifetime"`
	}{}
	if err := h.database.QueryRow(r.Context(), `SELECT plan FROM account_portfolio_memberships WHERE user_id=$1`, userID).Scan(&data.Plan); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio membership is unavailable")
		return
	}
	data.Lifetime = data.Plan == "lifetime"
	writeData(w, r, http.StatusOK, data)
}

func (h *service) notifications(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	type notification struct {
		ID        string     `json:"id"`
		Title     string     `json:"title"`
		Body      string     `json:"body"`
		Kind      string     `json:"kind"`
		ReadAt    *time.Time `json:"read_at,omitempty"`
		CreatedAt time.Time  `json:"created_at"`
	}
	data := struct {
		Notifications []notification `json:"notifications"`
	}{Notifications: make([]notification, 0)}
	rows, err := h.database.Query(r.Context(), `SELECT id, title, body, kind, read_at, created_at FROM account_portfolio_notifications WHERE user_id=$1 ORDER BY created_at DESC, id DESC LIMIT 100`, userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio notifications are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value notification
		if err := rows.Scan(&value.ID, &value.Title, &value.Body, &value.Kind, &value.ReadAt, &value.CreatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio notifications are unavailable")
			return
		}
		data.Notifications = append(data.Notifications, value)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio notifications are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) tickets(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	type ticket struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Category  string    `json:"category"`
		Status    string    `json:"status"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	data := struct {
		Tickets []ticket `json:"tickets"`
	}{Tickets: make([]ticket, 0)}
	rows, err := h.database.Query(r.Context(), `SELECT id, title, category, status, updated_at FROM account_portfolio_tickets WHERE user_id=$1 ORDER BY updated_at DESC, id DESC LIMIT 100`, userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio tickets are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value ticket
		if err := rows.Scan(&value.ID, &value.Title, &value.Category, &value.Status, &value.UpdatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio tickets are unavailable")
			return
		}
		data.Tickets = append(data.Tickets, value)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio tickets are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) membershipOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	type order struct {
		ID          string    `json:"id"`
		Plan        string    `json:"plan"`
		AmountCents int       `json:"amount_cents"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"created_at"`
	}
	data := struct {
		Orders []order `json:"orders"`
	}{Orders: make([]order, 0)}
	rows, err := h.database.Query(r.Context(), `SELECT id, plan, amount_cents, status, created_at FROM account_portfolio_membership_orders WHERE user_id=$1 ORDER BY created_at DESC, id DESC LIMIT 100`, userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio orders are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value order
		if err := rows.Scan(&value.ID, &value.Plan, &value.AmountCents, &value.Status, &value.CreatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio orders are unavailable")
			return
		}
		data.Orders = append(data.Orders, value)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio orders are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) prepareAccount(w http.ResponseWriter, r *http.Request) (string, bool) {
	value, _ := r.Context().Value(actorKey).(actor)
	if value.userID == "" {
		writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "Account Portfolio actor is invalid")
		return "", false
	}
	if err := h.ensureAccount(r.Context(), value.userID); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio is unavailable")
		return "", false
	}
	return value.userID, true
}

func (h *service) ensureAccount(ctx context.Context, userID string) error {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, statement := range []string{
		`INSERT INTO account_portfolio_accounts(user_id) VALUES($1) ON CONFLICT (user_id) DO NOTHING`,
		`INSERT INTO account_portfolio_points(user_id) VALUES($1) ON CONFLICT (user_id) DO NOTHING`,
		`INSERT INTO account_portfolio_memberships(user_id) VALUES($1) ON CONFLICT (user_id) DO NOTHING`,
	} {
		if _, err := tx.Exec(ctx, statement, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (h *service) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if len(requestID) > 120 || (requestID != "" && !requestIDPattern.MatchString(requestID) && uuid.Validate(requestID) != nil) {
			requestID = ""
		}
		if requestID == "" {
			requestID = "req_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		}
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
	})
}

func requestID(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey).(string)
	return value
}

func writeData(w http.ResponseWriter, r *http.Request, status int, data any) {
	writeJSON(w, status, map[string]any{"data": data, "request_id": requestID(r)})
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}, "request_id": requestID(r)})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
