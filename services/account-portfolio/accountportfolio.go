// Package accountportfolio owns persistent user points, membership,
// membership orders, notifications, and support tickets. It deliberately does
// not read or translate legacy Study account state.
package accountportfolio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"henukit.dev/account-portfolio/internal/contract"
)

var (
	requestIDPattern      = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)
	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	ticketCategoryPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// maxPublicPointValue keeps every numeric point fact exactly representable by
// Portal and Console JavaScript clients. Point JSON values are never rounded
// across the owner boundary.
const maxPublicPointValue int64 = 9_007_199_254_740_991

// Config contains only private service-to-service configuration. Browser
// clients always go through Portal Gateway and never receive these values.
type Config struct {
	Database        *pgxpool.Pool
	ClientID        string
	Keys            map[string]string
	ConsoleClientID string
	ConsoleKeys     map[string]string
	// PaymentProvider is nil in every production configuration until the
	// separately authorized provider Spike supplies a real implementation.
	PaymentProvider PaymentProvider
	// PointCursorKey is the independently configured AES-256-GCM key that
	// encrypts private Point Ledger continuation boundaries. It never crosses
	// the Owner HTTP boundary.
	PointCursorKey []byte
	Now            func() time.Time
}

type service struct {
	database        *pgxpool.Pool
	clientID        string
	consoleClientID string
	clientKeys      map[string]map[string]string
	paymentProvider PaymentProvider
	pointCursors    *pointCursorCodec
	now             func() time.Time
}

type actor struct {
	userID   string
	clientID string
}
type contextKey string

const (
	requestIDKey contextKey = "request-id"
	actorKey     contextKey = "actor"
)

type ticketView struct {
	ID        string    `json:"id"`
	Reference string    `json:"reference"`
	Title     string    `json:"title"`
	Category  string    `json:"category"`
	Status    string    `json:"status"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ticketMessageView struct {
	ID         string    `json:"id"`
	AuthorKind string    `json:"author_kind"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type ticketEventView struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	CreatedAt  time.Time `json:"created_at"`
}

type notificationView struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	Kind            string     `json:"kind"`
	TicketID        *string    `json:"ticket_id,omitempty"`
	TicketReference *string    `json:"ticket_reference,omitempty"`
	ReadAt          *time.Time `json:"read_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type consoleMembershipView struct {
	Plan     string `json:"plan"`
	Lifetime bool   `json:"lifetime"`
	Version  int    `json:"version"`
}

type ticketRecord struct {
	Ticket ticketView
	UserID string
}

type pointLedgerEntryView struct {
	ID        string    `json:"id"`
	Amount    int64     `json:"amount"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type pointsView struct {
	Balance    int64                  `json:"balance"`
	Entries    []pointLedgerEntryView `json:"entries"`
	NextCursor *string                `json:"next_cursor"`
}

type pointCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type commandFailure struct {
	status  int
	code    string
	message string
}

func (e *commandFailure) Error() string { return e.message }

