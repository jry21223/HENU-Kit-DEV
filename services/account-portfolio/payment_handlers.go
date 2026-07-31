package accountportfolio

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type membershipOrderView struct {
	ID          string    `json:"id"`
	Plan        string    `json:"plan"`
	AmountCents int       `json:"amount_cents"`
	Status      string    `json:"status"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type membershipOrderRecord struct {
	View                  membershipOrderView
	UserID                string
	Provider              string
	ProviderOrderID       string
	ProviderEventSequence int64
}

type paymentOrderIntentRecord struct {
	Order                  membershipOrderRecord
	MerchantOrderID        string
	DeliveryState          string
	DeliveryLeaseID        string
	DeliveryLeaseExpiresAt *time.Time
}

var errProviderOrderConflicted = errors.New("provider order is already bound to another membership order")

const paymentOrderDeliveryLease = 30 * time.Second

func (h *service) createMembershipOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.prepareAccount(w, r)
	if !ok {
		return
	}
	var input struct{}
	raw, failure := decodeCommand(r, &input)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	key, failure := idempotencyKey(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	provider := h.paymentProvider
	if provider == nil {
		writeError(w, r, http.StatusServiceUnavailable, "PAYMENT_PROVIDER_UNAVAILABLE", "Membership payment is not available")
		return
	}
	providerName := provider.Name()
	if !validPaymentProviderName(providerName) {
		writeError(w, r, http.StatusServiceUnavailable, "PAYMENT_PROVIDER_UNAVAILABLE", "Membership payment is not available")
		return
	}

	order, failure := h.persistMembershipOrderIntent(r.Context(), authenticatedActor(r).clientID, userID, r.URL.Path, key, raw, providerName)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	order, failure = h.dispatchMembershipOrderIntent(r.Context(), provider, order.ID)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, http.StatusCreated, map[string]any{"order": order})
}

func (h *service) paymentProviderNotification(w http.ResponseWriter, r *http.Request) {
	provider := h.paymentProvider
	if provider == nil {
		writeError(w, r, http.StatusServiceUnavailable, "PAYMENT_PROVIDER_UNAVAILABLE", "Membership payment is not available")
		return
	}
	providerName := chi.URLParam(r, "provider")
	if !validPaymentProviderName(providerName) || providerName != provider.Name() {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "payment provider was not found")
		return
	}
	raw, failure := readPaymentNotification(r)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	digest := paymentDigest(raw)
	notification, err := provider.VerifyNotification(r.Context(), raw)
	if err != nil {
		if auditErr := h.auditPaymentNotification(r.Context(), providerName, nil, nil, "notification_rejected", "notification_verification_failed", digest); auditErr != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment audit is unavailable")
			return
		}
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "payment notification is invalid")
		return
	}
	if !validVerifiedPaymentNotification(notification) {
		if auditErr := h.auditPaymentNotification(r.Context(), providerName, nil, nil, "notification_rejected", "notification_shape_invalid", digest); auditErr != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment audit is unavailable")
			return
		}
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "payment notification is invalid")
		return
	}
	providerOrder, err := provider.QueryOrder(r.Context(), notification.ExternalOrderID)
	if err != nil {
		if auditErr := h.auditPaymentNotification(r.Context(), providerName, nil, nil, "notification_query_failed", "provider_query_failed", digest); auditErr != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment audit is unavailable")
			return
		}
		writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Membership payment verification is unavailable")
		return
	}
	if !validProviderOrderCorrelation(notification, providerOrder) {
		if auditErr := h.auditPaymentNotification(r.Context(), providerName, nil, nil, "notification_rejected", "provider_query_mismatch", digest); auditErr != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment audit is unavailable")
			return
		}
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "payment notification is invalid")
		return
	}

	if failure := h.recoverProviderOrderFromNotification(r.Context(), providerName, notification, providerOrder, digest); failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	order, outcome, failure := h.applyVerifiedPaymentNotification(r.Context(), providerName, notification, digest)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"accepted": true, "outcome": outcome, "order": order})
}

// reconcilePendingMembershipOrders recovers a lost callback through the
// Provider's signed merchant-order query when the owner next reads their
// orders. The query-derived fact uses a deterministic event id, so repeated or
// concurrent reads cannot grant the entitlement twice.
func (h *service) reconcilePendingMembershipOrders(ctx context.Context, userID string) *commandFailure {
	provider := h.paymentProvider
	if provider == nil {
		return nil
	}
	providerName := provider.Name()
	if !validPaymentProviderName(providerName) {
		return dependencyFailure("Membership payment reconciliation is unavailable")
	}
	rows, err := h.database.Query(ctx, `
		SELECT o.provider_order_id, i.merchant_order_id::text
		FROM account_portfolio_membership_orders o
		JOIN account_portfolio_payment_order_intents i ON i.order_id=o.id
		WHERE o.user_id=$1 AND o.provider=$2 AND i.provider=$2
		  AND o.status='pending_payment' AND o.provider_order_id IS NOT NULL
		ORDER BY o.created_at ASC, o.id ASC
		LIMIT 100
	`, userID, providerName)
	if err != nil {
		return dependencyFailure("Account Portfolio payment reconciliation is unavailable")
	}
	type candidate struct {
		externalOrderID string
		merchantOrderID string
	}
	candidates := make([]candidate, 0)
	for rows.Next() {
		var value candidate
		if err := rows.Scan(&value.externalOrderID, &value.merchantOrderID); err != nil {
			rows.Close()
			return dependencyFailure("Account Portfolio payment reconciliation is unavailable")
		}
		candidates = append(candidates, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return dependencyFailure("Account Portfolio payment reconciliation is unavailable")
	}
	rows.Close()

	for _, value := range candidates {
		providerOrder, err := provider.QueryOrder(ctx, value.externalOrderID)
		if err != nil {
			return dependencyFailure("Membership payment verification is unavailable")
		}
		if providerOrder.Status == MembershipOrderPendingPayment {
			continue
		}
		notification := VerifiedPaymentNotification{
			EventID:         "query:" + providerName + ":" + value.merchantOrderID + ":" + string(providerOrder.Status),
			ExternalOrderID: providerOrder.ExternalOrderID,
			MerchantOrderID: providerOrder.MerchantOrderID,
			AmountCents:     providerOrder.AmountCents,
			Currency:        providerOrder.Currency,
			Plan:            providerOrder.Plan,
			Status:          providerOrder.Status,
			Sequence:        1,
			OccurredAt:      h.now().UTC(),
		}
		if value.merchantOrderID != notification.MerchantOrderID ||
			!validVerifiedPaymentNotification(notification) ||
			!validProviderOrderCorrelation(notification, providerOrder) {
			return dependencyFailure("Membership payment verification is unavailable")
		}
		// Query retries may observe different local clocks. Digest the stable
		// Provider-derived event id so concurrent reconciliation treats the
		// same authoritative state as a replay rather than payload reuse.
		if _, _, failure := h.applyVerifiedPaymentNotification(ctx, providerName, notification, paymentDigest([]byte(notification.EventID))); failure != nil {
			return failure
		}
	}
	return nil
}

// persistMembershipOrderIntent commits the local order and its stable merchant
// id before a Provider call. This is the durability boundary: all retries use
// the same merchant id, so Provider creation remains idempotent after a crash.
func (h *service) persistMembershipOrderIntent(ctx context.Context, clientID, userID, targetPath, key string, raw []byte, providerName string) (membershipOrderView, *commandFailure) {
	digest := commandRequestHash(targetPath, raw)
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inserted, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_command_idempotency(client_id, actor_user_id, operation, idempotency_key, request_hash, response_status, response_payload)
		VALUES($1, $2, 'membership_order_create', $3, $4, $5, '{}'::jsonb)
		ON CONFLICT (client_id, actor_user_id, operation, idempotency_key) DO NOTHING
	`, clientID, userID, key, digest[:], http.StatusCreated)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	if inserted.RowsAffected() == 0 {
		var storedHash []byte
		if err := tx.QueryRow(ctx, `
			SELECT request_hash
			FROM account_portfolio_command_idempotency
			WHERE client_id=$1 AND actor_user_id=$2 AND operation='membership_order_create' AND idempotency_key=$3
		`, clientID, userID, key).Scan(&storedHash); err != nil {
			return membershipOrderView{}, dependencyFailure("Account Portfolio command store is unavailable")
		}
		if !bytes.Equal(storedHash, digest[:]) {
			return membershipOrderView{}, &commandFailure{status: http.StatusConflict, code: "IDEMPOTENCY_KEY_REUSED", message: "Idempotency-Key belongs to a different command payload"}
		}
		order, err := membershipOrderByUserIdempotency(ctx, tx, userID, key)
		if err != nil {
			return membershipOrderView{}, dependencyFailure("Account Portfolio membership order is unavailable")
		}
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, dependencyFailure("Account Portfolio command store is unavailable")
		}
		return order.View, nil
	}

	now := h.now().UTC()
	merchantOrderID, err := newMembershipMerchantOrderID()
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	order := membershipOrderView{
		ID:          uuid.NewString(),
		Plan:        "lifetime",
		AmountCents: lifetimeMembershipAmountCents,
		Status:      string(MembershipOrderCreated),
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_membership_orders(
			id, user_id, plan, amount_cents, status, provider, idempotency_key,
			version, provider_event_sequence, created_at, updated_at
		)
		VALUES($1, $2, 'lifetime', $3, 'created', $4, $5, 1, 0, $6, $6)
	`, order.ID, userID, lifetimeMembershipAmountCents, providerName, key, now); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio membership order is unavailable")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_payment_order_intents(
			order_id, provider, merchant_order_id, delivery_state, created_at, updated_at
		)
		VALUES($1, $2, $3, 'pending', $4, $4)
	`, order.ID, providerName, merchantOrderID, now); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	if failure := h.recordPaymentAudit(ctx, tx, &order.ID, nil, providerName, "order_intent_persisted", "merchant_order_persisted", paymentDigest(raw), now); failure != nil {
		return membershipOrderView{}, failure
	}
	payload, err := json.Marshal(map[string]any{"order": order})
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio command serialization is unavailable")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE account_portfolio_command_idempotency
		SET response_payload=$5
		WHERE client_id=$1 AND actor_user_id=$2 AND operation='membership_order_create' AND idempotency_key=$3 AND request_hash=$4
	`, clientID, userID, key, digest[:], payload); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio command store is unavailable")
	}
	return order, nil
}

// dispatchMembershipOrderIntent performs every Provider call after the intent
// claim transaction has committed. The short-lived lease makes concurrent
// idempotency retries observe one active dispatch, while the stable merchant
// order id makes a recovered retry safe after a process crash.
func (h *service) dispatchMembershipOrderIntent(ctx context.Context, provider PaymentProvider, orderID string) (membershipOrderView, *commandFailure) {
	intent, claimed, err := h.claimPaymentOrderDelivery(ctx, provider.Name(), orderID)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	if !claimed {
		return intent.Order.View, nil
	}
	signed, err := provider.Sign(ctx, PaymentOrderRequest{
		MerchantOrderID: intent.MerchantOrderID,
		AmountCents:     lifetimeMembershipAmountCents,
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
	})
	if err != nil {
		return h.recordRecoverableDeliveryFailure(ctx, intent, "provider_sign_failed")
	}
	providerOrder, err := provider.CreateOrder(ctx, signed)
	if err != nil {
		return h.recordRecoverableDeliveryFailure(ctx, intent, "provider_create_failed")
	}
	if !validCreatedProviderOrder(intent, providerOrder) {
		return h.recordRecoverableDeliveryFailure(ctx, intent, "provider_response_invalid")
	}
	bound, err := h.bindProviderOrder(ctx, intent, providerOrder, "order_dispatched", intent.DeliveryLeaseID)
	if err == nil {
		return bound, nil
	}
	if errors.Is(err, errProviderOrderConflicted) {
		return h.failConflictedPaymentOrderIntent(ctx, intent, "external_order_conflict")
	}
	// The Provider has already created (or re-returned) this merchant order.
	// Do not fail it locally: a retry or a verified callback can attach it.
	return h.recordRecoverableDeliveryFailure(ctx, intent, "provider_bind_failed")
}

// claimPaymentOrderDelivery commits the dispatch lease before any Provider
// interaction. A callback is allowed to recover an unexpired lease because it
// is already backed by a verified Provider query.
func (h *service) claimPaymentOrderDelivery(ctx context.Context, providerName, orderID string) (paymentOrderIntentRecord, bool, error) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return paymentOrderIntentRecord{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, orderID)
	if err != nil {
		return paymentOrderIntentRecord{}, false, err
	}
	if locked.Order.Provider != providerName {
		return paymentOrderIntentRecord{}, false, errors.New("payment order intent does not match its provider")
	}
	if locked.Order.ProviderOrderID != "" || locked.Order.View.Status != string(MembershipOrderCreated) || locked.DeliveryState == "delivered" || locked.DeliveryState == "failed" {
		if err := tx.Commit(ctx); err != nil {
			return paymentOrderIntentRecord{}, false, err
		}
		return locked, false, nil
	}
	now := h.now().UTC()
	if locked.DeliveryState == "dispatching" && locked.DeliveryLeaseExpiresAt != nil && locked.DeliveryLeaseExpiresAt.After(now) {
		if err := tx.Commit(ctx); err != nil {
			return paymentOrderIntentRecord{}, false, err
		}
		return locked, false, nil
	}
	if locked.DeliveryState != "pending" && locked.DeliveryState != "dispatching" {
		return paymentOrderIntentRecord{}, false, errors.New("payment order intent is not dispatchable")
	}
	leaseID := uuid.NewString()
	leaseExpiresAt := now.Add(paymentOrderDeliveryLease)
	if _, err := tx.Exec(ctx, `
		UPDATE account_portfolio_payment_order_intents
		SET delivery_state='dispatching', delivery_attempts=delivery_attempts+1,
			delivery_lease_id=$2, delivery_lease_expires_at=$3,
			last_attempt_at=$4, last_error_code=NULL, updated_at=$4
		WHERE order_id=$1
	`, locked.Order.View.ID, leaseID, leaseExpiresAt, now); err != nil {
		return paymentOrderIntentRecord{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return paymentOrderIntentRecord{}, false, err
	}
	locked.DeliveryState = "dispatching"
	locked.DeliveryLeaseID = leaseID
	locked.DeliveryLeaseExpiresAt = &leaseExpiresAt
	return locked, true, nil
}

func (h *service) recordRecoverableDeliveryFailure(ctx context.Context, intent paymentOrderIntentRecord, reason string) (membershipOrderView, *commandFailure) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment audit is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, intent.Order.View.ID)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	if locked.Order.ProviderOrderID != "" || locked.DeliveryState != "dispatching" || locked.DeliveryLeaseID != intent.DeliveryLeaseID {
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
		}
		return locked.Order.View, nil
	}
	now := h.now().UTC()
	if _, err := tx.Exec(ctx, `
		UPDATE account_portfolio_payment_order_intents
		SET delivery_state='pending', delivery_lease_id=NULL, delivery_lease_expires_at=NULL,
			last_error_code=$2, updated_at=$3
		WHERE order_id=$1 AND delivery_state='dispatching' AND delivery_lease_id=$4
	`, locked.Order.View.ID, reason, now, intent.DeliveryLeaseID); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	if failure := h.recordPaymentAudit(ctx, tx, &locked.Order.View.ID, nil, locked.Order.Provider, "order_delivery_failed", reason, paymentOrderDeliveryDigest(locked.MerchantOrderID), now); failure != nil {
		return membershipOrderView{}, failure
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment audit is unavailable")
	}
	return locked.Order.View, nil
}

func (h *service) failConflictedPaymentOrderIntent(ctx context.Context, intent paymentOrderIntentRecord, reason string) (membershipOrderView, *commandFailure) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, intent.Order.View.ID)
	if err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	if locked.Order.ProviderOrderID != "" || locked.DeliveryState != "dispatching" || locked.DeliveryLeaseID != intent.DeliveryLeaseID || locked.Order.View.Status != string(MembershipOrderCreated) {
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
		}
		return locked.Order.View, nil
	}
	now := h.now().UTC()
	var order membershipOrderView
	if err := tx.QueryRow(ctx, `
		UPDATE account_portfolio_membership_orders
		SET status='failed', version=version+1, updated_at=$2
		WHERE id=$1 AND status='created'
		RETURNING id, plan, amount_cents, status, version, created_at, updated_at
	`, intent.Order.View.ID, now).Scan(&order.ID, &order.Plan, &order.AmountCents, &order.Status, &order.Version, &order.CreatedAt, &order.UpdatedAt); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio membership order is unavailable")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE account_portfolio_payment_order_intents
		SET delivery_state='failed', delivery_lease_id=NULL, delivery_lease_expires_at=NULL,
			last_error_code=$2, updated_at=$3
		WHERE order_id=$1 AND delivery_state='dispatching' AND delivery_lease_id=$4
	`, order.ID, reason, now, intent.DeliveryLeaseID); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	if failure := h.recordPaymentAudit(ctx, tx, &order.ID, nil, locked.Order.Provider, "order_delivery_conflicted", reason, paymentOrderDeliveryDigest(locked.MerchantOrderID), now); failure != nil {
		return membershipOrderView{}, failure
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderView{}, dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	return order, nil
}

