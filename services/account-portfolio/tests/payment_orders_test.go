package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	accountportfolio "henukit.dev/account-portfolio"
)

func TestMembershipOrderPurchaseFailsClosedWhenProviderIsDisabled(t *testing.T) {
	server, pool := newAccountPortfolioServer(t)
	defer server.Close()
	defer pool.Close()

	const ownerID = "a1111111-1111-4111-8111-111111111111"
	response := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-payment-provider-disabled", "idem_payment_disabled", `{}`)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("disabled payment provider status = %d, want 503: %s", response.StatusCode, responseText(t, response))
	}
	if body := responseText(t, response); !strings.Contains(body, `"code":"PAYMENT_PROVIDER_UNAVAILABLE"`) {
		t.Fatalf("disabled payment provider response = %s, want explicit unavailable semantics", body)
	}
	callback := sendFakePaymentNotification(t, server.URL, "fake", []byte(`{"notification":{},"signature":"not-used"}`))
	if callback.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("disabled payment callback status = %d, want 503: %s", callback.StatusCode, responseText(t, callback))
	}
	if body := responseText(t, callback); !strings.Contains(body, `"code":"PAYMENT_PROVIDER_UNAVAILABLE"`) {
		t.Fatalf("disabled payment callback response = %s, want explicit unavailable semantics", body)
	}

	var orders, paymentFacts, membershipEvents int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_membership_orders WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_payment_facts),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID).Scan(&orders, &paymentFacts, &membershipEvents); err != nil {
		t.Fatal(err)
	}
	if orders != 0 || paymentFacts != 0 || membershipEvents != 0 {
		t.Fatalf("disabled payment facts orders/payment_facts/membership_events = %d/%d/%d, want 0/0/0", orders, paymentFacts, membershipEvents)
	}
}

func TestPaymentMerchantIntentIsPrivateAndRejectsCrossOrderCallback(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const ownerID = "a1212121-2121-4121-8121-212121212121"
	firstResponse := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-private-merchant-first", "idem_private_merchant_first", `{}`)
	if firstResponse.StatusCode != http.StatusCreated {
		t.Fatalf("first private merchant order status = %d: %s", firstResponse.StatusCode, responseText(t, firstResponse))
	}
	firstRaw := responseText(t, firstResponse)
	var firstBody struct {
		Data struct {
			Order fakeMembershipOrder `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(firstRaw), &firstBody); err != nil {
		t.Fatal(err)
	}
	if firstBody.Data.Order.ID == "" {
		t.Fatalf("first private merchant order response = %s", firstRaw)
	}
	second := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-private-merchant-second", "idem_private_merchant_second")
	firstMerchantID := paymentIntentMerchantID(t, pool, firstBody.Data.Order.ID)
	secondMerchantID := paymentIntentMerchantID(t, pool, second.ID)
	if firstMerchantID == firstBody.Data.Order.ID || secondMerchantID == second.ID || firstMerchantID == secondMerchantID {
		t.Fatalf("merchant ids must be distinct private values: first=%q/%q second=%q/%q", firstBody.Data.Order.ID, firstMerchantID, second.ID, secondMerchantID)
	}
	if strings.Contains(firstRaw, firstMerchantID) || strings.Contains(firstRaw, secondMerchantID) {
		t.Fatalf("public order response exposed a private merchant id: %s", firstRaw)
	}
	var commandPayload string
	if err := pool.QueryRow(t.Context(), `
		SELECT response_payload::text
		FROM account_portfolio_command_idempotency
		WHERE actor_user_id=$1 AND operation='membership_order_create' AND idempotency_key='idem_private_merchant_first'
	`, ownerID).Scan(&commandPayload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(commandPayload, firstMerchantID) {
		t.Fatalf("durable command response payload exposed a private merchant id: %s", commandPayload)
	}

	firstExternalOrderID := provider.ExternalOrderID(firstMerchantID)
	paid, err := provider.Transition(firstExternalOrderID, accountportfolio.MembershipOrderPaid)
	if err != nil {
		t.Fatal(err)
	}
	forged := paid
	forged.EventID = "fake-cross-order-merchant"
	forged.MerchantOrderID = secondMerchantID
	forgedResponse := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(forged))
	if forgedResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-order merchant callback status = %d, want 400: %s", forgedResponse.StatusCode, responseText(t, forgedResponse))
	}
	_ = responseText(t, forgedResponse)

	correctResponse := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(paid))
	if correctResponse.StatusCode != http.StatusOK {
		t.Fatalf("intent-correlated callback status = %d: %s", correctResponse.StatusCode, responseText(t, correctResponse))
	}
	_ = responseText(t, correctResponse)
	var firstStatus, secondStatus string
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT status FROM account_portfolio_membership_orders WHERE id=$1),
			(SELECT status FROM account_portfolio_membership_orders WHERE id=$2)
	`, firstBody.Data.Order.ID, second.ID).Scan(&firstStatus, &secondStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != "paid" || secondStatus != "pending_payment" {
		t.Fatalf("intent-correlated order states = %q/%q, want paid/pending_payment", firstStatus, secondStatus)
	}
}

func TestPaymentCallbackRequiresExactProviderQueryCorrelation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*accountportfolio.ProviderOrder)
	}{
		{name: "external_order", mutate: func(order *accountportfolio.ProviderOrder) { order.ExternalOrderID = "fake-other-order" }},
		{name: "merchant_order", mutate: func(order *accountportfolio.ProviderOrder) {
			order.MerchantOrderID = "b1313131-3131-4131-8131-313131313131"
		}},
		{name: "amount", mutate: func(order *accountportfolio.ProviderOrder) { order.AmountCents++ }},
		{name: "currency", mutate: func(order *accountportfolio.ProviderOrder) { order.Currency = "USD" }},
		{name: "plan", mutate: func(order *accountportfolio.ProviderOrder) { order.Plan = "other" }},
		{name: "status", mutate: func(order *accountportfolio.ProviderOrder) { order.Status = accountportfolio.MembershipOrderRefunded }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := accountportfolio.NewFakePaymentProvider()
			provider := &queryOverridePaymentProvider{fake: fake, mutate: test.mutate}
			server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
			defer server.Close()
			defer pool.Close()

			const ownerID = "a1313131-3131-4131-8131-313131313131"
			created := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-query-correlation-"+test.name, "idem_query_correlation_"+test.name)
			externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, fake, created.ID)
			paid, err := fake.Transition(externalOrderID, accountportfolio.MembershipOrderPaid)
			if err != nil {
				t.Fatal(err)
			}
			response := sendFakePaymentNotification(t, server.URL, "fake", fake.NotificationPayload(paid))
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s query mismatch status = %d, want 400: %s", test.name, response.StatusCode, responseText(t, response))
			}
			_ = responseText(t, response)
			var rejected int
			if err := pool.QueryRow(t.Context(), `
				SELECT count(*)
				FROM account_portfolio_payment_audits
				WHERE outcome='notification_rejected' AND reason_code='provider_query_mismatch'
			`).Scan(&rejected); err != nil {
				t.Fatal(err)
			}
			if rejected != 1 {
				t.Fatalf("%s query mismatch rejected audits = %d, want 1", test.name, rejected)
			}
		})
	}
}