// New creates an independently deployable Account Portfolio HTTP handler.
func New(config Config) (http.Handler, error) {
	if config.Database == nil || strings.TrimSpace(config.ClientID) == "" || !validClientKeys(config.Keys) {
		return nil, errors.New("account portfolio database and service credentials are required")
	}
	pointCursors, err := newPointCursorCodec(config.PointCursorKey)
	if err != nil {
		return nil, errors.New("account portfolio point cursor encryption key is invalid")
	}
	if pointCursorKeyReusesServiceSecret(config.PointCursorKey, config.Keys, config.ConsoleKeys) {
		return nil, errors.New("account portfolio point cursor encryption key must be independent from service credentials")
	}
	consoleConfigured := strings.TrimSpace(config.ConsoleClientID) != "" || len(config.ConsoleKeys) != 0
	if consoleConfigured && (strings.TrimSpace(config.ConsoleClientID) == "" || config.ConsoleClientID == config.ClientID || !validClientKeys(config.ConsoleKeys) || sharedServiceSecret(config.Keys, config.ConsoleKeys)) {
		return nil, errors.New("account portfolio Console service credentials are invalid")
	}
	if config.PaymentProvider != nil && !validPaymentProviderName(config.PaymentProvider.Name()) {
		return nil, errors.New("account portfolio payment provider is invalid")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	clientKeys := map[string]map[string]string{config.ClientID: config.Keys}
	if consoleConfigured {
		clientKeys[config.ConsoleClientID] = config.ConsoleKeys
	}
	h := &service{database: config.Database, clientID: config.ClientID, consoleClientID: config.ConsoleClientID, clientKeys: clientKeys, paymentProvider: config.PaymentProvider, pointCursors: pointCursors, now: now}
	router := chi.NewRouter()
	router.Use(h.requestContext)
	router.Get(contract.HealthRoute, h.health)
	router.Post(contract.PaymentProviderNotificationRoute, h.paymentProviderNotification)
	router.Group(func(protected chi.Router) {
		protected.Use(h.authenticate)
		protected.Get(contract.SummaryRoute, h.summary)
		protected.Get(contract.PointsRoute, h.points)
		protected.Get(contract.MembershipRoute, h.membership)
		protected.Get(contract.NotificationsRoute, h.notifications)
		protected.Post(contract.NotificationReadRoute, h.markNotificationRead)
		protected.Get(contract.TicketsRoute, h.tickets)
		protected.Post(contract.TicketsRoute, h.createTicket)
		protected.Get(contract.TicketRoute, h.ticket)
		protected.Post(contract.TicketFollowUpsRoute, h.createTicketFollowUp)
		protected.Get(contract.MembershipOrdersRoute, h.membershipOrders)
		protected.Post(contract.MembershipOrderCreateRoute, h.createMembershipOrder)
		protected.Get(contract.ConsoleMembershipRoute, h.consoleMembership)
		protected.Post(contract.ConsoleMembershipGrantsRoute, h.grantConsoleMembership)
		protected.Post(contract.ConsoleMembershipRevocationsRoute, h.revokeConsoleMembership)
		protected.Post(contract.ConsolePointAdjustmentsRoute, h.adjustConsolePoints)
		protected.Get(contract.ConsoleTicketsRoute, h.consoleTickets)
		protected.Get(contract.ConsoleTicketRoute, h.consoleTicket)
		protected.Post(contract.ConsoleTicketRepliesRoute, h.replyConsoleTicket)
		protected.Post(contract.ConsoleTicketTransitionsRoute, h.transitionConsoleTicket)
	})
	return router, nil
}

func validClientKeys(keys map[string]string) bool {
	if len(keys) == 0 || len(keys) > 2 {
		return false
	}
	for keyID, secret := range keys {
		if strings.TrimSpace(keyID) == "" || len(secret) < 32 {
			return false
		}
	}
	return true
}

func sharedServiceSecret(first, second map[string]string) bool {
	for _, firstSecret := range first {
		for _, secondSecret := range second {
			if subtle.ConstantTimeCompare([]byte(firstSecret), []byte(secondSecret)) == 1 {
				return true
			}
		}
	}
	return false
}

func pointCursorKeyReusesServiceSecret(pointCursorKey []byte, keySets ...map[string]string) bool {
	for _, keys := range keySets {
		for _, secret := range keys {
			if subtle.ConstantTimeCompare(pointCursorKey, []byte(secret)) == 1 {
				return true
			}
			for _, encoding := range []*base64.Encoding{
				base64.StdEncoding,
				base64.RawStdEncoding,
				base64.URLEncoding,
				base64.RawURLEncoding,
			} {
				decoded, err := encoding.DecodeString(secret)
				if err == nil && subtle.ConstantTimeCompare(pointCursorKey, decoded) == 1 {
					return true
				}
			}
		}
	}
	return false
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
		SELECT COALESCE((SELECT SUM(l.amount) FROM account_portfolio_point_ledger l WHERE l.user_id=$1), 0), m.plan,
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1 AND read_at IS NULL),
			(SELECT count(*) FROM account_portfolio_tickets WHERE user_id=$1 AND status <> 'resolved')
		FROM account_portfolio_memberships m
		WHERE m.user_id=$1
	`, userID).Scan(&data.PointsBalance, &data.Plan, &data.UnreadNotificationCount, &data.OpenTicketCount)
	if err != nil || data.PointsBalance < 0 || data.PointsBalance > maxPublicPointValue {
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
	limit, cursor, failure := h.pointPage(r, userID)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	data := pointsView{Entries: make([]pointLedgerEntryView, 0)}
	if err := h.database.QueryRow(r.Context(), `SELECT COALESCE(SUM(amount), 0) FROM account_portfolio_point_ledger WHERE user_id=$1`, userID).Scan(&data.Balance); err != nil || data.Balance < 0 || data.Balance > maxPublicPointValue {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
		return
	}
	query := `SELECT id, amount, reason, created_at FROM account_portfolio_point_ledger WHERE user_id=$1`
	args := []any{userID}
	if cursor != nil {
		query += ` AND (created_at, id) < ($2, $3)`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)
	rows, err := h.database.Query(r.Context(), query, args...)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value pointLedgerEntryView
		if err := rows.Scan(&value.ID, &value.Amount, &value.Reason, &value.CreatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
			return
		}
		if value.Amount < -maxPublicPointValue || value.Amount > maxPublicPointValue || value.Amount == 0 {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
			return
		}
		if len(data.Entries) == limit {
			encoded, err := h.pointCursors.encode(userID, data.Entries[len(data.Entries)-1], h.now().UTC())
			if err != nil {
				writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio points are unavailable")
				return
			}
			data.NextCursor = &encoded
			break
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
	data := struct {
		Notifications []notificationView `json:"notifications"`
	}{Notifications: make([]notificationView, 0)}
	rows, err := h.database.Query(r.Context(), `
		SELECT id, title, body, kind, ticket_id::text, read_at, created_at
		FROM account_portfolio_notifications
		WHERE user_id=$1
		ORDER BY created_at DESC, id DESC
		LIMIT 100
	`, userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio notifications are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value notificationView
		if err := rows.Scan(&value.ID, &value.Title, &value.Body, &value.Kind, &value.TicketID, &value.ReadAt, &value.CreatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio notifications are unavailable")
			return
		}
		setTicketReference(&value)
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
	data := struct {
		Tickets []ticketView `json:"tickets"`
	}{Tickets: make([]ticketView, 0)}
	rows, err := h.database.Query(r.Context(), `
		SELECT id, title, category, status, version, created_at, updated_at
		FROM account_portfolio_tickets
		WHERE user_id=$1
		ORDER BY updated_at DESC, id DESC
		LIMIT 100
	`, userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio tickets are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value ticketView
		if err := rows.Scan(&value.ID, &value.Title, &value.Category, &value.Status, &value.Version, &value.CreatedAt, &value.UpdatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio tickets are unavailable")
			return
		}
		value.Reference = ticketReference(value.ID)
		data.Tickets = append(data.Tickets, value)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio tickets are unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) consoleMembership(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	userID, failure := membershipTargetID(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	membership, err := h.consoleMembershipByUserID(r.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Account Portfolio membership is not initialized for this user")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio membership is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"membership": membership})
}

func (h *service) grantConsoleMembership(w http.ResponseWriter, r *http.Request) {
	h.mutateConsoleMembership(w, r, "membership_grant", "grant", "free", "lifetime", "operator_grant", "membership_lifetime_granted", "终身会员权益已发放", "你的终身会员权益已由运营人员发放。")
}

func (h *service) revokeConsoleMembership(w http.ResponseWriter, r *http.Request) {
	h.mutateConsoleMembership(w, r, "membership_revoke", "revoke", "lifetime", "free", "operator_revocation", "membership_lifetime_revoked", "终身会员权益已撤销", "你的终身会员权益已由运营人员撤销。")
}

func (h *service) mutateConsoleMembership(w http.ResponseWriter, r *http.Request, operation, eventKind, fromPlan, toPlan, source, notificationKind, notificationTitle, notificationBody string) {
	operator, ok := h.prepareConsole(w, r)
	if !ok {
		return
	}
	userID, failure := membershipTargetID(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	var input struct {
		Reason          string `json:"reason"`
		ExpectedVersion int    `json:"expected_version"`
	}
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Reason)) == 0 || len([]rune(input.Reason)) > 1000 || input.ExpectedVersion < 1 {
		writeCommandFailure(w, r, invalidCommand("membership mutation is invalid"))
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload, status, failure := h.executeCommand(r.Context(), operator.clientID, operator.userID, operation, r.URL.Path, key, raw, http.StatusOK, func(tx pgx.Tx) (any, *commandFailure) {
		var current consoleMembershipView
		err := tx.QueryRow(r.Context(), `
			SELECT plan, version
			FROM account_portfolio_memberships
			WHERE user_id=$1
			FOR UPDATE
		`, userID).Scan(&current.Plan, &current.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFoundFailure("Account Portfolio membership is not initialized for this user")
		}
		if err != nil {
			return nil, dependencyFailure("Account Portfolio membership is unavailable")
		}
		current.Lifetime = current.Plan == "lifetime"
		if current.Version != input.ExpectedVersion {
			return nil, membershipVersionConflictFailure()
		}
		if current.Plan != fromPlan {
			return nil, invalidStateFailure("membership transition is not allowed")
		}
		now := h.now().UTC()
		if eventKind == "grant" {
			err = tx.QueryRow(r.Context(), `
				UPDATE account_portfolio_memberships
				SET plan=$2, source=$3, payment_fact_id=NULL, granted_at=$4, version=version+1, updated_at=$4
				WHERE user_id=$1 AND version=$5
				RETURNING plan, version
			`, userID, toPlan, source, now, input.ExpectedVersion).Scan(&current.Plan, &current.Version)
		} else {
			err = tx.QueryRow(r.Context(), `
				UPDATE account_portfolio_memberships
				SET plan=$2, source=$3, payment_fact_id=NULL, version=version+1, updated_at=$4
				WHERE user_id=$1 AND version=$5
				RETURNING plan, version
			`, userID, toPlan, source, now, input.ExpectedVersion).Scan(&current.Plan, &current.Version)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, membershipVersionConflictFailure()
		}
		if err != nil {
			return nil, dependencyFailure("Account Portfolio membership is unavailable")
		}
		current.Lifetime = current.Plan == "lifetime"
		eventID := uuid.NewString()
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_membership_events(id, user_id, kind, from_plan, to_plan, source, actor_user_id, reason, idempotency_key, created_at)
			VALUES($1, $2, $3, $4, $5, 'operator', $6, $7, $8, $9)
		`, eventID, userID, eventKind, fromPlan, toPlan, operator.userID, input.Reason, key, now); err != nil {
			return nil, dependencyFailure("Account Portfolio membership audit is unavailable")
		}
		if failure := h.createMembershipNotification(r.Context(), tx, eventID, userID, notificationKind, notificationTitle, notificationBody, now); failure != nil {
			return nil, failure
		}
		return map[string]any{"membership": current}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
}

