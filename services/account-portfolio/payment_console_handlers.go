package accountportfolio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

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
	context_, ok := h.beginConsoleOrderCommand(w, r, "membership_order_close", []MembershipOrderStatus{MembershipOrderCreated, MembershipOrderPendingPayment})
	if !ok {
		return
	}
	if context_.Replayed {
		writeRawCommandData(w, r, context_.StoredStatus, context_.StoredPayload)
		return
	}
	providerOrder, err := context_.Provider.CloseOrder(r.Context(), context_.MerchantOrderID)
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
	h.finishConsoleOrderCommand(r.Context(), context_, map[string]any{"order": view}, http.StatusOK)
	writeData(w, r, http.StatusOK, map[string]any{"order": view})
}

func (h *service) refundConsoleMembershipOrder(w http.ResponseWriter, r *http.Request) {
	context_, ok := h.beginConsoleOrderCommand(w, r, "membership_order_refund", []MembershipOrderStatus{MembershipOrderPaid})
	if !ok {
		return
	}
	if context_.Replayed {
		writeRawCommandData(w, r, context_.StoredStatus, context_.StoredPayload)
		return
	}
	refund, err := context_.Provider.Refund(r.Context(), context_.MerchantOrderID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment provider is unavailable")
		return
	}
	// An abnormal refund needs operator handling and must never be recorded as
	// either a completed or a harmlessly pending refund.
	if refund.Status == MembershipRefundAbnormal {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio refund needs operator handling")
		return
	}
	view, revoked, failure := h.settleConsoleRefund(r.Context(), context_.Provider.Name(), context_.Order, refund)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	record, failure := h.upsertConsoleMembershipRefund(r.Context(), context_.Order.ID, context_.Provider.Name(), context_.MerchantOrderID, refund, revoked)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	payload := map[string]any{"order": view, "refund": consoleRefundView(record)}
	h.finishConsoleOrderCommand(r.Context(), context_, payload, http.StatusAccepted)
	writeData(w, r, http.StatusAccepted, payload)
}

// getConsoleMembershipOrderRefund reconciles one stored refund against the
// provider. It is read-only for the caller, but it still settles a refund that
// has completed since it was submitted, so a refund cannot stay pending
// forever, and it persists every reconciled state — including closed and
// abnormal — so the contract enum is reachable.
func (h *service) getConsoleMembershipOrderRefund(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.prepareConsole(w, r); !ok {
		return
	}
	orderID, failure := consoleOrderTargetID(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	refundID := strings.TrimSpace(chi.URLParam(r, "refund_id"))
	if uuid.Validate(refundID) != nil {
		writeCommandFailure(w, r, invalidCommand("membership order refund id is invalid"))
		return
	}
	provider := h.paymentProvider
	if provider == nil {
		writeError(w, r, http.StatusServiceUnavailable, "PAYMENT_PROVIDER_UNAVAILABLE", "Membership payment is not available")
		return
	}
	record, failure := h.membershipOrderRefundByID(r.Context(), refundID)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	if record.OrderID != orderID || record.ProviderName != provider.Name() {
		writeCommandFailure(w, r, notFoundFailure("membership order refund was not found"))
		return
	}
	target, _, failure := h.consoleOrderTarget(r.Context(), orderID, provider.Name())
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	view := target.View
	refund, err := provider.QueryRefund(r.Context(), record.MerchantOrderID)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment provider is unavailable")
		return
	}
	revoked := false
	if refund.Status == MembershipRefundSucceeded {
		var reconcileFailure *commandFailure
		view, revoked, reconcileFailure = h.settleConsoleRefund(r.Context(), provider.Name(), view, refund)
		if reconcileFailure != nil {
			writeCommandFailure(w, r, reconcileFailure)
			return
		}
	}
	updated, failure := h.updateConsoleMembershipRefund(r.Context(), record.ID, refund.Status, revoked)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"order": view, "refund": consoleRefundView(updated)})
}

// settleConsoleRefund applies a refund only once the provider says it settled.
// An unsettled refund is not a refund fact: the order stays paid and the
// lifetime entitlement stays granted until the provider confirms. A settled one
// goes through the same durable path a provider notification takes, which is
// what keeps the revocation guarded by payment-fact ownership.
func (h *service) settleConsoleRefund(
	ctx context.Context, providerName string, current membershipOrderView, refund PaymentRefund,
) (membershipOrderView, bool, *commandFailure) {
	if refund.Status != MembershipRefundSucceeded {
		return current, false, nil
	}
	applied, _, revoked, failure := h.applyVerifiedPaymentNotification(
		ctx, providerName, refund.Notification, notificationDigest(refund.Notification),
	)
	if failure != nil {
		return membershipOrderView{}, false, failure
	}
	if applied.ID == "" {
		return current, false, nil
	}
	return applied, revoked, nil
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
	Provider        PaymentProvider
	Order           membershipOrderView
	MerchantOrderID string
	ClientID        string
	OperatorID      string
	Operation       string
	Key             string
	Replayed        bool
	StoredStatus    int
	StoredPayload   json.RawMessage
}

