package accountportfolio

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const lifetimeMembershipAmountCents = 990

// MembershipOrderStatus is the durable lifecycle for the one ¥9.9 lifetime
// product. It intentionally does not model a provider-specific state.
type MembershipOrderStatus string

const (
	MembershipOrderCreated        MembershipOrderStatus = "created"
	MembershipOrderPendingPayment MembershipOrderStatus = "pending_payment"
	MembershipOrderPaid           MembershipOrderStatus = "paid"
	MembershipOrderClosed         MembershipOrderStatus = "closed"
	MembershipOrderFailed         MembershipOrderStatus = "failed"
	MembershipOrderRefunded       MembershipOrderStatus = "refunded"
)

// PaymentProvider isolates the provider-specific signing, ordering, querying,
// notification verification, and refund protocols from Account Portfolio's
// durable membership-order state machine. Production deliberately supplies no
// implementation until a separate payment-provider Spike is approved.
type PaymentProvider interface {
	Name() string
	Sign(context.Context, PaymentOrderRequest) (SignedPaymentOrder, error)
	CreateOrder(context.Context, SignedPaymentOrder) (ProviderOrder, error)
	QueryOrder(context.Context, string) (ProviderOrder, error)
	VerifyNotification(context.Context, []byte) (VerifiedPaymentNotification, error)
	Refund(context.Context, string) (PaymentRefund, error)
}

type PaymentOrderRequest struct {
	MerchantOrderID string
	AmountCents     int
	Product         string
}

type SignedPaymentOrder struct {
	Request   PaymentOrderRequest
	Signature string
}

type ProviderOrder struct {
	ExternalOrderID string
	MerchantOrderID string
	AmountCents     int
	Status          MembershipOrderStatus
}

type VerifiedPaymentNotification struct {
	EventID         string                `json:"event_id"`
	ExternalOrderID string                `json:"external_order_id"`
	MerchantOrderID string                `json:"merchant_order_id"`
	AmountCents     int                   `json:"amount_cents"`
	Status          MembershipOrderStatus `json:"status"`
	Sequence        int64                 `json:"sequence"`
	OccurredAt      time.Time             `json:"occurred_at"`
}

type PaymentRefund struct {
	Notification VerifiedPaymentNotification
}

func validProviderNotificationStatus(status MembershipOrderStatus) bool {
	switch status {
	case MembershipOrderPendingPayment, MembershipOrderPaid, MembershipOrderClosed, MembershipOrderFailed, MembershipOrderRefunded:
		return true
	default:
		return false
	}
}

// membershipOrderTransitionAllowed makes every terminal edge explicit. A
// repeated pending-payment notice is permitted only to record a newer provider
// sequence; it never creates an entitlement.
func membershipOrderTransitionAllowed(from, to MembershipOrderStatus) bool {
	if from == MembershipOrderPendingPayment && to == MembershipOrderPendingPayment {
		return true
	}
	switch from {
	case MembershipOrderCreated:
		return to == MembershipOrderPendingPayment || to == MembershipOrderFailed || to == MembershipOrderClosed
	case MembershipOrderPendingPayment:
		return to == MembershipOrderPaid || to == MembershipOrderClosed || to == MembershipOrderFailed
	case MembershipOrderPaid:
		return to == MembershipOrderRefunded
	default:
		return false
	}
}

func validPaymentProviderName(value string) bool {
	if len(value) < 1 || len(value) > 80 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9' && index > 0) || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return value[0] >= 'a' && value[0] <= 'z'
}

// FakePaymentProvider is deterministic test infrastructure. It is not wired
// from process configuration and carries no production credential or protocol.
type FakePaymentProvider struct {
	mu                     sync.Mutex
	secret                 []byte
	orders                 map[string]ProviderOrder
	merchantExternalOrders map[string]string
	nextExternalOrder      int
	nextEvent              int
	createCalls            int
	failNextCreate         bool
	failNextQuery          bool
	nextExternalOrderID    string
}