func TestPaymentProviderEventCannotBeReusedAcrossOrders(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const ownerID = "a1414141-4141-4141-8141-414141414141"
	first := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-event-conflict-first", "idem_event_conflict_first")
	second := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-event-conflict-second", "idem_event_conflict_second")
	firstPaid, err := provider.Transition(fakeExternalOrderIDForLocalOrder(t, pool, provider, first.ID), accountportfolio.MembershipOrderPaid)
	if err != nil {
		t.Fatal(err)
	}
	applyFakeNotification(t, server.URL, provider, firstPaid)
	secondPaid, err := provider.Transition(fakeExternalOrderIDForLocalOrder(t, pool, provider, second.ID), accountportfolio.MembershipOrderPaid)
	if err != nil {
		t.Fatal(err)
	}
	secondPaid.EventID = firstPaid.EventID
	conflicted := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(secondPaid))
	if conflicted.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-order provider event status = %d, want 400: %s", conflicted.StatusCode, responseText(t, conflicted))
	}
	_ = responseText(t, conflicted)
	var rejected, secondFacts int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='notification_rejected' AND reason_code='provider_event_order_conflict'),
			(SELECT count(*) FROM account_portfolio_payment_facts WHERE order_id=$1)
	`, second.ID).Scan(&rejected, &secondFacts); err != nil {
		t.Fatal(err)
	}
	if rejected != 1 || secondFacts != 0 {
		t.Fatalf("cross-order provider event rejected/facts = %d/%d, want 1/0", rejected, secondFacts)
	}
}

func TestVerifiedFakePaymentLifecycleIsIdempotentOrderedAndRefundable(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const ownerID = "a2222222-2222-4222-8222-222222222222"
	created := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-payment-create", "idem_payment_create", `{}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create membership order status = %d: %s", created.StatusCode, responseText(t, created))
	}
	var createdBody struct {
		Data struct {
			Order struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				Version int    `json:"version"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, created, &createdBody)
	if createdBody.Data.Order.ID == "" || createdBody.Data.Order.Status != "pending_payment" || createdBody.Data.Order.Version != 2 {
		t.Fatalf("created membership order = %+v, want one pending durable order", createdBody.Data.Order)
	}

	retry := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-payment-create-retry", "idem_payment_create", `{}`)
	if retry.StatusCode != http.StatusCreated {
		t.Fatalf("idempotent membership order retry status = %d: %s", retry.StatusCode, responseText(t, retry))
	}
	var retryBody struct {
		Data struct {
			Order struct {
				ID string `json:"id"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, retry, &retryBody)
	if retryBody.Data.Order.ID != createdBody.Data.Order.ID {
		t.Fatalf("idempotent membership order id = %q, want %q", retryBody.Data.Order.ID, createdBody.Data.Order.ID)
	}

	externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, provider, createdBody.Data.Order.ID)
	if externalOrderID == "" {
		t.Fatal("Fake Provider did not receive the merchant order id")
	}
	paid, err := provider.Transition(externalOrderID, accountportfolio.MembershipOrderPaid)
	if err != nil {
		t.Fatal(err)
	}
	paidPayload := provider.NotificationPayload(paid)
	acknowledged := sendFakePaymentNotification(t, server.URL, "fake", paidPayload)
	if acknowledged.StatusCode != http.StatusOK {
		t.Fatalf("verified paid notification status = %d: %s", acknowledged.StatusCode, responseText(t, acknowledged))
	}
	var acknowledgedBody struct {
		Data struct {
			Outcome string `json:"outcome"`
			Order   struct {
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, acknowledged, &acknowledgedBody)
	if acknowledgedBody.Data.Outcome != "applied" || acknowledgedBody.Data.Order.Status != "paid" {
		t.Fatalf("paid notification acknowledgement = %+v, want applied paid order", acknowledgedBody.Data)
	}

	membership := sendOwnerJSON(t, server.URL, http.MethodGet, ownerID, "/api/v1/account/membership", "nonce-payment-membership", "", "")
	if membership.StatusCode != http.StatusOK {
		t.Fatalf("membership after verified payment status = %d: %s", membership.StatusCode, responseText(t, membership))
	}
	var membershipBody struct {
		Data struct {
			Plan     string `json:"plan"`
			Lifetime bool   `json:"lifetime"`
		} `json:"data"`
	}
	decodeResponse(t, membership, &membershipBody)
	if membershipBody.Data.Plan != "lifetime" || !membershipBody.Data.Lifetime {
		t.Fatalf("membership after verified payment = %+v, want lifetime entitlement", membershipBody.Data)
	}

	replayed := sendFakePaymentNotification(t, server.URL, "fake", paidPayload)
	if replayed.StatusCode != http.StatusOK {
		t.Fatalf("replayed payment notification status = %d: %s", replayed.StatusCode, responseText(t, replayed))
	}
	var replayedBody struct {
		Data struct {
			Outcome string `json:"outcome"`
		} `json:"data"`
	}
	decodeResponse(t, replayed, &replayedBody)
	if replayedBody.Data.Outcome != "replayed" {
		t.Fatalf("replayed payment notification outcome = %q, want replayed", replayedBody.Data.Outcome)
	}

	refund, err := provider.Refund(context.Background(), externalOrderID)
	if err != nil {
		t.Fatal(err)
	}
	refunded := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(refund.Notification))
	if refunded.StatusCode != http.StatusOK {
		t.Fatalf("verified refund notification status = %d: %s", refunded.StatusCode, responseText(t, refunded))
	}
	var refundedBody struct {
		Data struct {
			Outcome string `json:"outcome"`
			Order   struct {
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, refunded, &refundedBody)
	if refundedBody.Data.Outcome != "applied" || refundedBody.Data.Order.Status != "refunded" {
		t.Fatalf("refund notification acknowledgement = %+v, want applied refunded order", refundedBody.Data)
	}

	latePaid, err := provider.NewNotification(externalOrderID, accountportfolio.MembershipOrderPaid, paid.Sequence, "fake-late-paid")
	if err != nil {
		t.Fatal(err)
	}
	late := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(latePaid))
	if late.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback paid/query refunded mismatch status = %d, want 400: %s", late.StatusCode, responseText(t, late))
	}
	_ = responseText(t, late)

	var plan, source string
	var events, notifications, paidFacts, refundedFacts, replayAudits, outOfOrderAudits, rejectedAudits int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT source FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_payment_facts WHERE order_id=$2 AND status='paid'),
			(SELECT count(*) FROM account_portfolio_payment_facts WHERE order_id=$2 AND status='refunded'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE order_id=$2 AND outcome='notification_replayed'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE order_id=$2 AND outcome='notification_out_of_order'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='notification_rejected' AND reason_code='provider_query_mismatch')
	`, ownerID, createdBody.Data.Order.ID).Scan(&plan, &source, &events, &notifications, &paidFacts, &refundedFacts, &replayAudits, &outOfOrderAudits, &rejectedAudits); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || source != "payment_refund" || events != 2 || notifications != 2 || paidFacts != 1 || refundedFacts != 1 || replayAudits != 1 || outOfOrderAudits != 0 || rejectedAudits != 1 {
		t.Fatalf("payment lifecycle facts plan/source/events/notifications/paid/refunded/replay/out_of_order/rejected = %s/%s/%d/%d/%d/%d/%d/%d/%d, want free/payment_refund/2/2/1/1/1/0/1", plan, source, events, notifications, paidFacts, refundedFacts, replayAudits, outOfOrderAudits, rejectedAudits)
	}
}

func TestPaymentProviderFailuresAreAuditedWithoutRawSignaturesOrSecrets(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const ownerID = "a3333333-3333-4333-8333-333333333333"
	provider.FailNextCreate()
	failed := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-payment-create-failure", "idem_payment_create_failure", `{}`)
	if failed.StatusCode != http.StatusCreated {
		t.Fatalf("failed provider order status = %d: %s", failed.StatusCode, responseText(t, failed))
	}
	var failedBody struct {
		Data struct {
			Order struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, failed, &failedBody)
	if failedBody.Data.Order.ID == "" || failedBody.Data.Order.Status != "created" {
		t.Fatalf("failed provider order = %+v, want a durable retryable intent", failedBody.Data.Order)
	}
	retried := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-payment-create-retry", "idem_payment_create_failure", `{}`)
	if retried.StatusCode != http.StatusCreated {
		t.Fatalf("retryable provider order status = %d: %s", retried.StatusCode, responseText(t, retried))
	}
	var retriedBody struct {
		Data struct {
			Order struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, retried, &retriedBody)
	if retriedBody.Data.Order.ID != failedBody.Data.Order.ID || retriedBody.Data.Order.Status != "pending_payment" {
		t.Fatalf("retryable provider order = %+v, want the same pending order", retriedBody.Data.Order)
	}

	invalid := sendFakePaymentNotification(t, server.URL, "fake", []byte(`{"signature":"untrusted-provider-signature"}`))
	if invalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid provider notification status = %d, want 400: %s", invalid.StatusCode, responseText(t, invalid))
	}
	_ = responseText(t, invalid)

	created := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-payment-query-create", "idem_payment_query_failure", `{}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("query failure order create status = %d: %s", created.StatusCode, responseText(t, created))
	}
	var createdBody struct {
		Data struct {
			Order struct {
				ID string `json:"id"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, created, &createdBody)
	externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, provider, createdBody.Data.Order.ID)
	paid, err := provider.Transition(externalOrderID, accountportfolio.MembershipOrderPaid)
	if err != nil {
		t.Fatal(err)
	}
	provider.FailNextQuery()
	queryFailed := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(paid))
	if queryFailed.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("provider query failure status = %d, want 503: %s", queryFailed.StatusCode, responseText(t, queryFailed))
	}
	_ = responseText(t, queryFailed)

	var deliveryFailures, rejectedNotifications, queryFailures, granted int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='order_delivery_failed'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='notification_rejected'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='notification_query_failed'),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID).Scan(&deliveryFailures, &rejectedNotifications, &queryFailures, &granted); err != nil {
		t.Fatal(err)
	}
	if deliveryFailures != 1 || rejectedNotifications != 1 || queryFailures != 1 || granted != 0 {
		t.Fatalf("payment failure audit delivery/rejected/query/granted = %d/%d/%d/%d, want 1/1/1/0", deliveryFailures, rejectedNotifications, queryFailures, granted)
	}

	rows, err := pool.Query(t.Context(), `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='account_portfolio_payment_audits'
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		if column == "payload" || strings.Contains(column, "signature") || strings.Contains(column, "secret") {
			t.Fatalf("payment audit column %q could retain raw sensitive provider data", column)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var digestBytes int
	if err := pool.QueryRow(t.Context(), `SELECT octet_length(payload_sha256) FROM account_portfolio_payment_audits ORDER BY created_at ASC, id ASC LIMIT 1`).Scan(&digestBytes); err != nil {
		t.Fatal(err)
	}
	if digestBytes != 32 {
		t.Fatalf("payment audit digest bytes = %d, want 32", digestBytes)
	}
}

func TestVerifiedCloseAndFailureTransitionsNeverGrantMembership(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	for _, test := range []struct {
		name    string
		ownerID string
		nonce   string
		key     string
		status  accountportfolio.MembershipOrderStatus
	}{
		{name: "closed", ownerID: "a7777777-7777-4777-8777-777777777777", nonce: "nonce-payment-close", key: "idem_payment_close", status: accountportfolio.MembershipOrderClosed},
		{name: "failed", ownerID: "a8888888-8888-4888-8888-888888888888", nonce: "nonce-payment-failed", key: "idem_payment_failed", status: accountportfolio.MembershipOrderFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			created := sendOwnerJSON(t, server.URL, http.MethodPost, test.ownerID, "/api/v1/account/membership-orders", test.nonce, test.key, `{}`)
			if created.StatusCode != http.StatusCreated {
				t.Fatalf("create membership order status = %d: %s", created.StatusCode, responseText(t, created))
			}
			var createdBody struct {
				Data struct {
					Order struct {
						ID string `json:"id"`
					} `json:"order"`
				} `json:"data"`
			}
			decodeResponse(t, created, &createdBody)
			notification, err := provider.Transition(fakeExternalOrderIDForLocalOrder(t, pool, provider, createdBody.Data.Order.ID), test.status)
			if err != nil {
				t.Fatal(err)
			}
			acknowledged := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(notification))
			if acknowledged.StatusCode != http.StatusOK {
				t.Fatalf("%s notification status = %d: %s", test.status, acknowledged.StatusCode, responseText(t, acknowledged))
			}
			var acknowledgedBody struct {
				Data struct {
					Outcome string `json:"outcome"`
					Order   struct {
						Status string `json:"status"`
					} `json:"order"`
				} `json:"data"`
			}
			decodeResponse(t, acknowledged, &acknowledgedBody)
			if acknowledgedBody.Data.Outcome != "applied" || acknowledgedBody.Data.Order.Status != string(test.status) {
				t.Fatalf("%s acknowledgement = %+v, want applied %s", test.status, acknowledgedBody.Data, test.status)
			}
		})
	}

	var grants int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM account_portfolio_membership_events
		WHERE user_id IN ('a7777777-7777-4777-8777-777777777777', 'a8888888-8888-4888-8888-888888888888')
	`).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("closed or failed payment events = %d, want 0 membership grants", grants)
	}
}

func TestMembershipOrderExternalReferenceIsUniqueAndIdempotencySurvivesARace(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const firstOwnerID = "a4444444-4444-4444-8444-444444444444"
	first := sendOwnerJSON(t, server.URL, http.MethodPost, firstOwnerID, "/api/v1/account/membership-orders", "nonce-payment-external-first", "idem_payment_external_first", `{}`)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first external order status = %d: %s", first.StatusCode, responseText(t, first))
	}
	var firstBody struct {
		Data struct {
			Order struct {
				ID string `json:"id"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, first, &firstBody)
	externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, provider, firstBody.Data.Order.ID)
	provider.UseExternalOrderIDOnNextCreate(externalOrderID)

	const secondOwnerID = "a5555555-5555-4555-8555-555555555555"
	second := sendOwnerJSON(t, server.URL, http.MethodPost, secondOwnerID, "/api/v1/account/membership-orders", "nonce-payment-external-second", "idem_payment_external_second", `{}`)
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("duplicate external order status = %d: %s", second.StatusCode, responseText(t, second))
	}
	var secondBody struct {
		Data struct {
			Order struct {
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, second, &secondBody)
	if secondBody.Data.Order.Status != "failed" {
		t.Fatalf("duplicate external order = %+v, want failed without a duplicate external reference", secondBody.Data.Order)
	}

	const raceOwnerID = "a6666666-6666-4666-8666-666666666666"
	statuses := make(chan int, 2)
	var group sync.WaitGroup
	for index := range 2 {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			response := sendOwnerJSON(t, server.URL, http.MethodPost, raceOwnerID, "/api/v1/account/membership-orders", "nonce-payment-race-"+strconv.Itoa(index), "idem_payment_race", `{}`)
			statuses <- response.StatusCode
			_ = responseText(t, response)
		}(index)
	}
	group.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusCreated {
			t.Fatalf("idempotent purchase race status = %d, want 201", status)
		}
	}

	var externalReferences, racedOrders int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_membership_orders WHERE provider_order_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_orders WHERE user_id=$2)
	`, externalOrderID, raceOwnerID).Scan(&externalReferences, &racedOrders); err != nil {
		t.Fatal(err)
	}
	if externalReferences != 1 || racedOrders != 1 || provider.CreateCalls() != 3 {
		t.Fatalf("external uniqueness/race facts external_refs/raced_orders/create_calls = %d/%d/%d, want 1/1/3", externalReferences, racedOrders, provider.CreateCalls())
	}
}

func TestPostProviderCreatePersistenceFailureRecoversWithSameIntent(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	removeFault := installPaymentOrderBindFailure(t, pool)
	defer removeFault()
	const ownerID = "a9999999-9999-4999-8999-999999999999"
	created := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-post-provider-failure", "idem_post_provider_failure")
	if created.Status != "created" {
		t.Fatalf("faulted provider bind order status = %q, want created retryable intent", created.Status)
	}
	externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, provider, created.ID)
	if externalOrderID == "" || provider.CreateCalls() != 1 {
		t.Fatalf("post-provider failure external/create_calls = %q/%d, want durable external order and one Provider call", externalOrderID, provider.CreateCalls())
	}
	removeFault()

	retried := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-post-provider-retry", "idem_post_provider_failure")
	if retried.ID != created.ID || retried.Status != "pending_payment" {
		t.Fatalf("recovered retry order = %+v, want same pending order %+v", retried, created)
	}
	if provider.CreateCalls() != 2 || fakeExternalOrderIDForLocalOrder(t, pool, provider, created.ID) != externalOrderID {
		t.Fatalf("idempotent Provider recovery create_calls/external = %d/%q, want 2/%q", provider.CreateCalls(), fakeExternalOrderIDForLocalOrder(t, pool, provider, created.ID), externalOrderID)
	}
	var merchantID, deliveryState string
	var attempts int
	if err := pool.QueryRow(t.Context(), `
		SELECT merchant_order_id::text, delivery_state, delivery_attempts
		FROM account_portfolio_payment_order_intents
		WHERE order_id=$1
	`, created.ID).Scan(&merchantID, &deliveryState, &attempts); err != nil {
		t.Fatal(err)
	}
	if merchantID == "" || merchantID == created.ID || deliveryState != "delivered" || attempts != 2 {
		t.Fatalf("recovered intent merchant/state/attempts = %q/%q/%d, want private-merchant/delivered/2", merchantID, deliveryState, attempts)
	}
}

func TestVerifiedCallbackRecoversPostProviderCreateCrash(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	removeFault := installPaymentOrderBindFailure(t, pool)
	defer removeFault()
	const ownerID = "aaaaaaa1-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
	created := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-post-provider-crash", "idem_post_provider_crash")
	if created.Status != "created" {
		t.Fatalf("simulated crash order status = %q, want created", created.Status)
	}
	externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, provider, created.ID)
	if externalOrderID == "" {
		t.Fatal("Provider did not create the durable external order before local bind fault")
	}
	removeFault()

	paid, err := provider.Transition(externalOrderID, accountportfolio.MembershipOrderPaid)
	if err != nil {
		t.Fatal(err)
	}
	callback := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(paid))
	if callback.StatusCode != http.StatusOK {
		t.Fatalf("crash recovery callback status = %d: %s", callback.StatusCode, responseText(t, callback))
	}
	var callbackBody struct {
		Data struct {
			Outcome string `json:"outcome"`
			Order   struct {
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, callback, &callbackBody)
	if callbackBody.Data.Outcome != "applied" || callbackBody.Data.Order.Status != "paid" || provider.CreateCalls() != 1 {
		t.Fatalf("crash recovery callback = %+v with %d Provider creates, want applied paid with one create", callbackBody.Data, provider.CreateCalls())
	}
	var plan, providerOrderID, deliveryState string
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT provider_order_id FROM account_portfolio_membership_orders WHERE id=$2),
			(SELECT delivery_state FROM account_portfolio_payment_order_intents WHERE order_id=$2)
	`, ownerID, created.ID).Scan(&plan, &providerOrderID, &deliveryState); err != nil {
		t.Fatal(err)
	}
	if plan != "lifetime" || providerOrderID != externalOrderID || deliveryState != "delivered" {
		t.Fatalf("crash recovery plan/provider_order/intent = %q/%q/%q, want lifetime/%q/delivered", plan, providerOrderID, deliveryState, externalOrderID)
	}
}