// beginConsoleOrderCommand authorizes the operator, validates the target order
// and its expected revision, and claims the Idempotency-Key — all before the
// provider is contacted.
func (h *service) beginConsoleOrderCommand(
	w http.ResponseWriter, r *http.Request, operation string, required []MembershipOrderStatus,
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
	orderID string, expectedVersion int, providerName string, required []MembershipOrderStatus,
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
		var storedStatus int
		if err := tx.QueryRow(ctx, `
			SELECT request_hash, response_payload, response_status
			FROM account_portfolio_command_idempotency
			WHERE client_id=$1 AND actor_user_id=$2 AND operation=$3 AND idempotency_key=$4
		`, clientID, operatorID, operation, key).Scan(&storedHash, &storedPayload, &storedStatus); err != nil {
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
		return consoleOrderCommandContext{Replayed: true, StoredStatus: storedStatus, StoredPayload: json.RawMessage(storedPayload)}, nil
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
	if !orderStatusAllowed(required, MembershipOrderStatus(intent.Order.View.Status)) {
		return consoleOrderCommandContext{}, invalidStateFailure("membership order is not in the required state")
	}
	if err := tx.Commit(ctx); err != nil {
		return consoleOrderCommandContext{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	return consoleOrderCommandContext{Order: intent.Order.View, MerchantOrderID: intent.MerchantOrderID}, nil
}

func orderStatusAllowed(allowed []MembershipOrderStatus, current MembershipOrderStatus) bool {
	for _, candidate := range allowed {
		if current == candidate {
			return true
		}
	}
	return false
}

// finishConsoleOrderCommand stores the response so a later retry of the same
// key replays it instead of contacting the provider again. The stored status is
// the status the original command returned, so a replay answers identically. A
// storage failure here does not fail the command: the durable payment effect
// already committed, and the retry path re-derives the same result from the
// provider.
func (h *service) finishConsoleOrderCommand(ctx context.Context, command consoleOrderCommandContext, payload map[string]any, status int) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = h.database.Exec(ctx, `
		UPDATE account_portfolio_command_idempotency
		SET response_payload=$5, response_status=$6
		WHERE client_id=$1 AND actor_user_id=$2 AND operation=$3 AND idempotency_key=$4
	`, command.ClientID, command.OperatorID, command.Operation, command.Key, encoded, status)
}

// consoleOrderTarget loads one membership order and its private merchant order
// number.
func (h *service) consoleOrderTarget(ctx context.Context, orderID, providerName string) (membershipOrderRecord, string, *commandFailure) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderRecord{}, "", dependencyFailure("Account Portfolio payment store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	intent, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return membershipOrderRecord{}, "", notFoundFailure("membership order was not found")
	}
	if err != nil {
		return membershipOrderRecord{}, "", dependencyFailure("Account Portfolio payment order is unavailable")
	}
	if intent.Order.Provider != providerName {
		return membershipOrderRecord{}, "", notFoundFailure("membership order was not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderRecord{}, "", dependencyFailure("Account Portfolio payment store is unavailable")
	}
	return intent.Order, intent.MerchantOrderID, nil
}

func consoleOrderTargetID(r *http.Request) (string, *commandFailure) {
	value := strings.TrimSpace(chi.URLParam(r, "order_id"))
	if uuid.Validate(value) != nil {
		return "", invalidCommand("membership order id is invalid")
	}
	return value, nil
}

// membershipOrderRefundRecord is the durable refund row. The public id is a
// random UUID; the gateway correlation (out_refund_no) and the private merchant
// order number stay server-side and never appear in any response.
type membershipOrderRefundRecord struct {
	ID                 string
	OrderID            string
	ProviderName       string
	MerchantOrderID    string
	OutRefundNo        string
	Status             string
	AmountCents        int
	EntitlementRevoked bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// upsertConsoleMembershipRefund persists one refund under the
// (provider_name, merchant_order_id) idempotency anchor, reusing the existing
// public id on retry, and returns the durable row the response view is built
// from. entitlement_revoked is monotonic: once a refund revoked the
// entitlement, later reconciliations may not clear it.
func (h *service) upsertConsoleMembershipRefund(ctx context.Context, orderID, providerName, merchantOrderID string, refund PaymentRefund, revoked bool) (membershipOrderRefundRecord, *commandFailure) {
	status := MembershipRefundProcessing
	if refund.Settled {
		status = MembershipRefundSucceeded
	}
	now := h.now().UTC()
	var record membershipOrderRefundRecord
	err := h.database.QueryRow(ctx, `
		INSERT INTO account_portfolio_membership_order_refunds(
			order_id, provider_name, merchant_order_id, out_refund_no, status, amount_cents,
			entitlement_revoked, created_at, updated_at
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (provider_name, merchant_order_id) DO UPDATE
		SET out_refund_no = EXCLUDED.out_refund_no,
			status = EXCLUDED.status,
			amount_cents = EXCLUDED.amount_cents,
			entitlement_revoked = account_portfolio_membership_order_refunds.entitlement_revoked OR EXCLUDED.entitlement_revoked,
			updated_at = EXCLUDED.updated_at
		RETURNING id, order_id, status, amount_cents, entitlement_revoked, created_at, updated_at
	`, orderID, providerName, merchantOrderID, refund.RefundID, string(status), refund.Notification.AmountCents, revoked, now).Scan(
		&record.ID, &record.OrderID, &record.Status, &record.AmountCents, &record.EntitlementRevoked, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return membershipOrderRefundRecord{}, dependencyFailure("Account Portfolio refund store is unavailable")
	}
	record.ProviderName = providerName
	record.MerchantOrderID = merchantOrderID
	record.OutRefundNo = refund.RefundID
	return record, nil
}

// updateConsoleMembershipRefund reconciles a stored refund's status against the
// provider. entitlement_revoked is monotonic exactly like the upsert path.
func (h *service) updateConsoleMembershipRefund(ctx context.Context, refundID string, status MembershipRefundStatus, revoked bool) (membershipOrderRefundRecord, *commandFailure) {
	var record membershipOrderRefundRecord
	err := h.database.QueryRow(ctx, `
		UPDATE account_portfolio_membership_order_refunds
		SET status=$2,
			entitlement_revoked = entitlement_revoked OR $3,
			updated_at=$4
		WHERE id=$1
		RETURNING id, order_id, provider_name, merchant_order_id, out_refund_no, status, amount_cents, entitlement_revoked, created_at, updated_at
	`, refundID, string(status), revoked, h.now().UTC()).Scan(
		&record.ID, &record.OrderID, &record.ProviderName, &record.MerchantOrderID, &record.OutRefundNo, &record.Status,
		&record.AmountCents, &record.EntitlementRevoked, &record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return membershipOrderRefundRecord{}, dependencyFailure("Account Portfolio refund store is unavailable")
	}
	return record, nil
}

func (h *service) membershipOrderRefundByID(ctx context.Context, refundID string) (membershipOrderRefundRecord, *commandFailure) {
	var record membershipOrderRefundRecord
	err := h.database.QueryRow(ctx, `
		SELECT id, order_id, provider_name, merchant_order_id, out_refund_no, status, amount_cents, entitlement_revoked, created_at, updated_at
		FROM account_portfolio_membership_order_refunds
		WHERE id=$1
	`, refundID).Scan(
		&record.ID, &record.OrderID, &record.ProviderName, &record.MerchantOrderID, &record.OutRefundNo, &record.Status,
		&record.AmountCents, &record.EntitlementRevoked, &record.CreatedAt, &record.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return membershipOrderRefundRecord{}, notFoundFailure("membership order refund was not found")
	}
	if err != nil {
		return membershipOrderRefundRecord{}, dependencyFailure("Account Portfolio refund store is unavailable")
	}
	return record, nil
}

// consoleRefundView renders the durable refund row exactly as the contract
// declares it: the public UUID id, the order, the reconciled status, the
// amount, the entitlement-revocation fact, and the durable timestamps. No
// provider correlation ever leaves the service.
func consoleRefundView(record membershipOrderRefundRecord) map[string]any {
	return map[string]any{
		"id":                  record.ID,
		"order_id":            record.OrderID,
		"amount_cents":        record.AmountCents,
		"status":              record.Status,
		"entitlement_revoked": record.EntitlementRevoked,
		"created_at":          record.CreatedAt.UTC(),
		"updated_at":          record.UpdatedAt.UTC(),
	}
}

func notificationDigest(notification VerifiedPaymentNotification) [sha256.Size]byte {
	raw, err := json.Marshal(notification)
	if err != nil {
		return sha256.Sum256([]byte(notification.EventID))
	}
	return sha256.Sum256(raw)
}

func writeRawCommandData(w http.ResponseWriter, r *http.Request, status int, payload json.RawMessage) {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL", "stored command result is unreadable")
		return
	}
	writeData(w, r, status, decoded)
}