func (h *service) bindProviderOrder(ctx context.Context, intent paymentOrderIntentRecord, providerOrder ProviderOrder, auditOutcome, deliveryLeaseID string) (membershipOrderView, error) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderView{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	locked, err := paymentOrderIntentByOrderIDForUpdate(ctx, tx, intent.Order.View.ID)
	if err != nil {
		return membershipOrderView{}, err
	}
	if locked.Order.Provider != intent.Order.Provider || locked.MerchantOrderID != intent.MerchantOrderID {
		return membershipOrderView{}, errors.New("payment order intent does not match its provider")
	}
	if locked.Order.ProviderOrderID != "" {
		if locked.Order.ProviderOrderID != providerOrder.ExternalOrderID {
			return membershipOrderView{}, errProviderOrderConflicted
		}
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, err
		}
		return locked.Order.View, nil
	}
	if locked.Order.View.Status != string(MembershipOrderCreated) {
		return membershipOrderView{}, errors.New("payment order intent is not bindable")
	}
	if deliveryLeaseID == "" {
		if locked.DeliveryState != "pending" && locked.DeliveryState != "dispatching" {
			return membershipOrderView{}, errors.New("payment order intent is not recoverable")
		}
	} else if locked.DeliveryState != "dispatching" || locked.DeliveryLeaseID != deliveryLeaseID {
		return membershipOrderView{}, errors.New("payment order dispatch lease was lost")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, locked.Order.Provider+":"+providerOrder.ExternalOrderID); err != nil {
		return membershipOrderView{}, err
	}
	var existingID string
	err = tx.QueryRow(ctx, `SELECT id FROM account_portfolio_membership_orders WHERE provider_order_id=$1`, providerOrder.ExternalOrderID).Scan(&existingID)
	if err == nil && existingID != locked.Order.View.ID {
		return membershipOrderView{}, errProviderOrderConflicted
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return membershipOrderView{}, err
	}
	now := h.now().UTC()
	var bound membershipOrderView
	if err := tx.QueryRow(ctx, `
		UPDATE account_portfolio_membership_orders
		SET provider_order_id=$2, status='pending_payment', version=version+1, updated_at=$3
		WHERE id=$1 AND status='created'
		RETURNING id, plan, amount_cents, status, version, created_at, updated_at
	`, locked.Order.View.ID, providerOrder.ExternalOrderID, now).Scan(&bound.ID, &bound.Plan, &bound.AmountCents, &bound.Status, &bound.Version, &bound.CreatedAt, &bound.UpdatedAt); err != nil {
		return membershipOrderView{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE account_portfolio_payment_order_intents
		SET delivery_state='delivered', delivery_lease_id=NULL, delivery_lease_expires_at=NULL,
			last_error_code=NULL, updated_at=$2
		WHERE order_id=$1
	`, bound.ID, now); err != nil {
		return membershipOrderView{}, err
	}
	if failure := h.recordPaymentAudit(ctx, tx, &bound.ID, nil, locked.Order.Provider, auditOutcome, "provider_order_bound", paymentOrderDeliveryDigest(locked.MerchantOrderID), now); failure != nil {
		return membershipOrderView{}, failure
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderView{}, err
	}
	return bound, nil
}

func (h *service) recoverProviderOrderFromNotification(ctx context.Context, providerName string, notification VerifiedPaymentNotification, providerOrder ProviderOrder, _ [sha256.Size]byte) *commandFailure {
	if !validProviderOrderCorrelation(notification, providerOrder) {
		return invalidCommand("payment notification is invalid")
	}
	if _, err := membershipOrderByExternalID(ctx, h.database, providerName, notification.ExternalOrderID); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return dependencyFailure("Account Portfolio payment order is unavailable")
	}
	intent, err := h.paymentOrderIntentByMerchantID(ctx, providerName, notification.MerchantOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		// applyVerifiedPaymentNotification records the durable unknown-order audit.
		return nil
	}
	if err != nil {
		return dependencyFailure("Account Portfolio payment intent is unavailable")
	}
	if _, err := h.bindProviderOrder(ctx, intent, providerOrder, "order_recovered", ""); err != nil {
		if errors.Is(err, errProviderOrderConflicted) {
			return invalidCommand("payment notification is invalid")
		}
		return dependencyFailure("Account Portfolio payment recovery is unavailable")
	}
	return nil
}

func validCreatedProviderOrder(intent paymentOrderIntentRecord, order ProviderOrder) bool {
	return len(strings.TrimSpace(order.ExternalOrderID)) > 0 && len(order.ExternalOrderID) <= 200 && validProviderOrderCorrelation(VerifiedPaymentNotification{
		ExternalOrderID: order.ExternalOrderID,
		MerchantOrderID: order.MerchantOrderID,
		AmountCents:     order.AmountCents,
		Currency:        order.Currency,
		Plan:            order.Plan,
		Status:          order.Status,
	}, order) && order.MerchantOrderID == intent.MerchantOrderID && order.Status == MembershipOrderPendingPayment
}

func validProviderOrderCorrelation(notification VerifiedPaymentNotification, order ProviderOrder) bool {
	return order.ExternalOrderID != "" && order.ExternalOrderID == notification.ExternalOrderID &&
		order.MerchantOrderID == notification.MerchantOrderID &&
		paymentOrderTermsMatch(notification, order) &&
		notification.AmountCents == lifetimeMembershipAmountCents &&
		notification.Currency == lifetimeMembershipCurrency &&
		notification.Plan == lifetimeMembershipPlan &&
		order.Status == notification.Status && validProviderNotificationStatus(order.Status)
}

func paymentOrderTermsMatch(notification VerifiedPaymentNotification, order ProviderOrder) bool {
	return order.AmountCents == notification.AmountCents &&
		order.Currency == notification.Currency &&
		order.Plan == notification.Plan
}

func paymentOrderDeliveryDigest(merchantOrderID string) [sha256.Size]byte {
	return paymentDigest([]byte("payment-order-intent:" + merchantOrderID))
}

func (h *service) applyVerifiedPaymentNotification(ctx context.Context, providerName string, notification VerifiedPaymentNotification, digest [sha256.Size]byte) (membershipOrderView, string, *commandFailure) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	intent, err := paymentOrderIntentByExternalIDForUpdate(ctx, tx, providerName, notification.ExternalOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		if failure := h.recordPaymentAudit(ctx, tx, nil, nil, providerName, "notification_unknown_order", "merchant_order_not_found", digest, h.now().UTC()); failure != nil {
			return membershipOrderView{}, "", failure
		}
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment audit is unavailable")
		}
		return membershipOrderView{}, "", invalidCommand("payment notification is invalid")
	}
	if err != nil {
		return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment order is unavailable")
	}
	record := intent.Order
	if intent.Order.Provider != providerName || intent.MerchantOrderID != notification.MerchantOrderID {
		if failure := h.recordPaymentAudit(ctx, tx, &record.View.ID, nil, providerName, "notification_rejected", "merchant_order_mismatch", digest, h.now().UTC()); failure != nil {
			return membershipOrderView{}, "", failure
		}
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment audit is unavailable")
		}
		return membershipOrderView{}, "", invalidCommand("payment notification is invalid")
	}

	now := h.now().UTC()
	factID, inserted, err := h.insertPaymentFact(ctx, tx, record, providerName, notification, digest, now)
	if err != nil {
		if errors.Is(err, errProviderOrderConflicted) {
			if failure := h.recordPaymentAudit(ctx, tx, &record.View.ID, nil, providerName, "notification_rejected", "provider_event_order_conflict", digest, now); failure != nil {
				return membershipOrderView{}, "", failure
			}
			if err := tx.Commit(ctx); err != nil {
				return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment audit is unavailable")
			}
			return membershipOrderView{}, "", invalidCommand("payment notification is invalid")
		}
		return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment fact is unavailable")
	}
	if !inserted {
		var existingDigest []byte
		if err := tx.QueryRow(ctx, `SELECT payload_sha256 FROM account_portfolio_payment_facts WHERE provider=$1 AND provider_event_id=$2`, providerName, notification.EventID).Scan(&existingDigest); err != nil {
			return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment fact is unavailable")
		}
		if !bytes.Equal(existingDigest, digest[:]) {
			if failure := h.recordPaymentAudit(ctx, tx, &record.View.ID, nil, providerName, "notification_rejected", "provider_event_reused", digest, now); failure != nil {
				return membershipOrderView{}, "", failure
			}
			if err := tx.Commit(ctx); err != nil {
				return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment audit is unavailable")
			}
			return membershipOrderView{}, "", invalidCommand("payment notification is invalid")
		}
		if failure := h.recordPaymentAudit(ctx, tx, &record.View.ID, &factID, providerName, "notification_replayed", "provider_event_replayed", digest, now); failure != nil {
			return membershipOrderView{}, "", failure
		}
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment audit is unavailable")
		}
		return record.View, "replayed", nil
	}

	if notification.Sequence <= record.ProviderEventSequence {
		if failure := h.recordPaymentAudit(ctx, tx, &record.View.ID, &factID, providerName, "notification_out_of_order", "provider_sequence_stale", digest, now); failure != nil {
			return membershipOrderView{}, "", failure
		}
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment audit is unavailable")
		}
		return record.View, "ignored", nil
	}
	if !membershipOrderTransitionAllowed(MembershipOrderStatus(record.View.Status), notification.Status) {
		if failure := h.recordPaymentAudit(ctx, tx, &record.View.ID, &factID, providerName, "notification_invalid_transition", "provider_transition_invalid", digest, now); failure != nil {
			return membershipOrderView{}, "", failure
		}
		if err := tx.Commit(ctx); err != nil {
			return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment audit is unavailable")
		}
		return record.View, "ignored", nil
	}

	updated, err := h.transitionMembershipOrder(ctx, tx, record.View.ID, notification.Status, notification.Sequence, now)
	if err != nil {
		return membershipOrderView{}, "", dependencyFailure("Account Portfolio membership order is unavailable")
	}
	if notification.Status == MembershipOrderPaid {
		if failure := h.grantPaymentMembership(ctx, tx, record.View.ID, record.UserID, factID, now); failure != nil {
			return membershipOrderView{}, "", failure
		}
	}
	if notification.Status == MembershipOrderRefunded {
		if failure := h.revokePaymentMembership(ctx, tx, record.View.ID, record.UserID, factID, now); failure != nil {
			return membershipOrderView{}, "", failure
		}
	}
	if failure := h.recordPaymentAudit(ctx, tx, &updated.ID, &factID, providerName, "notification_applied", "provider_notification_applied", digest, now); failure != nil {
		return membershipOrderView{}, "", failure
	}
	if err := tx.Commit(ctx); err != nil {
		return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment store is unavailable")
	}
	return updated, "applied", nil
}

type paymentRowQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func membershipOrderByUserIdempotency(ctx context.Context, query paymentRowQueryer, userID, key string) (membershipOrderRecord, error) {
	var record membershipOrderRecord
	err := query.QueryRow(ctx, `
		SELECT id, user_id, plan, amount_cents, status, version, created_at, updated_at,
			provider, COALESCE(provider_order_id, ''), provider_event_sequence
		FROM account_portfolio_membership_orders
		WHERE user_id=$1 AND idempotency_key=$2
	`, userID, key).Scan(
		&record.View.ID, &record.UserID, &record.View.Plan, &record.View.AmountCents, &record.View.Status,
		&record.View.Version, &record.View.CreatedAt, &record.View.UpdatedAt, &record.Provider,
		&record.ProviderOrderID, &record.ProviderEventSequence,
	)
	return record, err
}

// paymentOrderIntentByOrderIDForUpdate obtains the intent and local order in
// one short local transaction. Callers release that transaction before any
// Provider interaction.
func paymentOrderIntentByOrderIDForUpdate(ctx context.Context, tx pgx.Tx, orderID string) (paymentOrderIntentRecord, error) {
	var intent paymentOrderIntentRecord
	err := tx.QueryRow(ctx, `
		SELECT o.id, o.user_id, o.plan, o.amount_cents, o.status, o.version, o.created_at, o.updated_at,
			o.provider, COALESCE(o.provider_order_id, ''), o.provider_event_sequence,
			i.merchant_order_id::text, i.delivery_state,
			COALESCE(i.delivery_lease_id::text, ''), i.delivery_lease_expires_at
		FROM account_portfolio_payment_order_intents i
		JOIN account_portfolio_membership_orders o ON o.id=i.order_id
		WHERE i.order_id=$1
		FOR UPDATE OF i, o
	`, orderID).Scan(
		&intent.Order.View.ID, &intent.Order.UserID, &intent.Order.View.Plan, &intent.Order.View.AmountCents, &intent.Order.View.Status,
		&intent.Order.View.Version, &intent.Order.View.CreatedAt, &intent.Order.View.UpdatedAt, &intent.Order.Provider,
		&intent.Order.ProviderOrderID, &intent.Order.ProviderEventSequence,
		&intent.MerchantOrderID, &intent.DeliveryState, &intent.DeliveryLeaseID, &intent.DeliveryLeaseExpiresAt,
	)
	return intent, err
}

// paymentOrderIntentByExternalIDForUpdate locks the same intent/order pair in
// the same relation order as bindProviderOrder. Callback application therefore
// validates the private merchant correlation without inverting bind's locks.
func paymentOrderIntentByExternalIDForUpdate(ctx context.Context, tx pgx.Tx, providerName, externalOrderID string) (paymentOrderIntentRecord, error) {
	var intent paymentOrderIntentRecord
	err := tx.QueryRow(ctx, `
		SELECT o.id, o.user_id, o.plan, o.amount_cents, o.status, o.version, o.created_at, o.updated_at,
			o.provider, COALESCE(o.provider_order_id, ''), o.provider_event_sequence,
			i.merchant_order_id::text, i.delivery_state,
			COALESCE(i.delivery_lease_id::text, ''), i.delivery_lease_expires_at
		FROM account_portfolio_payment_order_intents i
		JOIN account_portfolio_membership_orders o ON o.id=i.order_id
		WHERE i.provider=$1 AND o.provider=$1 AND o.provider_order_id=$2
		FOR UPDATE OF i, o
	`, providerName, externalOrderID).Scan(
		&intent.Order.View.ID, &intent.Order.UserID, &intent.Order.View.Plan, &intent.Order.View.AmountCents, &intent.Order.View.Status,
		&intent.Order.View.Version, &intent.Order.View.CreatedAt, &intent.Order.View.UpdatedAt, &intent.Order.Provider,
		&intent.Order.ProviderOrderID, &intent.Order.ProviderEventSequence,
		&intent.MerchantOrderID, &intent.DeliveryState, &intent.DeliveryLeaseID, &intent.DeliveryLeaseExpiresAt,
	)
	return intent, err
}

func (h *service) paymentOrderIntentByMerchantID(ctx context.Context, providerName, merchantOrderID string) (paymentOrderIntentRecord, error) {
	var intent paymentOrderIntentRecord
	err := h.database.QueryRow(ctx, `
		SELECT o.id, o.user_id, o.plan, o.amount_cents, o.status, o.version, o.created_at, o.updated_at,
			o.provider, COALESCE(o.provider_order_id, ''), o.provider_event_sequence,
			i.merchant_order_id::text, i.delivery_state,
			COALESCE(i.delivery_lease_id::text, ''), i.delivery_lease_expires_at
		FROM account_portfolio_payment_order_intents i
		JOIN account_portfolio_membership_orders o ON o.id=i.order_id
		WHERE i.provider=$1 AND i.merchant_order_id=$2
	`, providerName, merchantOrderID).Scan(
		&intent.Order.View.ID, &intent.Order.UserID, &intent.Order.View.Plan, &intent.Order.View.AmountCents, &intent.Order.View.Status,
		&intent.Order.View.Version, &intent.Order.View.CreatedAt, &intent.Order.View.UpdatedAt, &intent.Order.Provider,
		&intent.Order.ProviderOrderID, &intent.Order.ProviderEventSequence,
		&intent.MerchantOrderID, &intent.DeliveryState, &intent.DeliveryLeaseID, &intent.DeliveryLeaseExpiresAt,
	)
	return intent, err
}

func membershipOrderByExternalID(ctx context.Context, query paymentRowQueryer, providerName, externalOrderID string) (membershipOrderRecord, error) {
	var record membershipOrderRecord
	err := query.QueryRow(ctx, `
		SELECT id, user_id, plan, amount_cents, status, version, created_at, updated_at,
			provider, provider_order_id, provider_event_sequence
		FROM account_portfolio_membership_orders
		WHERE provider=$1 AND provider_order_id=$2
	`, providerName, externalOrderID).Scan(
		&record.View.ID, &record.UserID, &record.View.Plan, &record.View.AmountCents, &record.View.Status,
		&record.View.Version, &record.View.CreatedAt, &record.View.UpdatedAt, &record.Provider,
		&record.ProviderOrderID, &record.ProviderEventSequence,
	)
	return record, err
}

func (h *service) insertPaymentFact(ctx context.Context, tx pgx.Tx, record membershipOrderRecord, providerName string, notification VerifiedPaymentNotification, digest [sha256.Size]byte, now time.Time) (string, bool, error) {
	factID := uuid.NewString()
	inserted, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_payment_facts(
			id, order_id, provider, provider_event_id, external_order_id, status,
			provider_sequence, occurred_at, payload_sha256, created_at
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
	`, factID, record.View.ID, providerName, notification.EventID, notification.ExternalOrderID,
		string(notification.Status), notification.Sequence, notification.OccurredAt.UTC(), digest[:], now)
	if err != nil {
		return "", false, err
	}
	if inserted.RowsAffected() == 1 {
		return factID, true, nil
	}
	var existingOrderID string
	if err := tx.QueryRow(ctx, `
		SELECT id, order_id
		FROM account_portfolio_payment_facts
		WHERE provider=$1 AND provider_event_id=$2
		FOR KEY SHARE
	`, providerName, notification.EventID).Scan(&factID, &existingOrderID); err != nil {
		return "", false, err
	}
	if existingOrderID != record.View.ID {
		return "", false, errProviderOrderConflicted
	}
	return factID, false, nil
}

