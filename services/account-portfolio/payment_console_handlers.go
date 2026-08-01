package accountportfolio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Operator commands over one membership order.
//
// Each command names the order by its Account Portfolio identifier only, so the
// private merchant order number stays server-side and no caller can address, or
// learn, another tenant's order.
//
// The provider call happens between two transactions rather than inside one,
// matching the order-creation path: holding a row lock across a call to the
// gateway would let a slow gateway stall unrelated commands. The command's
// Idempotency-Key is claimed in the first transaction, so a retry short-circuits
// before reaching the provider and can never produce a second refund.

type consoleOrderCommandInput struct {
	Reason          string `json:"reason"`
	ExpectedVersion int    `json:"expected_version"`
}

func (h *service) closeConsoleMembershipOrder(w http.ResponseWriter, r *http.Request) {
	context_, ok := h.beginConsoleOrderCommand(w, r, "membership_order_close", MembershipOrderPendingPayment)
	if !ok {
		return
	}
	if context_.Replayed {
		writeRawCommandData(w, r, http.StatusOK, context_.StoredPayload)
		return
	}
	providerOrder, err := context_.Provider.CloseOrder(r.Context(), context_.ExternalOrderID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment provider is unavailable")
		return
	}
	if providerOrder.Status != MembershipOrderClosed {
		writeCommandFailure(w, r, invalidStateFailure("membership order was not closed"))
		return
	}
	// Closing records no payment fact: no money moved. Only the order transitions.
	view, failure := h.settleConsoleOrderClose(r.Context(), context_.Order.ID)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	h.finishConsoleOrderCommand(r.Context(), context_, map[string]any{"order": view})
	writeData(w, r, http.StatusOK, map[string]any{"order": view})
}

func (h *service) refundConsoleMembershipOrder(w http.ResponseWriter, r *http.Request) {
	context_, ok := h.beginConsoleOrderCommand(w, r, "membership_order_refund", MembershipOrderPaid)
	if !ok {
		return
	}
	if context_.Replayed {
		writeRawCommandData(w, r, http.StatusOK, context_.StoredPayload)
		return
	}
	refund, err := context_.Provider.Refund(r.Context(), context_.ExternalOrderID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment provider is unavailable")
		return
	}
	view, failure := h.settleConsoleRefund(r.Context(), context_.Provider.Name(), context_.Order, refund)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload := map[string]any{"order": view, "refund": consoleRefundView(refund)}
	h.finishConsoleOrderCommand(r.Context(), context_, payload)
	writeData(w, r, http.StatusAccepted, payload)
}

// getConsoleMembershipOrderRefund reconciles a refund against the provider. It
// is read-only for the caller, but it still settles a refund that has completed
// since it was submitted, so a refund cannot stay pending forever.
func (h *service) getConsoleMembershipOrderRefund(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	orderID, failure := consoleOrderTargetID(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	provider := h.paymentProvider
	if provider == nil {
		writeError(w, r, http.StatusServiceUnavailable, "PAYMENT_PROVIDER_UNAVAILABLE", "Membership payment is not available")
		return
	}
	record, failure := h.consoleOrderTarget(r.Context(), orderID, provider.Name())
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	refund, err := provider.QueryRefund(r.Context(), record.ProviderOrderID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment provider is unavailable")
		return
	}
	if strings.TrimSpace(chi.URLParam(r, "refund_id")) != refund.RefundID {
		writeCommandFailure(w, r, notFoundFailure("membership order refund was not found"))
		return
	}
	view, failure := h.settleConsoleRefund(r.Context(), provider.Name(), record.View, refund)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"order": view, "refund": consoleRefundView(refund)})
}

// settleConsoleRefund applies a refund only once the provider says it settled.
// An unsettled refund is not a refund fact: the order stays paid and the
// lifetime entitlement stays granted until the provider confirms. A settled one
// goes through the same durable path a provider notification takes, which is
// what keeps the revocation guarded by payment-fact ownership.
func (h *service) settleConsoleRefund(
	ctx context.Context, providerName string, current membershipOrderView, refund PaymentRefund,
) (membershipOrderView, *commandFailure) {
	if !refund.Settled {
		return current, nil
	}
	applied, _, failure := h.applyVerifiedPaymentNotification(
		ctx, providerName, refund.Notification, notificationDigest(refund.Notification),
	)
	if failure != nil {
		return membershipOrderView{}, failure
	}
	if applied.ID == "" {
		return current, nil
	}
	return applied, nil
}

