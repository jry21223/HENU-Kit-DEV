package accountportfolio

import (
	"bytes"
	"context"
	"crypto/sha256"
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

	payload, status, failure := h.executeCommand(r.Context(), authenticatedActor(r).clientID, userID, "membership_order_create", r.URL.Path, key, raw, http.StatusCreated, func(tx pgx.Tx) (any, *commandFailure) {
		now := h.now().UTC()
		order := membershipOrderView{
			ID:          uuid.NewString(),
			Plan:        "lifetime",
			AmountCents: lifetimeMembershipAmountCents,
			Status:      string(MembershipOrderCreated),
			Version:     1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO account_portfolio_membership_orders(
				id, user_id, plan, amount_cents, status, provider, idempotency_key,
				version, provider_event_sequence, created_at, updated_at
			)
			VALUES($1, $2, 'lifetime', $3, 'created', $4, $5, 1, 0, $6, $6)
		`, order.ID, userID, lifetimeMembershipAmountCents, providerName, key, now); err != nil {
			return nil, dependencyFailure("Account Portfolio membership order is unavailable")
		}

		signed, err := provider.Sign(r.Context(), PaymentOrderRequest{
			MerchantOrderID: order.ID,
			AmountCents:     lifetimeMembershipAmountCents,
			Product:         "lifetime",
		})
		if err != nil {
			return h.failMembershipOrder(r.Context(), tx, order.ID, providerName, raw, now, "provider_sign_failed")
		}
		providerOrder, err := provider.CreateOrder(r.Context(), signed)
		if err != nil {
			return h.failMembershipOrder(r.Context(), tx, order.ID, providerName, raw, now, "provider_create_failed")
		}
		if providerOrder.MerchantOrderID != order.ID || providerOrder.AmountCents != lifetimeMembershipAmountCents || providerOrder.ExternalOrderID == "" || providerOrder.Status != MembershipOrderPendingPayment {
			return h.failMembershipOrder(r.Context(), tx, order.ID, providerName, raw, now, "provider_response_invalid")
		}
		pending, claimed, err := h.moveMembershipOrderToPendingPayment(r.Context(), tx, order.ID, providerName, providerOrder.ExternalOrderID, now)
		if err != nil {
			return nil, dependencyFailure("Account Portfolio membership order is unavailable")
		}
		if !claimed {
			return h.failMembershipOrder(r.Context(), tx, order.ID, providerName, raw, now, "external_order_conflict")
		}
		if failure := h.recordPaymentAudit(r.Context(), tx, &pending.ID, nil, providerName, "order_created", "provider_order_created", paymentDigest(raw), now); failure != nil {
			return nil, failure
		}
		return map[string]any{"order": pending}, nil
	})
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, status, payload)
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
	if providerOrder.ExternalOrderID != notification.ExternalOrderID || providerOrder.MerchantOrderID != notification.MerchantOrderID || providerOrder.AmountCents != lifetimeMembershipAmountCents || !validProviderNotificationStatus(providerOrder.Status) {
		if auditErr := h.auditPaymentNotification(r.Context(), providerName, nil, nil, "notification_rejected", "provider_query_mismatch", digest); auditErr != nil {
			writeError(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "Account Portfolio payment audit is unavailable")
			return
		}
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "payment notification is invalid")
		return
	}

	order, outcome, failure := h.applyVerifiedPaymentNotification(r.Context(), providerName, notification, digest)
	if failure != nil {
		writeCommandFailure(w, r, failure)
		return
	}
	writeData(w, r, http.StatusOK, map[string]any{"accepted": true, "outcome": outcome, "order": order})
}

func (h *service) failMembershipOrder(ctx context.Context, tx pgx.Tx, orderID, providerName string, raw []byte, now time.Time, reason string) (any, *commandFailure) {
	var order membershipOrderView
	err := tx.QueryRow(ctx, `
		UPDATE account_portfolio_membership_orders
		SET status='failed', version=version+1, updated_at=$2
		WHERE id=$1 AND status='created'
		RETURNING id, plan, amount_cents, status, version, created_at, updated_at
	`, orderID, now).Scan(&order.ID, &order.Plan, &order.AmountCents, &order.Status, &order.Version, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return nil, dependencyFailure("Account Portfolio membership order is unavailable")
	}
	if failure := h.recordPaymentAudit(ctx, tx, &order.ID, nil, providerName, "order_creation_failed", reason, paymentDigest(raw), now); failure != nil {
		return nil, failure
	}
	return map[string]any{"order": order}, nil
}

func (h *service) moveMembershipOrderToPendingPayment(ctx context.Context, tx pgx.Tx, orderID, providerName, externalOrderID string, now time.Time) (membershipOrderView, bool, error) {
	lockName := providerName + ":" + externalOrderID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockName); err != nil {
		return membershipOrderView{}, false, err
	}
	var existingID string
	err := tx.QueryRow(ctx, `SELECT id FROM account_portfolio_membership_orders WHERE provider_order_id=$1`, externalOrderID).Scan(&existingID)
	if err == nil && existingID != orderID {
		return membershipOrderView{}, false, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return membershipOrderView{}, false, err
	}
	var order membershipOrderView
	err = tx.QueryRow(ctx, `
		UPDATE account_portfolio_membership_orders
		SET provider_order_id=$2, status='pending_payment', version=version+1, updated_at=$3
		WHERE id=$1 AND status='created'
		RETURNING id, plan, amount_cents, status, version, created_at, updated_at
	`, orderID, externalOrderID, now).Scan(&order.ID, &order.Plan, &order.AmountCents, &order.Status, &order.Version, &order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		return membershipOrderView{}, false, err
	}
	return order, true, nil
}

func (h *service) applyVerifiedPaymentNotification(ctx context.Context, providerName string, notification VerifiedPaymentNotification, digest [sha256.Size]byte) (membershipOrderView, string, *commandFailure) {
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return membershipOrderView{}, "", dependencyFailure("Account Portfolio payment store is unavailable")
	}
	defer func() { _ = tx.Rollback(ctx) }()

	record, err := h.membershipOrderByExternalID(ctx, tx, providerName, notification.ExternalOrderID)
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
	if record.View.ID != notification.MerchantOrderID {
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
		if failure := h.grantPaymentMembership(ctx, tx, record.UserID, factID, now); failure != nil {
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

func (h *service) membershipOrderByExternalID(ctx context.Context, tx pgx.Tx, providerName, externalOrderID string) (membershipOrderRecord, error) {
	var record membershipOrderRecord
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, plan, amount_cents, status, version, created_at, updated_at,
			provider, provider_order_id, provider_event_sequence
		FROM account_portfolio_membership_orders
		WHERE provider=$1 AND provider_order_id=$2
		FOR UPDATE
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
	if err := tx.QueryRow(ctx, `SELECT id FROM account_portfolio_payment_facts WHERE provider=$1 AND provider_event_id=$2`, providerName, notification.EventID).Scan(&factID); err != nil {
		return "", false, err
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

func (h *service) grantPaymentMembership(ctx context.Context, tx pgx.Tx, userID, paymentFactID string, now time.Time) *commandFailure {
	var plan string
	var version int
	err := tx.QueryRow(ctx, `
		SELECT plan, version
		FROM account_portfolio_memberships
		WHERE user_id=$1
		FOR UPDATE
	`, userID).Scan(&plan, &version)
	if err != nil {
		return dependencyFailure("Account Portfolio membership is unavailable")
	}
	if plan != "free" {
		return nil
	}
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

func (h *service) revokePaymentMembership(ctx context.Context, tx pgx.Tx, orderID, userID, refundFactID string, now time.Time) *commandFailure {
	var paidFactID string
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM account_portfolio_payment_facts
		WHERE order_id=$1 AND status='paid'
		ORDER BY provider_sequence ASC, id ASC
		LIMIT 1
	`, orderID).Scan(&paidFactID)
	if err != nil {
		return dependencyFailure("Account Portfolio payment fact is unavailable")
	}
	var plan, source string
	var sourcePaymentFactID *string
	var version int
	err = tx.QueryRow(ctx, `
		SELECT plan, source, payment_fact_id::text, version
		FROM account_portfolio_memberships
		WHERE user_id=$1
		FOR UPDATE
	`, userID).Scan(&plan, &source, &sourcePaymentFactID, &version)
	if err != nil {
		return dependencyFailure("Account Portfolio membership is unavailable")
	}
	if plan != "lifetime" || source != "payment" || sourcePaymentFactID == nil || *sourcePaymentFactID != paidFactID {
		return nil
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
		uuid.Validate(notification.MerchantOrderID) == nil &&
		notification.AmountCents == lifetimeMembershipAmountCents &&
		validProviderNotificationStatus(notification.Status) &&
		notification.Sequence > 0 &&
		!notification.OccurredAt.IsZero()
}

func paymentDigest(raw []byte) [sha256.Size]byte { return sha256.Sum256(raw) }