func TestVerifiedRefundCallbackRecoversUnboundOrderWithoutGrantingMembership(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	removeFault := installPaymentOrderBindFailure(t, pool)
	defer removeFault()
	const ownerID = "aabababa-baba-4aba-8aba-abababababab"
	created := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-post-provider-refund-crash", "idem_post_provider_refund_crash")
	if created.Status != "created" {
		t.Fatalf("simulated refund crash order status = %q, want created", created.Status)
	}
	externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, provider, created.ID)
	removeFault()
	if _, err := provider.Transition(externalOrderID, accountportfolio.MembershipOrderPaid); err != nil {
		t.Fatal(err)
	}
	refunded, err := provider.Refund(context.Background(), externalOrderID)
	if err != nil {
		t.Fatal(err)
	}
	callback := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(refunded.Notification))
	if callback.StatusCode != http.StatusOK {
		t.Fatalf("unbound refunded callback status = %d: %s", callback.StatusCode, responseText(t, callback))
	}
	var callbackBody struct {
		Data struct {
			Outcome string `json:"outcome"`
			Order   struct {
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, callback, &callbackBody)
	if callbackBody.Data.Outcome != "applied" || callbackBody.Data.Order.Status != "refunded" {
		t.Fatalf("unbound refunded recovery callback = %+v, want applied refunded", callbackBody.Data)
	}
	var plan, status, providerOrderID, deliveryState string
	var paymentFacts, membershipEvents int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT status FROM account_portfolio_membership_orders WHERE id=$2),
			(SELECT provider_order_id FROM account_portfolio_membership_orders WHERE id=$2),
			(SELECT delivery_state FROM account_portfolio_payment_order_intents WHERE order_id=$2),
			(SELECT count(*) FROM account_portfolio_payment_facts WHERE order_id=$2 AND status='refunded'),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID, created.ID).Scan(&plan, &status, &providerOrderID, &deliveryState, &paymentFacts, &membershipEvents); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || status != "refunded" || providerOrderID != externalOrderID || deliveryState != "delivered" || paymentFacts != 1 || membershipEvents != 0 {
		t.Fatalf("unbound refunded recovery plan/status/provider/intent/facts/events = %q/%q/%q/%q/%d/%d, want free/refunded/%q/delivered/1/0", plan, status, providerOrderID, deliveryState, paymentFacts, membershipEvents, externalOrderID)
	}
}