func (h *service) settleConsoleOrderClose(ctx context.Context, orderID string) (membershipOrderView, *commandFailure) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	intent, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, orderID)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment order is unavailable")
	}
	if MembershipOrderStatus(intent.Order.View.Status) == MembershipOrderClosed {
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, dependencyFailure("Account Portfolio payment store is unavailable")
		}
		return intent.Order.View, nil
	}
	if !membershipOrderTransitionAllowed(MembershipOrderStatus(intent.Order.View.Status), MembershipOrderClosed) {
		return membershipOrderView{}, invalidStateFailure("membership order transition is not allowed")
	}
	view, err := h.transitionMembershipOrder(
		ctx, tx, orderID, MembershipOrderClosed, intent.Order.ProviderEventSequence, h.now().UTC(),
	)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio membership order is unavailable")
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment store is unavailable")
	}
	return view, nil
}

type consoleOrderCommandContext struct {
	Provider PaymentProvider
	Order    membershipOrderView
	// ExternalOrderID is the provider's own order identifier, which is what
	// every PaymentProvider method takes. It is not the private merchant order
	// number; the two coincide for EasyPay but must not be conflated.
	ExternalOrderID string
	ClientID        string
	OperatorID      string
	Operation       string
	Key             string
	Replayed        bool
	StoredPayload   json.RawMessage
}

// beginConsoleOrderCommand authorizes the operator, validates the target order
// and its expected revision, and claims the Idempotency-Key — all before the
// provider is contacted.
func (h *service) beginConsoleOrderCommand(
	w http.ResponseWriter, r *http.Request, operation string, required MembershipOrderStatus,
) (consoleOrderCommandContext, bool) {
	operator, ok := h.prepareConsole(w, r)
	if !ok {
		return consoleOrderCommandContext{}, false
	}
	orderID, failure := consoleOrderTargetID(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return consoleOrderCommandContext{}, false
	}
	var input consoleOrderCommandInput
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return consoleOrderCommandContext{}, false
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len([]rune(input.Reason)) == 0 || len([]rune(input.Reason)) > 1000 || input.ExpectedVersion < 1 {
		writeCommandFailure(w, r, invalidCommand("membership order command is invalid"))
		return consoleOrderCommandContext{}, false
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return consoleOrderCommandContext{}, false
	}
	provider := h.paymentProvider
	if provider == nil {
		writeError(w, r, http.StatusServiceUnavailable, "PAYMENT_PROVIDER_UNAVAILABLE", "Membership payment is not available")
		return consoleOrderCommandContext{}, false
	}

	claim, failure := h.claimConsoleOrderCommand(
		r.Context(), operator.clientID, operator.userID, operation, r.URL.Path, key, raw,
		orderID, input.ExpectedVersion, provider.Name(), required,
	)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return consoleOrderCommandContext{}, false
	}
	claim.Provider = provider
	claim.ClientID = operator.clientID
	claim.OperatorID = operator.userID
	claim.Operation = operation
	claim.Key = key
	return claim, true
}