func NewFakePaymentProvider() *FakePaymentProvider {
	secret := sha256.Sum256([]byte("account-portfolio-fake-payment-provider"))
	return &FakePaymentProvider{
		secret:                 secret[:],
		orders:                 make(map[string]ProviderOrder),
		merchantExternalOrders: make(map[string]string),
	}
}

func (p *FakePaymentProvider) Name() string { return "fake" }

func (p *FakePaymentProvider) Sign(_ context.Context, request PaymentOrderRequest) (SignedPaymentOrder, error) {
	if p == nil || request.MerchantOrderID == "" || request.AmountCents != lifetimeMembershipAmountCents || request.Product != "lifetime" {
		return SignedPaymentOrder{}, errors.New("fake payment order is invalid")
	}
	return SignedPaymentOrder{Request: request, Signature: p.signOrder(request)}, nil
}

func (p *FakePaymentProvider) CreateOrder(_ context.Context, signed SignedPaymentOrder) (ProviderOrder, error) {
	if p == nil || !hmac.Equal([]byte(signed.Signature), []byte(p.signOrder(signed.Request))) {
		return ProviderOrder{}, errors.New("fake payment signature is invalid")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.createCalls++
	if p.failNextCreate {
		p.failNextCreate = false
		return ProviderOrder{}, errors.New("fake payment provider create failure")
	}
	if existing := p.merchantExternalOrders[signed.Request.MerchantOrderID]; existing != "" {
		return p.orders[existing], nil
	}
	p.nextExternalOrder++
	externalOrderID := p.nextExternalOrderID
	p.nextExternalOrderID = ""
	if externalOrderID == "" {
		externalOrderID = fmt.Sprintf("fake-order-%d", p.nextExternalOrder)
	}
	order := ProviderOrder{
		ExternalOrderID: externalOrderID,
		MerchantOrderID: signed.Request.MerchantOrderID,
		AmountCents:     signed.Request.AmountCents,
		Status:          MembershipOrderPendingPayment,
	}
	p.orders[externalOrderID] = order
	p.merchantExternalOrders[signed.Request.MerchantOrderID] = externalOrderID
	return order, nil
}

func (p *FakePaymentProvider) QueryOrder(_ context.Context, externalOrderID string) (ProviderOrder, error) {
	if p == nil {
		return ProviderOrder{}, errors.New("fake payment provider is unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failNextQuery {
		p.failNextQuery = false
		return ProviderOrder{}, errors.New("fake payment provider query failure")
	}
	order, ok := p.orders[externalOrderID]
	if !ok {
		return ProviderOrder{}, errors.New("fake payment order was not found")
	}
	return order, nil
}

func (p *FakePaymentProvider) VerifyNotification(_ context.Context, raw []byte) (VerifiedPaymentNotification, error) {
	if p == nil {
		return VerifiedPaymentNotification{}, errors.New("fake payment provider is unavailable")
	}
	var wire struct {
		Notification VerifiedPaymentNotification `json:"notification"`
		Signature    string                      `json:"signature"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil || !hmac.Equal([]byte(wire.Signature), []byte(p.signNotification(wire.Notification))) {
		return VerifiedPaymentNotification{}, errors.New("fake payment notification is invalid")
	}
	return wire.Notification, nil
}

func (p *FakePaymentProvider) Refund(_ context.Context, externalOrderID string) (PaymentRefund, error) {
	if p == nil {
		return PaymentRefund{}, errors.New("fake payment provider is unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	order, ok := p.orders[externalOrderID]
	if !ok || order.Status != MembershipOrderPaid {
		return PaymentRefund{}, errors.New("fake payment order cannot be refunded")
	}
	p.nextEvent++
	order.Status = MembershipOrderRefunded
	p.orders[externalOrderID] = order
	return PaymentRefund{Notification: VerifiedPaymentNotification{
		EventID:         fmt.Sprintf("fake-refund-%d", p.nextEvent),
		ExternalOrderID: order.ExternalOrderID,
		MerchantOrderID: order.MerchantOrderID,
		AmountCents:     order.AmountCents,
		Status:          MembershipOrderRefunded,
		Sequence:        int64(p.nextEvent),
		OccurredAt:      time.Now().UTC(),
	}}, nil
}

// Transition records a fake provider-side state before producing the signed
// callback payload that Account Portfolio verifies in tests.
func (p *FakePaymentProvider) Transition(externalOrderID string, status MembershipOrderStatus) (VerifiedPaymentNotification, error) {
	if p == nil || !validProviderNotificationStatus(status) {
		return VerifiedPaymentNotification{}, errors.New("fake payment transition is invalid")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	order, ok := p.orders[externalOrderID]
	if !ok {
		return VerifiedPaymentNotification{}, errors.New("fake payment order was not found")
	}
	p.nextEvent++
	order.Status = status
	p.orders[externalOrderID] = order
	return VerifiedPaymentNotification{
		EventID:         fmt.Sprintf("fake-event-%d", p.nextEvent),
		ExternalOrderID: order.ExternalOrderID,
		MerchantOrderID: order.MerchantOrderID,
		AmountCents:     order.AmountCents,
		Status:          status,
		Sequence:        int64(p.nextEvent),
		OccurredAt:      time.Now().UTC(),
	}, nil
}

// NewNotification provides a separately signed, potentially stale event
// without changing the fake provider's current order state.
func (p *FakePaymentProvider) NewNotification(externalOrderID string, status MembershipOrderStatus, sequence int64, eventID string) (VerifiedPaymentNotification, error) {
	if p == nil || !validProviderNotificationStatus(status) || sequence < 1 || strings.TrimSpace(eventID) == "" {
		return VerifiedPaymentNotification{}, errors.New("fake payment notification is invalid")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	order, ok := p.orders[externalOrderID]
	if !ok {
		return VerifiedPaymentNotification{}, errors.New("fake payment order was not found")
	}
	return VerifiedPaymentNotification{
		EventID:         eventID,
		ExternalOrderID: order.ExternalOrderID,
		MerchantOrderID: order.MerchantOrderID,
		AmountCents:     order.AmountCents,
		Status:          status,
		Sequence:        sequence,
		OccurredAt:      time.Now().UTC(),
	}, nil
}

func (p *FakePaymentProvider) NotificationPayload(notification VerifiedPaymentNotification) []byte {
	if p == nil {
		return nil
	}
	payload, _ := json.Marshal(struct {
		Notification VerifiedPaymentNotification `json:"notification"`
		Signature    string                      `json:"signature"`
	}{Notification: notification, Signature: p.signNotification(notification)})
	return payload
}

func (p *FakePaymentProvider) ExternalOrderID(merchantOrderID string) string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.merchantExternalOrders[merchantOrderID]
}

func (p *FakePaymentProvider) FailNextCreate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNextCreate = true
}

func (p *FakePaymentProvider) FailNextQuery() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNextQuery = true
}

func (p *FakePaymentProvider) UseExternalOrderIDOnNextCreate(externalOrderID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextExternalOrderID = externalOrderID
}

func (p *FakePaymentProvider) CreateCalls() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.createCalls
}

func (p *FakePaymentProvider) signOrder(request PaymentOrderRequest) string {
	mac := hmac.New(sha256.New, p.secret)
	_, _ = mac.Write([]byte(strings.Join([]string{request.MerchantOrderID, fmt.Sprintf("%d", request.AmountCents), request.Product}, "\n")))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (p *FakePaymentProvider) signNotification(notification VerifiedPaymentNotification) string {
	mac := hmac.New(sha256.New, p.secret)
	_, _ = mac.Write([]byte(strings.Join([]string{
		notification.EventID,
		notification.ExternalOrderID,
		notification.MerchantOrderID,
		fmt.Sprintf("%d", notification.AmountCents),
		string(notification.Status),
		fmt.Sprintf("%d", notification.Sequence),
		notification.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, "\n")))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
