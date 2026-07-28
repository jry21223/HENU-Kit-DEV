package accountportfolio

import (
	"context"
	"testing"
)

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
	signed, err := provider.Sign(context.Background(), PaymentOrderRequest{
		MerchantOrderID: "a1111111-1111-4111-8111-111111111111",
		AmountCents:     lifetimeMembershipAmountCents,
		Product:         "lifetime",
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
	refunded, err := provider.Refund(context.Background(), created.ExternalOrderID)
	if err != nil {
		t.Fatal(err)
	}
	if refunded.Notification.Status != MembershipOrderRefunded || refunded.Notification.Sequence <= paid.Sequence {
		t.Fatalf("fake refund = %+v, want a later refunded notification", refunded)
	}
}