func (h *service) claimConsoleOrderCommand(
	ctx context.Context, clientID, operatorID, operation, targetPath, key string, raw []byte,
	orderID string, expectedVersion int, providerName string, required MembershipOrderStatus,
) (consoleOrderCommandContext, *commandFailure) {
	digest := commandRequestHash(targetPath, raw)
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return consoleOrderCommandContext{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	inserted, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_command_idempotency(client_id, actor_user_id, operation, idempotency_key, request_hash, response_status, response_payload)
		VALUES($1, $2, $3, $4, $5, $6, '{}'::jsonb)
		ON CONFLICT (client_id, actor_user_id, operation, idempotency_key) DO NOTHING
	`, clientID, operatorID, operation, key, digest[:], http.StatusOK)
	if err != nil {
		return consoleOrderCommandContext{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	if inserted.RowsAffected() == 0 {
		var storedHash, storedPayload []byte
		if err := tx.QueryRow(ctx, `
			SELECT request_hash, response_payload
			FROM account_portfolio_command_idempotency
			WHERE client_id=$1 AND actor_user_id=$2 AND operation=$3 AND idempotency_key=$4
		`, clientID, operatorID, operation, key).Scan(&storedHash, &storedPayload); err != nil {
			return consoleOrderCommandContext{}, dependencyFailure("Account Portfolio command store is unavailable")
		}
		if !bytes.Equal(storedHash, digest[:]) {
			return consoleOrderCommandContext{}, &commandFailure{
				status: http.StatusConflict, code: "IDEMPOTENCY_KEY_REUSED",
				message: "Idempotency-Key belongs to a different command payload",
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return consoleOrderCommandContext{}, dependencyFailure("Account Portfolio command store is unavailable")
		}
		// The claim is written before the provider is contacted and only filled
		// in once the command has settled, so an empty payload means the
		// original command is still in flight or never completed. Returning it
		// would answer a duplicate with an empty success, so this reports a
		// conflict and the caller retries for the real result instead.
		if !completedCommandPayload(storedPayload) {
			return consoleOrderCommandContext{}, &commandFailure{
				status: http.StatusConflict, code: "COMMAND_IN_PROGRESS",
				message: "An earlier command with this Idempotency-Key has not finished yet",
			}
		}
		return consoleOrderCommandContext{Replayed: true, StoredPayload: json.RawMessage(storedPayload)}, nil
	}

	intent, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return consoleOrderCommandContext{}, notFoundFailure("membership order was not found")
	}
	if err != nil {
		return consoleOrderCommandContext{}, dependencyFailure("Account Portfolio payment order is unavailable")
	}
	if intent.Order.Provider != providerName {
		return consoleOrderCommandContext{}, notFoundFailure("membership order was not found")
	}
	if intent.Order.View.Version != expectedVersion {
		return consoleOrderCommandContext{}, membershipVersionConflictFailure()
	}
	if MembershipOrderStatus(intent.Order.View.Status) != required {
		return consoleOrderCommandContext{}, invalidStateFailure("membership order is not in the required state")
	}
	if err := tx.Commit(ctx); err != nil {
		return consoleOrderCommandContext{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	return consoleOrderCommandContext{Order: intent.Order.View, ExternalOrderID: intent.Order.ProviderOrderID}, nil
}

// finishConsoleOrderCommand stores the response so a later retry of the same
// key replays it instead of contacting the provider again. A storage failure
// here does not fail the command: the durable payment effect already committed,
// and the retry path re-derives the same result from the provider.
func (h *service) finishConsoleOrderCommand(ctx context.Context, command consoleOrderCommandContext, payload map[string]any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = h.database.Exec(ctx, `
		UPDATE account_portfolio_command_idempotency
		SET response_payload=$5
		WHERE client_id=$1 AND actor_user_id=$2 AND operation=$3 AND idempotency_key=$4
	`, command.ClientID, command.OperatorID, command.Operation, command.Key, encoded)
}

// consoleOrderTarget loads one membership order and its private merchant order
// number.
func (h *service) consoleOrderTarget(ctx context.Context, orderID, providerName string) (membershipOrderRecord, *commandFailure) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderRecord{}, dependencyFailure("Account Portfolio payment store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	intent, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return membershipOrderRecord{}, notFoundFailure("membership order was not found")
	}
	if err != nil {
		return membershipOrderRecord{}, dependencyFailure("Account Portfolio payment order is unavailable")
	}
	if intent.Order.Provider != providerName {
		return membershipOrderRecord{}, notFoundFailure("membership order was not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderRecord{}, dependencyFailure("Account Portfolio payment store is unavailable")
	}
	return intent.Order, nil
}

func consoleOrderTargetID(r *http.Request) (string, *commandFailure) {
	value := strings.TrimSpace(chi.URLParam(r, "order_id"))
	if uuid.Validate(value) != nil {
		return "", invalidCommand("membership order id is invalid")
	}
	return value, nil
}

func consoleRefundView(refund PaymentRefund) map[string]any {
	status := "processing"
	if refund.Settled {
		status = "succeeded"
	}
	return map[string]any{
		"id":           refund.RefundID,
		"status":       status,
		"amount_cents": refund.Notification.AmountCents,
	}
}

func notificationDigest(notification VerifiedPaymentNotification) [sha256.Size]byte {
	raw, err := json.Marshal(notification)
	if err != nil {
		return sha256.Sum256([]byte(notification.EventID))
	}
	return sha256.Sum256(raw)
}

// completedCommandPayload reports whether a stored idempotency payload is a
// real command result rather than the placeholder written when the command was
// claimed.
func completedCommandPayload(payload []byte) bool {
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return false
	}
	_, ok := decoded["order"]
	return ok
}

func writeRawCommandData(w http.ResponseWriter, r *http.Request, status int, payload json.RawMessage) {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL", "stored command result is unreadable")
		return
	}
	writeData(w, r, status, decoded)
}
