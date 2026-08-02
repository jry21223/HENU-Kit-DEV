package accountportfolio

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestMembershipMerchantOrderIDsUseTheHENUKITPrefix(t *testing.T) {
	seen := make(map[string]struct{}, 256)
	for range 256 {
		merchantOrderID, err := newMembershipMerchantOrderID()
		if err != nil {
			t.Fatal(err)
		}
		if !validMembershipMerchantOrderID(merchantOrderID) {
			t.Fatalf("merchant order id %q is not a valid HENU Kit provider order number", merchantOrderID)
		}
		if !strings.HasPrefix(merchantOrderID, "HNK") || len(merchantOrderID) != 32 {
			t.Fatalf("merchant order id = %q, want HNK plus 29 characters", merchantOrderID)
		}
		if _, exists := seen[merchantOrderID]; exists {
			t.Fatalf("merchant order id %q was generated twice", merchantOrderID)
		}
		seen[merchantOrderID] = struct{}{}
	}
}

func TestMembershipOrderTransitionsAreExplicitAndTerminal(t *testing.T) {
	for _, test := range []struct {
		name string
		from MembershipOrderStatus
		to   MembershipOrderStatus
		want bool
	}{
		{name: "create payment", from: MembershipOrderCreated, to: MembershipOrderPendingPayment, want: true},
		{name: "create failure", from: MembershipOrderCreated, to: MembershipOrderFailed, want: true},
		{name: "pending paid", from: MembershipOrderPendingPayment, to: MembershipOrderPaid, want: true},
		{name: "pending close", from: MembershipOrderPendingPayment, to: MembershipOrderClosed, want: true},
		{name: "pending failure", from: MembershipOrderPendingPayment, to: MembershipOrderFailed, want: true},
		{name: "paid refund", from: MembershipOrderPaid, to: MembershipOrderRefunded, want: true},
		{name: "same pending notice", from: MembershipOrderPendingPayment, to: MembershipOrderPendingPayment, want: true},
		{name: "closed cannot pay", from: MembershipOrderClosed, to: MembershipOrderPaid, want: false},
		{name: "failed cannot pay", from: MembershipOrderFailed, to: MembershipOrderPaid, want: false},
		{name: "refunded cannot pay again", from: MembershipOrderRefunded, to: MembershipOrderPaid, want: false},
		{name: "paid cannot close", from: MembershipOrderPaid, to: MembershipOrderClosed, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := membershipOrderTransitionAllowed(test.from, test.to); got != test.want {
				t.Fatalf("transition %s -> %s allowed = %t, want %t", test.from, test.to, got, test.want)
			}
		})
	}
}

func TestFakePaymentProviderImplementsTheCompletePaymentSeam(t *testing.T) {
	var provider PaymentProvider = NewFakePaymentProvider()
	if _, err := provider.Sign(context.Background(), PaymentOrderRequest{
		MerchantOrderID: "a1111111-1111-4111-8111-111111111111",
		AmountCents:     lifetimeMembershipAmountCents,
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
	}); err == nil {
		t.Fatal("Fake Provider accepted a merchant order without the HNK prefix")
	}
	merchantOrderID, err := newMembershipMerchantOrderID()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := provider.Sign(context.Background(), PaymentOrderRequest{
		MerchantOrderID: merchantOrderID,
		AmountCents:     lifetimeMembershipAmountCents,
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := provider.CreateOrder(context.Background(), signed)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != MembershipOrderPendingPayment || created.ExternalOrderID == "" {
		t.Fatalf("created fake order = %+v, want a pending external order", created)
	}
	queried, err := provider.QueryOrder(context.Background(), created.ExternalOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if queried != created {
		t.Fatalf("queried fake order = %+v, want %+v", queried, created)
	}

	fake := provider.(*FakePaymentProvider)
	paid, err := fake.Transition(created.ExternalOrderID, MembershipOrderPaid)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := provider.VerifyNotification(context.Background(), fake.NotificationPayload(paid))
	if err != nil {
		t.Fatal(err)
	}
	if verified != paid {
		t.Fatalf("verified fake notification = %+v, want %+v", verified, paid)
	}
	refunded, err := provider.Refund(context.Background(), merchantOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if refunded.Notification.Status != MembershipOrderRefunded || refunded.Notification.Sequence <= paid.Sequence {
		t.Fatalf("fake refund = %+v, want a later refunded notification", refunded)
	}
	// The service addresses refund and close operations by the private merchant
	// order number (the gateway's out_trade_no), so the fake must resolve that
	// id to the order it created under its external id.
	reconciled, err := provider.QueryRefund(context.Background(), merchantOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if !reconciled.Settled || reconciled.Status != MembershipRefundSucceeded {
		t.Fatalf("fake refund query by merchant id = %+v, want the settled refund", reconciled)
	}
	if _, err := provider.CloseOrder(context.Background(), merchantOrderID); err == nil || !strings.Contains(err.Error(), "cannot be closed") {
		t.Fatalf("fake close by merchant id must refuse the refunded order, got %v", err)
	}
}

func TestPaymentKernelRollbackGuardsEveryDurablePaymentRecord(t *testing.T) {
	down, err := os.ReadFile("db/migrations/000004_membership_order_payment_kernel.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	guardEnd := strings.Index(string(down), "END $$;")
	if guardEnd < 0 {
		t.Fatal("payment-kernel rollback migration has no durable-record guard")
	}
	guard := string(down[:guardEnd])
	for _, table := range []string{
		"account_portfolio_membership_orders",
		"account_portfolio_payment_order_intents",
		"account_portfolio_payment_facts",
		"account_portfolio_payment_audits",
	} {
		if !strings.Contains(guard, table) {
			t.Fatalf("payment-kernel rollback guard does not protect %s", table)
		}
	}
}

func TestHENUKITMerchantOrderMigrationRefusesAnExistingPaymentPromise(t *testing.T) {
	up, err := os.ReadFile("db/migrations/000006_henukit_merchant_order_prefix.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(up)
	for _, expected := range []string{
		"IF EXISTS (SELECT 1 FROM account_portfolio_payment_order_intents)",
		"RAISE EXCEPTION",
		"ALTER COLUMN merchant_order_id TYPE TEXT",
		"^HNK[A-Z2-7]{29}$",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("HENU Kit merchant-order migration is missing %q", expected)
		}
	}
}