func TestPaymentDispatchLeaseCoalescesConcurrentRetryAndRecoversAfterExpiry(t *testing.T) {
	fake := accountportfolio.NewFakePaymentProvider()
	provider := newBlockingFakePaymentProvider(fake)
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()
	defer provider.release()

	const ownerID = "aabababb-babb-4abb-8abb-abababababbb"
	firstResponse := make(chan *http.Response, 1)
	go func() {
		firstResponse <- sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-dispatch-lease-first", "idem_dispatch_lease", `{}`)
	}()
	select {
	case <-provider.createStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first Provider create did not start")
	}

	activeRetry := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-dispatch-lease-active", "idem_dispatch_lease", `{}`)
	if activeRetry.StatusCode != http.StatusCreated {
		t.Fatalf("active dispatch retry status = %d: %s", activeRetry.StatusCode, responseText(t, activeRetry))
	}
	_ = responseText(t, activeRetry)
	if provider.createCallCount() != 1 {
		t.Fatalf("concurrent active dispatch Provider calls = %d, want exactly 1", provider.createCallCount())
	}

	var orderID, deliveryState string
	if err := pool.QueryRow(t.Context(), `
		SELECT o.id, i.delivery_state
		FROM account_portfolio_membership_orders o
		JOIN account_portfolio_payment_order_intents i ON i.order_id=o.id
		WHERE o.user_id=$1 AND o.idempotency_key='idem_dispatch_lease'
	`, ownerID).Scan(&orderID, &deliveryState); err != nil {
		t.Fatal(err)
	}
	if deliveryState != "dispatching" {
		t.Fatalf("active dispatch lease state = %q, want dispatching", deliveryState)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE account_portfolio_payment_order_intents
		SET delivery_lease_expires_at=now() - interval '1 second'
		WHERE order_id=$1 AND delivery_state='dispatching'
	`, orderID); err != nil {
		t.Fatal(err)
	}

	expiredRetry := sendOwnerJSON(t, server.URL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", "nonce-dispatch-lease-expired", "idem_dispatch_lease", `{}`)
	if expiredRetry.StatusCode != http.StatusCreated {
		t.Fatalf("expired dispatch retry status = %d: %s", expiredRetry.StatusCode, responseText(t, expiredRetry))
	}
	_ = responseText(t, expiredRetry)
	if provider.createCallCount() != 2 {
		t.Fatalf("expired dispatch recovery Provider calls = %d, want 2", provider.createCallCount())
	}
	provider.release()
	select {
	case response := <-firstResponse:
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("first dispatch response status = %d: %s", response.StatusCode, responseText(t, response))
		}
		_ = responseText(t, response)
	case <-time.After(5 * time.Second):
		t.Fatal("first dispatch did not finish after lease recovery")
	}

	var status, finalDeliveryState string
	var attempts int
	if err := pool.QueryRow(t.Context(), `
		SELECT o.status, i.delivery_state, i.delivery_attempts
		FROM account_portfolio_membership_orders o
		JOIN account_portfolio_payment_order_intents i ON i.order_id=o.id
		WHERE o.id=$1
	`, orderID).Scan(&status, &finalDeliveryState, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "pending_payment" || finalDeliveryState != "delivered" || attempts != 2 || fake.CreateCalls() != 2 {
		t.Fatalf("lease recovery order/status/attempts/fake_calls = %q/%q/%d/%d, want pending_payment/delivered/2/2", status, finalDeliveryState, attempts, fake.CreateCalls())
	}
}

func TestMultiplePaidOrdersKeepLaterEntitlementDuringEarlierRefund(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const ownerID = "abbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	first := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-multi-first", "idem_multi_first")
	second := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-multi-second", "idem_multi_second")
	firstExternal := fakeExternalOrderIDForLocalOrder(t, pool, provider, first.ID)
	secondExternal := fakeExternalOrderIDForLocalOrder(t, pool, provider, second.ID)
	applyFakeTransition(t, server.URL, provider, firstExternal, accountportfolio.MembershipOrderPaid)
	var firstFactID string
	if err := pool.QueryRow(t.Context(), `SELECT payment_fact_id::text FROM account_portfolio_memberships WHERE user_id=$1`, ownerID).Scan(&firstFactID); err != nil {
		t.Fatal(err)
	}
	applyFakeTransition(t, server.URL, provider, secondExternal, accountportfolio.MembershipOrderPaid)
	var secondFactID string
	if err := pool.QueryRow(t.Context(), `SELECT payment_fact_id::text FROM account_portfolio_memberships WHERE user_id=$1`, ownerID).Scan(&secondFactID); err != nil {
		t.Fatal(err)
	}
	if firstFactID == secondFactID {
		t.Fatalf("second paid order did not become the current ownership fact: %q", secondFactID)
	}

	firstRefund, err := provider.Refund(context.Background(), firstExternal)
	if err != nil {
		t.Fatal(err)
	}
	applyFakeNotification(t, server.URL, provider, firstRefund.Notification)
	var plan, currentFactID string
	var events int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT payment_fact_id::text FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID).Scan(&plan, &currentFactID, &events); err != nil {
		t.Fatal(err)
	}
	if plan != "lifetime" || currentFactID != secondFactID || events != 1 {
		t.Fatalf("earlier refund plan/current_fact/events = %q/%q/%d, want lifetime/%q/1", plan, currentFactID, events, secondFactID)
	}

	secondRefund, err := provider.Refund(context.Background(), secondExternal)
	if err != nil {
		t.Fatal(err)
	}
	applyFakeNotification(t, server.URL, provider, secondRefund.Notification)
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID).Scan(&plan, &events); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || events != 2 {
		t.Fatalf("all paid orders refunded plan/events = %q/%d, want free/2", plan, events)
	}
}

func TestStalePaidFactDoesNotBlockRefundOfCurrentOwnershipFact(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const ownerID = "accccccc-cccc-4ccc-8ccc-cccccccccccc"
	created := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-stale-paid", "idem_stale_paid")
	externalOrderID := fakeExternalOrderIDForLocalOrder(t, pool, provider, created.ID)
	if _, err := provider.Transition(externalOrderID, accountportfolio.MembershipOrderPaid); err != nil {
		t.Fatal(err)
	}
	currentPaid, err := provider.NewNotification(externalOrderID, accountportfolio.MembershipOrderPaid, 2, "current-paid")
	if err != nil {
		t.Fatal(err)
	}
	applyFakeNotification(t, server.URL, provider, currentPaid)
	stalePaid, err := provider.NewNotification(externalOrderID, accountportfolio.MembershipOrderPaid, 1, "stale-paid")
	if err != nil {
		t.Fatal(err)
	}
	stale := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(stalePaid))
	if stale.StatusCode != http.StatusOK {
		t.Fatalf("stale paid status = %d: %s", stale.StatusCode, responseText(t, stale))
	}
	var staleBody struct {
		Data struct {
			Outcome string `json:"outcome"`
		} `json:"data"`
	}
	decodeResponse(t, stale, &staleBody)
	if staleBody.Data.Outcome != "ignored" {
		t.Fatalf("stale paid outcome = %q, want ignored", staleBody.Data.Outcome)
	}
	// The stale paid event is valid only while Query still reports paid. Advance
	// the fake Provider without delivering those intermediate callbacks, then
	// use its real refund notification so callback and Query exactly agree.
	if _, err := provider.Transition(externalOrderID, accountportfolio.MembershipOrderPendingPayment); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Transition(externalOrderID, accountportfolio.MembershipOrderPaid); err != nil {
		t.Fatal(err)
	}
	refunded, err := provider.Refund(context.Background(), externalOrderID)
	if err != nil {
		t.Fatal(err)
	}
	applyFakeNotification(t, server.URL, provider, refunded.Notification)
	var plan, source string
	if err := pool.QueryRow(t.Context(), `SELECT plan, source FROM account_portfolio_memberships WHERE user_id=$1`, ownerID).Scan(&plan, &source); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || source != "payment_refund" {
		t.Fatalf("refund after stale paid fact plan/source = %q/%q, want free/payment_refund", plan, source)
	}
}

func TestConcurrentRefundsSerializePaymentFactOwnership(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithPaymentProvider(t, provider)
	defer server.Close()
	defer pool.Close()

	const ownerID = "addddddd-dddd-4ddd-8ddd-dddddddddddd"
	first := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-concurrent-refund-first", "idem_concurrent_refund_first")
	second := createFakeMembershipOrder(t, server.URL, ownerID, "nonce-concurrent-refund-second", "idem_concurrent_refund_second")
	firstExternal := fakeExternalOrderIDForLocalOrder(t, pool, provider, first.ID)
	secondExternal := fakeExternalOrderIDForLocalOrder(t, pool, provider, second.ID)
	applyFakeTransition(t, server.URL, provider, firstExternal, accountportfolio.MembershipOrderPaid)
	applyFakeTransition(t, server.URL, provider, secondExternal, accountportfolio.MembershipOrderPaid)
	firstRefund, err := provider.Refund(context.Background(), firstExternal)
	if err != nil {
		t.Fatal(err)
	}
	secondRefund, err := provider.Refund(context.Background(), secondExternal)
	if err != nil {
		t.Fatal(err)
	}

	statuses := make(chan int, 2)
	var group sync.WaitGroup
	for _, notification := range []accountportfolio.VerifiedPaymentNotification{firstRefund.Notification, secondRefund.Notification} {
		group.Add(1)
		go func(notification accountportfolio.VerifiedPaymentNotification) {
			defer group.Done()
			response := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(notification))
			statuses <- response.StatusCode
			_ = responseText(t, response)
		}(notification)
	}
	group.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent refund status = %d, want 200", status)
		}
	}
	var plan string
	var events int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID).Scan(&plan, &events); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || events != 2 {
		t.Fatalf("concurrent refunds plan/events = %q/%d, want free/2", plan, events)
	}
}

type fakeMembershipOrder struct {
	ID     string
	Status string
}

type blockingFakePaymentProvider struct {
	fake          *accountportfolio.FakePaymentProvider
	createStarted chan struct{}
	releaseCreate chan struct{}
	releaseOnce   sync.Once
	mu            sync.Mutex
	createCalls   int
}

type queryOverridePaymentProvider struct {
	fake   *accountportfolio.FakePaymentProvider
	mutate func(*accountportfolio.ProviderOrder)
}

func (p *queryOverridePaymentProvider) Name() string { return p.fake.Name() }

func (p *queryOverridePaymentProvider) Sign(ctx context.Context, request accountportfolio.PaymentOrderRequest) (accountportfolio.SignedPaymentOrder, error) {
	return p.fake.Sign(ctx, request)
}

func (p *queryOverridePaymentProvider) CreateOrder(ctx context.Context, signed accountportfolio.SignedPaymentOrder) (accountportfolio.ProviderOrder, error) {
	return p.fake.CreateOrder(ctx, signed)
}

func (p *queryOverridePaymentProvider) QueryOrder(ctx context.Context, externalOrderID string) (accountportfolio.ProviderOrder, error) {
	order, err := p.fake.QueryOrder(ctx, externalOrderID)
	if err != nil {
		return accountportfolio.ProviderOrder{}, err
	}
	p.mutate(&order)
	return order, nil
}

func (p *queryOverridePaymentProvider) VerifyNotification(ctx context.Context, payload []byte) (accountportfolio.VerifiedPaymentNotification, error) {
	return p.fake.VerifyNotification(ctx, payload)
}

func (p *queryOverridePaymentProvider) Refund(ctx context.Context, externalOrderID string) (accountportfolio.PaymentRefund, error) {
	return p.fake.Refund(ctx, externalOrderID)
}

func newBlockingFakePaymentProvider(fake *accountportfolio.FakePaymentProvider) *blockingFakePaymentProvider {
	return &blockingFakePaymentProvider{
		fake:          fake,
		createStarted: make(chan struct{}),
		releaseCreate: make(chan struct{}),
	}
}

func (p *blockingFakePaymentProvider) Name() string { return p.fake.Name() }

func (p *blockingFakePaymentProvider) Sign(ctx context.Context, request accountportfolio.PaymentOrderRequest) (accountportfolio.SignedPaymentOrder, error) {
	return p.fake.Sign(ctx, request)
}

func (p *blockingFakePaymentProvider) CreateOrder(ctx context.Context, signed accountportfolio.SignedPaymentOrder) (accountportfolio.ProviderOrder, error) {
	p.mu.Lock()
	p.createCalls++
	first := p.createCalls == 1
	p.mu.Unlock()
	if first {
		close(p.createStarted)
		select {
		case <-p.releaseCreate:
		case <-ctx.Done():
			return accountportfolio.ProviderOrder{}, ctx.Err()
		}
	}
	return p.fake.CreateOrder(ctx, signed)
}

func (p *blockingFakePaymentProvider) QueryOrder(ctx context.Context, externalOrderID string) (accountportfolio.ProviderOrder, error) {
	return p.fake.QueryOrder(ctx, externalOrderID)
}

func (p *blockingFakePaymentProvider) VerifyNotification(ctx context.Context, payload []byte) (accountportfolio.VerifiedPaymentNotification, error) {
	return p.fake.VerifyNotification(ctx, payload)
}

func (p *blockingFakePaymentProvider) Refund(ctx context.Context, externalOrderID string) (accountportfolio.PaymentRefund, error) {
	return p.fake.Refund(ctx, externalOrderID)
}

func (p *blockingFakePaymentProvider) createCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.createCalls
}

func (p *blockingFakePaymentProvider) release() {
	p.releaseOnce.Do(func() { close(p.releaseCreate) })
}

func createFakeMembershipOrder(t *testing.T, serverURL, ownerID, nonce, key string) fakeMembershipOrder {
	t.Helper()
	response := sendOwnerJSON(t, serverURL, http.MethodPost, ownerID, "/api/v1/account/membership-orders", nonce, key, `{}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create fake membership order status = %d: %s", response.StatusCode, responseText(t, response))
	}
	var body struct {
		Data struct {
			Order fakeMembershipOrder `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, response, &body)
	if body.Data.Order.ID == "" {
		t.Fatal("create fake membership order response omitted order id")
	}
	return body.Data.Order
}

func paymentIntentMerchantID(t *testing.T, pool *pgxpool.Pool, orderID string) string {
	t.Helper()
	var merchantOrderID string
	if err := pool.QueryRow(t.Context(), `
		SELECT merchant_order_id::text
		FROM account_portfolio_payment_order_intents
		WHERE order_id=$1
	`, orderID).Scan(&merchantOrderID); err != nil {
		t.Fatal(err)
	}
	if merchantOrderID == "" {
		t.Fatal("payment intent omitted its merchant order id")
	}
	return merchantOrderID
}

func fakeExternalOrderIDForLocalOrder(t *testing.T, pool *pgxpool.Pool, provider *accountportfolio.FakePaymentProvider, orderID string) string {
	t.Helper()
	externalOrderID := provider.ExternalOrderID(paymentIntentMerchantID(t, pool, orderID))
	if externalOrderID == "" {
		t.Fatalf("Fake Provider did not receive local order %q's private merchant id", orderID)
	}
	return externalOrderID
}

func applyFakeTransition(t *testing.T, serverURL string, provider *accountportfolio.FakePaymentProvider, externalOrderID string, status accountportfolio.MembershipOrderStatus) {
	t.Helper()
	notification, err := provider.Transition(externalOrderID, status)
	if err != nil {
		t.Fatal(err)
	}
	applyFakeNotification(t, serverURL, provider, notification)
}

func applyFakeNotification(t *testing.T, serverURL string, provider *accountportfolio.FakePaymentProvider, notification accountportfolio.VerifiedPaymentNotification) {
	t.Helper()
	response := sendFakePaymentNotification(t, serverURL, "fake", provider.NotificationPayload(notification))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("verified fake notification status = %d: %s", response.StatusCode, responseText(t, response))
	}
	_ = responseText(t, response)
}

func installPaymentOrderBindFailure(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION account_portfolio_test_fail_provider_bind()
		RETURNS trigger AS $$
		BEGIN
			IF OLD.provider_order_id IS NULL AND NEW.provider_order_id IS NOT NULL THEN
				RAISE EXCEPTION 'injected post-provider binding failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER account_portfolio_test_fail_provider_bind
		BEFORE UPDATE OF provider_order_id ON account_portfolio_membership_orders
		FOR EACH ROW EXECUTE FUNCTION account_portfolio_test_fail_provider_bind();
	`); err != nil {
		t.Fatal(err)
	}
	removed := false
	remove := func() {
		if removed {
			return
		}
		removed = true
		if _, err := pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS account_portfolio_test_fail_provider_bind ON account_portfolio_membership_orders;
			DROP FUNCTION IF EXISTS account_portfolio_test_fail_provider_bind();
		`); err != nil {
			t.Errorf("remove injected provider-bind failure: %v", err)
		}
	}
	t.Cleanup(remove)
	return remove
}

func newAccountPortfolioServerWithPaymentProvider(t *testing.T, provider accountportfolio.PaymentProvider) (*httptest.Server, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	clearAccountPortfolio(t, pool)
	handler, err := accountportfolio.New(accountportfolio.Config{
		Database:        pool,
		ClientID:        "portal-gateway",
		Keys:            map[string]string{"account-key": serviceSecret},
		PaymentProvider: provider,
	})
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return httptest.NewServer(handler), pool
}

func sendFakePaymentNotification(t *testing.T, serverURL, provider string, payload []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/payment-providers/"+provider+"/notifications", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