func (h *service) transitionMembershipOrder(ctx context.Context, tx pgx.Tx, orderID string, status MembershipOrderStatus, providerSequence int64, now time.Time) (membershipOrderView, error) {
	var order membershipOrderView
	err := tx.QueryRow(ctx, `
		UPDATE account_portfolio_membership_orders
		SET status=$2, provider_event_sequence=$3, version=version+1, updated_at=$4
		WHERE id=$1
		RETURNING id, plan, amount_cents, status, version, created_at, updated_at
	`, orderID, string(status), providerSequence, now).Scan(&order.ID, &order.Plan, &order.AmountCents, &order.Status, &order.Version, &order.CreatedAt, &order.UpdatedAt)
	return order, err
}

func (h *service) grantPaymentMembership(ctx context.Context, tx pgx.Tx, orderID, userID, paymentFactID string, now time.Time) *commandFailure {
	var factOrderID string
	if err := tx.QueryRow(ctx, `
		SELECT order_id
		FROM account_portfolio_payment_facts
		WHERE id=$1 AND order_id=$2 AND status='paid'
		FOR KEY SHARE
	`, paymentFactID, orderID).Scan(&factOrderID); err != nil || factOrderID != orderID {
		return dependencyFailure("Account Portfolio verified payment fact is unavailable")
	}
	var plan, source string
	var currentPaymentFactID *string
	var version int
	if err := tx.QueryRow(ctx, `
		SELECT plan, source, payment_fact_id::text, version
		FROM account_portfolio_memberships
		WHERE user_id=$1
		FOR UPDATE
	`, userID).Scan(&plan, &source, &currentPaymentFactID, &version); err != nil {
		return dependencyFailure("Account Portfolio membership is unavailable")
	}
	if plan == "free" {
		if err := tx.QueryRow(ctx, `
			UPDATE account_portfolio_memberships
			SET plan='lifetime', source='payment', payment_fact_id=$2, granted_at=$3, version=version+1, updated_at=$3
			WHERE user_id=$1 AND version=$4
			RETURNING version
		`, userID, paymentFactID, now, version).Scan(&version); err != nil {
			return dependencyFailure("Account Portfolio membership is unavailable")
		}
		eventID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO account_portfolio_membership_events(
				id, user_id, kind, from_plan, to_plan, source, actor_user_id, reason,
				idempotency_key, payment_fact_id, created_at
			)
			VALUES($1, $2, 'grant', 'free', 'lifetime', 'payment', NULL, $3, $4, $5, $6)
		`, eventID, userID, "Verified payment confirmation.", "payment:"+paymentFactID, paymentFactID, now); err != nil {
			return dependencyFailure("Account Portfolio membership audit is unavailable")
		}
		return h.createMembershipNotification(ctx, tx, eventID, userID, "membership_lifetime_granted", "终身会员权益已发放", "已确认支付成功，终身会员权益已发放。", now)
	}
	if source != "payment" || currentPaymentFactID == nil || *currentPaymentFactID == paymentFactID {
		return nil
	}
	// A later verified lifetime payment becomes the current ownership fact. It
	// does not emit a second entitlement grant, but protects the older payment's
	// refund from revoking this still-valid entitlement.
	if err := tx.QueryRow(ctx, `
		UPDATE account_portfolio_memberships
		SET payment_fact_id=$2, version=version+1, updated_at=$3
		WHERE user_id=$1 AND version=$4 AND source='payment'
		RETURNING version
	`, userID, paymentFactID, now, version).Scan(&version); err != nil {
		return dependencyFailure("Account Portfolio membership is unavailable")
	}
	return nil
}

func (h *service) revokePaymentMembership(ctx context.Context, tx pgx.Tx, orderID, userID, refundFactID string, now time.Time) *commandFailure {
	var refundFactOrderID string
	if err := tx.QueryRow(ctx, `
		SELECT order_id
		FROM account_portfolio_payment_facts
		WHERE id=$1 AND order_id=$2 AND status='refunded'
		FOR KEY SHARE
	`, refundFactID, orderID).Scan(&refundFactOrderID); err != nil || refundFactOrderID != orderID {
		return dependencyFailure("Account Portfolio verified refund fact is unavailable")
	}
	var plan, source string
	var currentPaymentFactID *string
	var version int
	if err := tx.QueryRow(ctx, `
		SELECT plan, source, payment_fact_id::text, version
		FROM account_portfolio_memberships
		WHERE user_id=$1
		FOR UPDATE
	`, userID).Scan(&plan, &source, &currentPaymentFactID, &version); err != nil {
		return dependencyFailure("Account Portfolio membership is unavailable")
	}
	if plan != "lifetime" || source != "payment" || currentPaymentFactID == nil {
		return nil
	}
	var currentFactOrderID string
	err := tx.QueryRow(ctx, `
		SELECT order_id
		FROM account_portfolio_payment_facts
		WHERE id=$1 AND status='paid'
		FOR KEY SHARE
	`, *currentPaymentFactID).Scan(&currentFactOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return dependencyFailure("Account Portfolio payment fact is unavailable")
	}
	// Reverse-check the exact ownership fact. A refund for an older order must
	// not downgrade a membership currently backed by a newer valid payment.
	if currentFactOrderID != orderID {
		return nil
	}

	var replacementFactID string
	err = tx.QueryRow(ctx, `
		SELECT f.id
		FROM account_portfolio_payment_facts f
		JOIN account_portfolio_membership_orders o ON o.id=f.order_id
		WHERE o.user_id=$1 AND o.id<>$2 AND o.status='paid' AND f.status='paid'
		ORDER BY f.occurred_at DESC, f.created_at DESC, f.id DESC
		LIMIT 1
		FOR KEY SHARE OF f
	`, userID, orderID).Scan(&replacementFactID)
	if err == nil {
		if err := tx.QueryRow(ctx, `
			UPDATE account_portfolio_memberships
			SET payment_fact_id=$2, version=version+1, updated_at=$3
			WHERE user_id=$1 AND version=$4 AND source='payment'
			RETURNING version
		`, userID, replacementFactID, now, version).Scan(&version); err != nil {
			return dependencyFailure("Account Portfolio membership is unavailable")
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dependencyFailure("Account Portfolio payment fact is unavailable")
	}
	if err := tx.QueryRow(ctx, `
		UPDATE account_portfolio_memberships
		SET plan='free', source='payment_refund', payment_fact_id=NULL, version=version+1, updated_at=$2
		WHERE user_id=$1 AND version=$3
		RETURNING version
	`, userID, now, version).Scan(&version); err != nil {
		return dependencyFailure("Account Portfolio membership is unavailable")
	}
	eventID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_membership_events(
			id, user_id, kind, from_plan, to_plan, source, actor_user_id, reason,
			idempotency_key, payment_fact_id, created_at
		)
		VALUES($1, $2, 'revoke', 'lifetime', 'free', 'payment', NULL, $3, $4, $5, $6)
	`, eventID, userID, "Verified payment refund confirmation.", "payment:"+refundFactID, refundFactID, now); err != nil {
		return dependencyFailure("Account Portfolio membership audit is unavailable")
	}
	return h.createMembershipNotification(ctx, tx, eventID, userID, "membership_lifetime_revoked", "终身会员权益已撤销", "已确认退款，终身会员权益已撤销。", now)
}