// adjustConsolePoints is deliberately a Console-only command. The target user
// comes from the signed Console body, while the operator comes only from the
// separately authenticated Console Gateway actor header.
func (h *service) adjustConsolePoints(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.prepareConsole(w, r)
	if !ok {
		return
	}
	var input struct {
		UserID string `json:"user_id"`
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.Reason = strings.TrimSpace(input.Reason)
	if uuid.Validate(input.UserID) != nil || input.Amount < -maxPublicPointValue || input.Amount > maxPublicPointValue || input.Amount == 0 || len([]rune(input.Reason)) == 0 || len([]rune(input.Reason)) > 1000 {
		writeCommandFailure(w, r, invalidCommand("point adjustment is invalid"))
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload, status, failure := h.executeCommand(r.Context(), operator.clientID, operator.userID, "point_adjustment", r.URL.Path, key, raw, http.StatusOK, func(tx pgx.Tx) (any, *commandFailure) {
		if err := ensureAccountTx(r.Context(), tx, input.UserID); err != nil {
			return nil, dependencyFailure("Account Portfolio points are unavailable")
		}
		var projection int64
		if err := tx.QueryRow(r.Context(), `
			SELECT balance
			FROM account_portfolio_points
			WHERE user_id=$1
			FOR UPDATE
		`, input.UserID).Scan(&projection); err != nil {
			return nil, dependencyFailure("Account Portfolio points are unavailable")
		}
		var balance int64
		if err := tx.QueryRow(r.Context(), `
			SELECT COALESCE(SUM(amount), 0)
			FROM account_portfolio_point_ledger
			WHERE user_id=$1
		`, input.UserID).Scan(&balance); err != nil || balance < 0 || balance > maxPublicPointValue {
			return nil, dependencyFailure("Account Portfolio points are unavailable")
		}
		if projection != balance {
			if _, err := tx.Exec(r.Context(), `UPDATE account_portfolio_points SET balance=$2, updated_at=$3 WHERE user_id=$1`, input.UserID, balance, h.now().UTC()); err != nil {
				return nil, dependencyFailure("Account Portfolio points are unavailable")
			}
		}
		newBalance, failure := adjustedPointBalance(balance, input.Amount)
		if failure != nil {
			return nil, failure
		}
		now := h.now().UTC()
		auditID := uuid.NewString()
		entry := pointLedgerEntryView{ID: uuid.NewString(), Amount: input.Amount, Reason: input.Reason, CreatedAt: now}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_point_adjustment_audits(id, operator_user_id, target_user_id, amount, reason, idempotency_key, created_at)
			VALUES($1, $2, $3, $4, $5, $6, $7)
		`, auditID, operator.userID, input.UserID, input.Amount, input.Reason, key, now); err != nil {
			return nil, dependencyFailure("Account Portfolio point adjustment audit is unavailable")
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_point_ledger(id, user_id, amount, reason, actor_user_id, audit_id, created_at)
			VALUES($1, $2, $3, $4, $5, $6, $7)
		`, entry.ID, input.UserID, input.Amount, input.Reason, operator.userID, auditID, now); err != nil {
			return nil, dependencyFailure("Account Portfolio point ledger is unavailable")
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE account_portfolio_points
			SET balance=$2, updated_at=$3
			WHERE user_id=$1
		`, input.UserID, newBalance, now); err != nil {
			return nil, dependencyFailure("Account Portfolio points are unavailable")
		}
		if failure := h.createPointNotification(r.Context(), tx, entry.ID, input.UserID, input.Amount, newBalance, now); failure != nil {
			return nil, failure
		}
		return map[string]any{"balance": newBalance, "entry": entry}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
}

func adjustedPointBalance(balance, amount int64) (int64, *commandFailure) {
	if balance < 0 || balance > maxPublicPointValue || amount < -maxPublicPointValue || amount > maxPublicPointValue || amount == 0 {
		return 0, invalidCommand("point adjustment is outside its supported range")
	}
	if amount > 0 {
		if balance > maxPublicPointValue-amount {
			return 0, invalidCommand("point balance would exceed its supported range")
		}
		return balance + amount, nil
	}
	debit := -amount
	if balance < debit {
		return 0, &commandFailure{status: http.StatusConflict, code: "INSUFFICIENT_POINTS", message: "point debit exceeds the available balance"}
	}
	return balance - debit, nil
}

func (h *service) consoleTickets(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	data := struct {
		Tickets []ticketView `json:"tickets"`
	}{Tickets: make([]ticketView, 0)}
	rows, err := h.database.Query(r.Context(), `
		SELECT id, title, category, status, version, created_at, updated_at
		FROM account_portfolio_tickets
		ORDER BY updated_at DESC, id DESC
		LIMIT 100
	`)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio Console queue is unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value ticketView
		if err := rows.Scan(&value.ID, &value.Title, &value.Category, &value.Status, &value.Version, &value.CreatedAt, &value.UpdatedAt); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio Console queue is unavailable")
			return
		}
		value.Reference = ticketReference(value.ID)
		data.Tickets = append(data.Tickets, value)
	}
	if err := rows.Err(); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio Console queue is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, data)
}

func (h *service) consoleTicket(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	ticketID := chi.URLParam(r, "ticket_id")
	if uuid.Validate(ticketID) != nil {
		writeCommandFailure(w, r, invalidCommand("ticket id is invalid"))
		return
	}
	value, err := h.ticketByID(r.Context(), ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "support ticket was not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio Console ticket is unavailable")
		return
	}
	messages, err := h.ticketMessages(r.Context(), ticketID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio Console ticket is unavailable")
		return
	}
	events, err := h.ticketEvents(r.Context(), ticketID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio Console ticket is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, struct {
		Ticket   ticketView          `json:"ticket"`
		Messages []ticketMessageView `json:"messages"`
		Events   []ticketEventView   `json:"events"`
	}{Ticket: value.Ticket, Messages: messages, Events: events})
}

func (h *service) replyConsoleTicket(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.prepareConsole(w, r)
	if !ok {
		return
	}
	ticketID := chi.URLParam(r, "ticket_id")
	if uuid.Validate(ticketID) != nil {
		writeCommandFailure(w, r, invalidCommand("ticket id is invalid"))
		return
	}
	var input struct {
		Body            string `json:"body"`
		ExpectedVersion int    `json:"expected_version"`
	}
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if len(input.Body) == 0 || len(input.Body) > 5000 || input.ExpectedVersion < 1 {
		writeCommandFailure(w, r, invalidCommand("operator reply is invalid"))
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload, status, failure := h.executeCommand(r.Context(), operator.clientID, operator.userID, "ticket_reply", r.URL.Path, key, raw, http.StatusOK, func(tx pgx.Tx) (any, *commandFailure) {
		var current ticketRecord
		err := tx.QueryRow(r.Context(), `
			SELECT id, user_id, title, category, status, version, created_at, updated_at
			FROM account_portfolio_tickets
			WHERE id=$1
			FOR UPDATE
		`, ticketID).Scan(&current.Ticket.ID, &current.UserID, &current.Ticket.Title, &current.Ticket.Category, &current.Ticket.Status, &current.Ticket.Version, &current.Ticket.CreatedAt, &current.Ticket.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFoundFailure("support ticket was not found")
		}
		if err != nil {
			return nil, dependencyFailure("Account Portfolio ticket reply is unavailable")
		}
		if current.Ticket.Version != input.ExpectedVersion {
			return nil, versionConflictFailure()
		}
		fromStatus := current.Ticket.Status
		if fromStatus == "resolved" {
			return nil, invalidStateFailure("resolved support tickets must be reopened by their owner")
		}
		toStatus := fromStatus
		if fromStatus == "open" {
			toStatus = "in_progress"
		}
		if toStatus != "in_progress" {
			return nil, invalidStateFailure("support ticket state cannot receive an operator reply")
		}
		now := h.now().UTC()
		messageID := uuid.NewString()
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_ticket_messages(id, ticket_id, author_kind, operator_user_id, body, created_at)
			VALUES($1, $2, 'operator', $3, $4, $5)
		`, messageID, ticketID, operator.userID, input.Body, now); err != nil {
			return nil, dependencyFailure("Account Portfolio ticket reply is unavailable")
		}
		if err := tx.QueryRow(r.Context(), `
			UPDATE account_portfolio_tickets
			SET status=$2, version=version+1, updated_at=$3
			WHERE id=$1 AND version=$4
			RETURNING id, title, category, status, version, created_at, updated_at
		`, ticketID, toStatus, now, input.ExpectedVersion).Scan(&current.Ticket.ID, &current.Ticket.Title, &current.Ticket.Category, &current.Ticket.Status, &current.Ticket.Version, &current.Ticket.CreatedAt, &current.Ticket.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, versionConflictFailure()
			}
			return nil, dependencyFailure("Account Portfolio ticket reply is unavailable")
		}
		eventID := uuid.NewString()
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_ticket_events(id, ticket_id, kind, actor_kind, actor_user_id, message_id, from_status, to_status, created_at)
			VALUES($1, $2, 'operator_reply', 'operator', $3, $4, $5, $6, $7)
		`, eventID, ticketID, operator.userID, messageID, fromStatus, toStatus, now); err != nil {
			return nil, dependencyFailure("Account Portfolio ticket reply is unavailable")
		}
		if failure := h.createTicketNotification(r.Context(), tx, ticketID, eventID, current.UserID, "ticket_operator_reply", "客服工单有新回复", "工单 "+ticketReference(ticketID)+" 已收到运营回复。", now); failure != nil {
			return nil, failure
		}
		current.Ticket.Reference = ticketReference(current.Ticket.ID)
		return map[string]any{"ticket": current.Ticket}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
}

func (h *service) transitionConsoleTicket(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.prepareConsole(w, r)
	if !ok {
		return
	}
	ticketID := chi.URLParam(r, "ticket_id")
	if uuid.Validate(ticketID) != nil {
		writeCommandFailure(w, r, invalidCommand("ticket id is invalid"))
		return
	}
	var input struct {
		Status          string `json:"status"`
		ExpectedVersion int    `json:"expected_version"`
	}
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	input.Status = strings.TrimSpace(input.Status)
	if input.ExpectedVersion < 1 || (input.Status != "in_progress" && input.Status != "resolved") {
		writeCommandFailure(w, r, invalidCommand("ticket transition is invalid"))
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload, status, failure := h.executeCommand(r.Context(), operator.clientID, operator.userID, "ticket_transition", r.URL.Path, key, raw, http.StatusOK, func(tx pgx.Tx) (any, *commandFailure) {
		var current ticketRecord
		err := tx.QueryRow(r.Context(), `
			SELECT id, user_id, title, category, status, version, created_at, updated_at
			FROM account_portfolio_tickets
			WHERE id=$1
			FOR UPDATE
		`, ticketID).Scan(&current.Ticket.ID, &current.UserID, &current.Ticket.Title, &current.Ticket.Category, &current.Ticket.Status, &current.Ticket.Version, &current.Ticket.CreatedAt, &current.Ticket.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFoundFailure("support ticket was not found")
		}
		if err != nil {
			return nil, dependencyFailure("Account Portfolio ticket transition is unavailable")
		}
		if current.Ticket.Version != input.ExpectedVersion {
			return nil, versionConflictFailure()
		}
		fromStatus := current.Ticket.Status
		if !allowedTicketTransition(fromStatus, input.Status) {
			return nil, invalidStateFailure("support ticket transition is not allowed")
		}
		now := h.now().UTC()
		if err := tx.QueryRow(r.Context(), `
			UPDATE account_portfolio_tickets
			SET status=$2, version=version+1, updated_at=$3
			WHERE id=$1 AND version=$4
			RETURNING id, title, category, status, version, created_at, updated_at
		`, ticketID, input.Status, now, input.ExpectedVersion).Scan(&current.Ticket.ID, &current.Ticket.Title, &current.Ticket.Category, &current.Ticket.Status, &current.Ticket.Version, &current.Ticket.CreatedAt, &current.Ticket.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, versionConflictFailure()
			}
			return nil, dependencyFailure("Account Portfolio ticket transition is unavailable")
		}
		eventID := uuid.NewString()
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_ticket_events(id, ticket_id, kind, actor_kind, actor_user_id, from_status, to_status, created_at)
			VALUES($1, $2, 'status_transition', 'operator', $3, $4, $5, $6)
		`, eventID, ticketID, operator.userID, fromStatus, input.Status, now); err != nil {
			return nil, dependencyFailure("Account Portfolio ticket transition is unavailable")
		}
		if failure := h.createTicketNotification(r.Context(), tx, ticketID, eventID, current.UserID, "ticket_status", "客服工单状态已更新", "工单 "+ticketReference(ticketID)+" 状态已更新为“"+ticketStatusLabel(input.Status)+"”。", now); failure != nil {
			return nil, failure
		}
		current.Ticket.Reference = ticketReference(current.Ticket.ID)
		return map[string]any{"ticket": current.Ticket}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
}

func (h *service) createTicket(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	var input struct {
		Title    string `json:"title"`
		Category string `json:"category"`
		Body     string `json:"body"`
	}
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Category = strings.TrimSpace(input.Category)
	input.Body = strings.TrimSpace(input.Body)
	if len(input.Title) == 0 || len(input.Title) > 160 || len(input.Category) == 0 || len(input.Category) > 80 || !ticketCategoryPattern.MatchString(input.Category) || len(input.Body) == 0 || len(input.Body) > 5000 {
		writeCommandFailure(w, r, invalidCommand("ticket input is invalid"))
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload, status, failure := h.executeCommand(r.Context(), authenticatedActor(r).clientID, userID, "ticket_create", r.URL.Path, key, raw, http.StatusCreated, func(tx pgx.Tx) (any, *commandFailure) {
		id := uuid.NewString()
		now := h.now().UTC()
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_tickets(id, user_id, title, category, status, version, created_at, updated_at)
			VALUES($1, $2, $3, $4, 'open', 1, $5, $5)
		`, id, userID, input.Title, input.Category, now); err != nil {
			return nil, dependencyFailure("Account Portfolio ticket creation is unavailable")
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_ticket_messages(id, ticket_id, author_kind, body, created_at)
			VALUES($1, $2, 'user', $3, $4)
		`, uuid.NewString(), id, input.Body, now); err != nil {
			return nil, dependencyFailure("Account Portfolio ticket creation is unavailable")
		}
		return map[string]any{"ticket": ticketView{
			ID: id, Reference: ticketReference(id), Title: input.Title, Category: input.Category,
			Status: "open", Version: 1, CreatedAt: now, UpdatedAt: now,
		}}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
}

func (h *service) ticket(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	ticketID := chi.URLParam(r, "ticket_id")
	if uuid.Validate(ticketID) != nil {
		writeCommandFailure(w, r, invalidCommand("ticket id is invalid"))
		return
	}
	value, err := h.ownerTicket(r.Context(), userID, ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "support ticket was not found")
		return
	}
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio ticket is unavailable")
		return
	}
	messages, err := h.ownerTicketMessages(r.Context(), userID, ticketID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio ticket is unavailable")
		return
	}
	events, err := h.ownerTicketEvents(r.Context(), userID, ticketID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio ticket is unavailable")
		return
	}
	writeData(w, r, http.StatusOK, struct {
		Ticket   ticketView          `json:"ticket"`
		Messages []ticketMessageView `json:"messages"`
		Events   []ticketEventView   `json:"events"`
	}{Ticket: value, Messages: messages, Events: events})
}

func (h *service) createTicketFollowUp(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	ticketID := chi.URLParam(r, "ticket_id")
	if uuid.Validate(ticketID) != nil {
		writeCommandFailure(w, r, invalidCommand("ticket id is invalid"))
		return
	}
	var input struct {
		Body            string `json:"body"`
		ExpectedVersion int    `json:"expected_version"`
	}
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if len(input.Body) == 0 || len(input.Body) > 5000 || input.ExpectedVersion < 1 {
		writeCommandFailure(w, r, invalidCommand("ticket follow-up is invalid"))
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload, status, failure := h.executeCommand(r.Context(), authenticatedActor(r).clientID, userID, "ticket_follow_up", r.URL.Path, key, raw, http.StatusOK, func(tx pgx.Tx) (any, *commandFailure) {
		var current ticketView
		err := tx.QueryRow(r.Context(), `
			SELECT id, title, category, status, version, created_at, updated_at
			FROM account_portfolio_tickets
			WHERE id=$1 AND user_id=$2
			FOR UPDATE
		`, ticketID, userID).Scan(&current.ID, &current.Title, &current.Category, &current.Status, &current.Version, &current.CreatedAt, &current.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFoundFailure("support ticket was not found")
		}
		if err != nil {
			return nil, dependencyFailure("Account Portfolio ticket is unavailable")
		}
		if current.Version != input.ExpectedVersion {
			return nil, versionConflictFailure()
		}
		now := h.now().UTC()
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_ticket_messages(id, ticket_id, author_kind, body, created_at)
			VALUES($1, $2, 'user', $3, $4)
		`, uuid.NewString(), ticketID, input.Body, now); err != nil {
			return nil, dependencyFailure("Account Portfolio ticket follow-up is unavailable")
		}
		newStatus := current.Status
		if current.Status == "resolved" {
			newStatus = "open"
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO account_portfolio_ticket_events(id, ticket_id, kind, actor_kind, actor_user_id, from_status, to_status, created_at)
				VALUES($1, $2, 'reopened', 'user', $3, 'resolved', 'open', $4)
			`, uuid.NewString(), ticketID, userID, now); err != nil {
				return nil, dependencyFailure("Account Portfolio ticket follow-up is unavailable")
			}
		}
		if err := tx.QueryRow(r.Context(), `
			UPDATE account_portfolio_tickets
			SET status=$3, version=version+1, updated_at=$4
			WHERE id=$1 AND user_id=$2 AND version=$5
			RETURNING id, title, category, status, version, created_at, updated_at
		`, ticketID, userID, newStatus, now, input.ExpectedVersion).Scan(&current.ID, &current.Title, &current.Category, &current.Status, &current.Version, &current.CreatedAt, &current.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, versionConflictFailure()
			}
			return nil, dependencyFailure("Account Portfolio ticket follow-up is unavailable")
		}
		current.Reference = ticketReference(current.ID)
		return map[string]any{"ticket": current}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
}

func (h *service) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	notificationID := chi.URLParam(r, "notification_id")
	if uuid.Validate(notificationID) != nil {
		writeCommandFailure(w, r, invalidCommand("notification id is invalid"))
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload, status, failure := h.executeCommand(r.Context(), authenticatedActor(r).clientID, userID, "notification_read", r.URL.Path, key, nil, http.StatusOK, func(tx pgx.Tx) (any, *commandFailure) {
		var value notificationView
		now := h.now().UTC()
		err := tx.QueryRow(r.Context(), `
			UPDATE account_portfolio_notifications
			SET read_at=COALESCE(read_at, $3)
			WHERE id=$1 AND user_id=$2
			RETURNING id, title, body, kind, ticket_id::text, read_at, created_at
		`, notificationID, userID, now).Scan(&value.ID, &value.Title, &value.Body, &value.Kind, &value.TicketID, &value.ReadAt, &value.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFoundFailure("notification was not found")
		}
		if err != nil {
			return nil, dependencyFailure("Account Portfolio notifications are unavailable")
		}
		setTicketReference(&value)
		return map[string]any{"notification": value}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
}

func membershipTargetID(r *http.Request) (string, *commandFailure) {
	userID := chi.URLParam(r, "user_id")
	if uuid.Validate(userID) != nil {
		return "", invalidCommand("membership user id is invalid")
	}
	return userID, nil
}

func (h *service) consoleMembershipByUserID(ctx context.Context, userID string) (consoleMembershipView, error) {
	var value consoleMembershipView
	err := h.database.QueryRow(ctx, `
		SELECT plan, version
		FROM account_portfolio_memberships
		WHERE user_id=$1
	`, userID).Scan(&value.Plan, &value.Version)
	if err == nil {
		value.Lifetime = value.Plan == "lifetime"
	}
	return value, err
}

func (h *service) ownerTicket(ctx context.Context, userID, ticketID string) (ticketView, error) {
	var value ticketView
	err := h.database.QueryRow(ctx, `
		SELECT id, title, category, status, version, created_at, updated_at
		FROM account_portfolio_tickets
		WHERE id=$1 AND user_id=$2
	`, ticketID, userID).Scan(&value.ID, &value.Title, &value.Category, &value.Status, &value.Version, &value.CreatedAt, &value.UpdatedAt)
	if err == nil {
		value.Reference = ticketReference(value.ID)
	}
	return value, err
}

func (h *service) ownerTicketMessages(ctx context.Context, userID, ticketID string) ([]ticketMessageView, error) {
	rows, err := h.database.Query(ctx, `
		SELECT m.id, m.author_kind, m.body, m.created_at
		FROM account_portfolio_ticket_messages m
		JOIN account_portfolio_tickets t ON t.id=m.ticket_id
		WHERE m.ticket_id=$1 AND t.user_id=$2
		ORDER BY m.created_at ASC, m.id ASC
	`, ticketID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ticketMessageView, 0)
	for rows.Next() {
		var value ticketMessageView
		if err := rows.Scan(&value.ID, &value.AuthorKind, &value.Body, &value.CreatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (h *service) ownerTicketEvents(ctx context.Context, userID, ticketID string) ([]ticketEventView, error) {
	rows, err := h.database.Query(ctx, `
		SELECT e.id, e.kind, e.from_status, e.to_status, e.created_at
		FROM account_portfolio_ticket_events e
		JOIN account_portfolio_tickets t ON t.id=e.ticket_id
		WHERE e.ticket_id=$1 AND t.user_id=$2
		ORDER BY e.created_at ASC, e.id ASC
	`, ticketID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ticketEventView, 0)
	for rows.Next() {
		var value ticketEventView
		if err := rows.Scan(&value.ID, &value.Kind, &value.FromStatus, &value.ToStatus, &value.CreatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (h *service) ticketByID(ctx context.Context, ticketID string) (ticketRecord, error) {
	var value ticketRecord
	err := h.database.QueryRow(ctx, `
		SELECT id, user_id, title, category, status, version, created_at, updated_at
		FROM account_portfolio_tickets
		WHERE id=$1
	`, ticketID).Scan(&value.Ticket.ID, &value.UserID, &value.Ticket.Title, &value.Ticket.Category, &value.Ticket.Status, &value.Ticket.Version, &value.Ticket.CreatedAt, &value.Ticket.UpdatedAt)
	if err == nil {
		value.Ticket.Reference = ticketReference(value.Ticket.ID)
	}
	return value, err
}

func (h *service) ticketMessages(ctx context.Context, ticketID string) ([]ticketMessageView, error) {
	rows, err := h.database.Query(ctx, `
		SELECT id, author_kind, body, created_at
		FROM account_portfolio_ticket_messages
		WHERE ticket_id=$1
		ORDER BY created_at ASC, id ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ticketMessageView, 0)
	for rows.Next() {
		var value ticketMessageView
		if err := rows.Scan(&value.ID, &value.AuthorKind, &value.Body, &value.CreatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (h *service) ticketEvents(ctx context.Context, ticketID string) ([]ticketEventView, error) {
	rows, err := h.database.Query(ctx, `
		SELECT id, kind, from_status, to_status, created_at
		FROM account_portfolio_ticket_events
		WHERE ticket_id=$1
		ORDER BY created_at ASC, id ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]ticketEventView, 0)
	for rows.Next() {
		var value ticketEventView
		if err := rows.Scan(&value.ID, &value.Kind, &value.FromStatus, &value.ToStatus, &value.CreatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (h *service) createTicketNotification(ctx context.Context, tx pgx.Tx, ticketID, eventID, userID, kind, title, body string, createdAt time.Time) *commandFailure {
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_notifications(id, user_id, title, body, kind, ticket_id, ticket_event_id, created_at)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8)
	`, uuid.NewString(), userID, title, body, kind, ticketID, eventID, createdAt); err != nil {
		return dependencyFailure("Account Portfolio notification delivery is unavailable")
	}
	return nil
}

func (h *service) createMembershipNotification(ctx context.Context, tx pgx.Tx, eventID, userID, kind, title, body string, createdAt time.Time) *commandFailure {
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_notifications(id, user_id, title, body, kind, membership_event_id, created_at)
		VALUES($1, $2, $3, $4, $5, $6, $7)
	`, uuid.NewString(), userID, title, body, kind, eventID, createdAt); err != nil {
		return dependencyFailure("Account Portfolio membership notification delivery is unavailable")
	}
	return nil
}

func (h *service) createPointNotification(ctx context.Context, tx pgx.Tx, ledgerID, userID string, amount, balance int64, createdAt time.Time) *commandFailure {
	delta := strconv.FormatInt(amount, 10)
	if amount > 0 {
		delta = "+" + delta
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_notifications(id, user_id, title, body, kind, point_ledger_id, created_at)
		VALUES($1, $2, '积分余额已调整', $3, 'points_adjusted', $4, $5)
	`, uuid.NewString(), userID, "你的积分已调整 "+delta+"，当前余额为 "+strconv.FormatInt(balance, 10)+"。", ledgerID, createdAt); err != nil {
		return dependencyFailure("Account Portfolio point notification delivery is unavailable")
	}
	return nil
}

func allowedTicketTransition(from, to string) bool {
	return (from == "open" && (to == "in_progress" || to == "resolved")) || (from == "in_progress" && to == "resolved")
}

func ticketStatusLabel(status string) string {
	switch status {
	case "in_progress":
		return "处理中"
	case "resolved":
		return "已解决"
	default:
		return status
	}
}

func (h *service) executeCommand(ctx context.Context, clientID, actorUserID, operation, targetPath, key string, raw []byte, status int, execute func(pgx.Tx) (any, *commandFailure)) (json.RawMessage, int, *commandFailure) {
	digest := commandRequestHash(targetPath, raw)
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return nil, 0, dependencyFailure("Account Portfolio command store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inserted, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_command_idempotency(client_id, actor_user_id, operation, idempotency_key, request_hash, response_status, response_payload)
		VALUES($1, $2, $3, $4, $5, $6, '{}'::jsonb)
		ON CONFLICT (client_id, actor_user_id, operation, idempotency_key) DO NOTHING
	`, clientID, actorUserID, operation, key, digest[:], status)
	if err != nil {
		return nil, 0, dependencyFailure("Account Portfolio command store is unavailable")
	}
	if inserted.RowsAffected() == 0 {
		var storedHash, storedPayload []byte
		var storedStatus int16
		if err := tx.QueryRow(ctx, `
			SELECT request_hash, response_status, response_payload
			FROM account_portfolio_command_idempotency
			WHERE client_id=$1 AND actor_user_id=$2 AND operation=$3 AND idempotency_key=$4
		`, clientID, actorUserID, operation, key).Scan(&storedHash, &storedStatus, &storedPayload); err != nil {
			return nil, 0, dependencyFailure("Account Portfolio command store is unavailable")
		}
		if !bytes.Equal(storedHash, digest[:]) {
			return nil, 0, &commandFailure{status: http.StatusConflict, code: "IDEMPOTENCY_KEY_REUSED", message: "Idempotency-Key belongs to a different command payload"}
		}
		return json.RawMessage(storedPayload), int(storedStatus), nil
	}
	result, failure := execute(tx)
	if failure != nil {
		return nil, 0, failure
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, 0, dependencyFailure("Account Portfolio command serialization is unavailable")
	}
	updated, err := tx.Exec(ctx, `
		UPDATE account_portfolio_command_idempotency
		SET response_payload=$5
		WHERE client_id=$1 AND actor_user_id=$2 AND operation=$3 AND idempotency_key=$4
	`, clientID, actorUserID, operation, key, payload)
	if err != nil || updated.RowsAffected() != 1 {
		return nil, 0, dependencyFailure("Account Portfolio command store is unavailable")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, dependencyFailure("Account Portfolio command store is unavailable")
	}
	return json.RawMessage(payload), status, nil
}

// commandRequestHash binds a retry key to both its payload and the concrete
// command target. This prevents a valid retry for one ticket or notification
// from being mistaken for a command against another resource.
func commandRequestHash(targetPath string, raw []byte) [sha256.Size]byte {
	hash := sha256.New()
	_, _ = hash.Write([]byte(targetPath))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(raw)
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}

func decodeCommand(r *http.Request, target any) ([]byte, *commandFailure) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
	if err != nil {
		return nil, invalidCommand("request body is invalid")
	}
	if len(raw) > 64<<10 {
		return nil, invalidCommand("request body is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return nil, invalidCommand("request body is invalid")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, invalidCommand("request body must contain one JSON object")
	}
	return raw, nil
}

func idempotencyKey(r *http.Request) (string, *commandFailure) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 200 || !idempotencyKeyPattern.MatchString(key) {
		return "", &commandFailure{status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY", message: "Idempotency-Key is required"}
	}
	return key, nil
}

func (h *service) pointPage(r *http.Request, userID string) (int, *pointCursor, *commandFailure) {
	query := r.URL.Query()
	for key := range query {
		if key != "limit" && key != "cursor" {
			return 0, nil, invalidCommand("point page query is invalid")
		}
	}
	if len(query["limit"]) > 1 || len(query["cursor"]) > 1 {
		return 0, nil, invalidCommand("point page query is invalid")
	}
	limit := 20
	if values, exists := query["limit"]; exists {
		rawLimit := values[0]
		if rawLimit == "" || strings.TrimSpace(rawLimit) != rawLimit {
			return 0, nil, invalidCommand("point page limit is invalid")
		}
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 50 {
			return 0, nil, invalidCommand("point page limit is invalid")
		}
		limit = parsed
	}
	values, exists := query["cursor"]
	if !exists {
		return limit, nil, nil
	}
	rawCursor := values[0]
	if rawCursor == "" || strings.TrimSpace(rawCursor) != rawCursor {
		return 0, nil, invalidCommand("point page cursor is invalid")
	}
	if len(rawCursor) > 512 {
		return 0, nil, invalidCommand("point page cursor is invalid")
	}
	cursor, err := h.pointCursors.decode(rawCursor, userID, h.now().UTC())
	if err != nil {
		return 0, nil, invalidCommand("point page cursor is invalid")
	}
	return limit, &cursor, nil
}

func ticketReference(id string) string {
	return "HKT-" + strings.ToLower(id)
}

func setTicketReference(value *notificationView) {
	if value.TicketID == nil {
		return
	}
	reference := ticketReference(*value.TicketID)
	value.TicketReference = &reference
}

func invalidCommand(message string) *commandFailure {
	return &commandFailure{status: http.StatusBadRequest, code: "INVALID_REQUEST", message: message}
}

func notFoundFailure(message string) *commandFailure {
	return &commandFailure{status: http.StatusNotFound, code: "NOT_FOUND", message: message}
}

func versionConflictFailure() *commandFailure {
	return &commandFailure{status: http.StatusConflict, code: "VERSION_CONFLICT", message: "ticket version no longer matches"}
}

func membershipVersionConflictFailure() *commandFailure {
	return &commandFailure{status: http.StatusConflict, code: "VERSION_CONFLICT", message: "membership version no longer matches"}
}

func invalidStateFailure(message string) *commandFailure {
	return &commandFailure{status: http.StatusConflict, code: "INVALID_STATE", message: message}
}

func dependencyFailure(message string) *commandFailure {
	return &commandFailure{status: http.StatusServiceUnavailable, code: "DEPENDENCY_UNAVAILABLE", message: message}
}

func writeCommandFailure(w http.ResponseWriter, r *http.Request, failure *commandFailure) {
	writeError(w, r, failure.status, failure.code, failure.message)
}

func authenticatedActor(r *http.Request) actor {
	value, _ := r.Context().Value(actorKey).(actor)
	return value
}

func (h *service) membershipOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	data := struct {
		Orders []membershipOrderView `json:"orders"`
	}{Orders: make([]membershipOrderView, 0)}
	rows, err := h.database.Query(r.Context(), `
		SELECT id, plan, amount_cents, status, version, created_at, updated_at
		FROM account_portfolio_membership_orders
		WHERE user_id=$1
		ORDER BY created_at DESC, id DESC
		LIMIT 100
	`, userID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio orders are unavailable")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value membershipOrderView
		if err := rows.Scan(&value.ID, &value.Plan, &value.AmountCents, &value.Status, &value.Version, &value.CreatedAt, &value.UpdatedAt); err != nil {
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
	if value.clientID != h.clientID {
		writeError(w, r, http.StatusForbidden, "ACCESS_DENIED", "Account Portfolio owner route requires Portal Gateway")
		return "", false
	}
	if err := h.ensureAccount(r.Context(), value.userID); err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio is unavailable")
		return "", false
	}
	return value.userID, true
}

func (h *service) prepareConsole(w http.ResponseWriter, r *http.Request) (actor, bool) {
	value := authenticatedActor(r)
	if value.userID == "" {
		writeError(w, r, http.StatusUnauthorized, "INVALID_ACTOR", "Account Portfolio actor is invalid")
		return actor{}, false
	}
	if h.consoleClientID == "" || value.clientID != h.consoleClientID {
		writeError(w, r, http.StatusForbidden, "ACCESS_DENIED", "Account Portfolio Console route requires Console Gateway")
		return actor{}, false
	}
	return value, true
}

func (h *service) ensureAccount(ctx context.Context, userID string) error {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := ensureAccountTx(ctx, tx, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ensureAccountTx(ctx context.Context, tx pgx.Tx, userID string) error {
	for _, statement := range []string{
		`INSERT INTO account_portfolio_accounts(user_id) VALUES($1) ON CONFLICT (user_id) DO NOTHING`,
		`INSERT INTO account_portfolio_points(user_id) VALUES($1) ON CONFLICT (user_id) DO NOTHING`,
		`INSERT INTO account_portfolio_memberships(user_id) VALUES($1) ON CONFLICT (user_id) DO NOTHING`,
	} {
		if _, err := tx.Exec(ctx, statement, userID); err != nil {
			return err
		}
	}
	return nil
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