func (h *service) auditPaymentNotification(ctx context.Context, providerName string, orderID, paymentFactID *string, outcome, reason string, digest [sha256.Size]byte) error {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if failure := h.recordPaymentAudit(ctx, tx, orderID, paymentFactID, providerName, outcome, reason, digest, h.now().UTC()); failure != nil {
		return failure
	}
	return tx.Commit(ctx)
}

func (h *service) recordPaymentAudit(ctx context.Context, tx pgx.Tx, orderID, paymentFactID *string, providerName, outcome, reason string, digest [sha256.Size]byte, now time.Time) *commandFailure {
	if _, err := tx.Exec(ctx, `
		INSERT INTO account_portfolio_payment_audits(
			id, order_id, payment_fact_id, provider, outcome, reason_code, payload_sha256, created_at
		)
		VALUES($1, $2, $3, $4, $5, $6, $7, $8)
	`, uuid.NewString(), orderID, paymentFactID, providerName, outcome, reason, digest[:], now); err != nil {
		return dependencyFailure("Account Portfolio payment audit is unavailable")
	}
	return nil
}

func readPaymentNotification(r *http.Request) ([]byte, *commandFailure) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
	if err != nil || len(raw) == 0 || len(raw) > 64<<10 {
		return nil, invalidCommand("payment notification is invalid")
	}
	return raw, nil
}

func validVerifiedPaymentNotification(notification VerifiedPaymentNotification) bool {
	return len(strings.TrimSpace(notification.EventID)) <= 200 && len(strings.TrimSpace(notification.EventID)) > 0 &&
		len(strings.TrimSpace(notification.ExternalOrderID)) <= 200 && len(strings.TrimSpace(notification.ExternalOrderID)) > 0 &&
		validMembershipMerchantOrderID(notification.MerchantOrderID) &&
		notification.AmountCents == lifetimeMembershipAmountCents &&
		notification.Currency == lifetimeMembershipCurrency &&
		notification.Plan == lifetimeMembershipPlan &&
		validProviderNotificationStatus(notification.Status) &&
		notification.Sequence > 0 &&
		!notification.OccurredAt.IsZero()
}

func paymentDigest(raw []byte) [sha256.Size]byte { return sha256.Sum256(raw) }
