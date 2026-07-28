package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

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

	externalOrderID := provider.ExternalOrderID(createdBody.Data.Order.ID)
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
	if late.StatusCode != http.StatusOK {
		t.Fatalf("late paid notification status = %d: %s", late.StatusCode, responseText(t, late))
	}
	var lateBody struct {
		Data struct {
			Outcome string `json:"outcome"`
			Order   struct {
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, late, &lateBody)
	if lateBody.Data.Outcome != "ignored" || lateBody.Data.Order.Status != "refunded" {
		t.Fatalf("late paid notification acknowledgement = %+v, want ignored refunded order", lateBody.Data)
	}

	var plan, source string
	var events, notifications, paidFacts, refundedFacts, replayAudits, outOfOrderAudits int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT source FROM account_portfolio_memberships WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_notifications WHERE user_id=$1),
			(SELECT count(*) FROM account_portfolio_payment_facts WHERE order_id=$2 AND status='paid'),
			(SELECT count(*) FROM account_portfolio_payment_facts WHERE order_id=$2 AND status='refunded'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE order_id=$2 AND outcome='notification_replayed'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE order_id=$2 AND outcome='notification_out_of_order')
	`, ownerID, createdBody.Data.Order.ID).Scan(&plan, &source, &events, &notifications, &paidFacts, &refundedFacts, &replayAudits, &outOfOrderAudits); err != nil {
		t.Fatal(err)
	}
	if plan != "free" || source != "payment_refund" || events != 2 || notifications != 2 || paidFacts != 2 || refundedFacts != 1 || replayAudits != 1 || outOfOrderAudits != 1 {
		t.Fatalf("payment lifecycle facts plan/source/events/notifications/paid/refunded/replay/out_of_order = %s/%s/%d/%d/%d/%d/%d/%d, want free/payment_refund/2/2/2/1/1/1", plan, source, events, notifications, paidFacts, refundedFacts, replayAudits, outOfOrderAudits)
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
				Status string `json:"status"`
			} `json:"order"`
		} `json:"data"`
	}
	decodeResponse(t, failed, &failedBody)
	if failedBody.Data.Order.Status != "failed" {
		t.Fatalf("failed provider order = %+v, want a durable failed order", failedBody.Data.Order)
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
	externalOrderID := provider.ExternalOrderID(createdBody.Data.Order.ID)
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

	var createFailures, rejectedNotifications, queryFailures, granted int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='order_creation_failed'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='notification_rejected'),
			(SELECT count(*) FROM account_portfolio_payment_audits WHERE outcome='notification_query_failed'),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1)
	`, ownerID).Scan(&createFailures, &rejectedNotifications, &queryFailures, &granted); err != nil {
		t.Fatal(err)
	}
	if createFailures != 1 || rejectedNotifications != 1 || queryFailures != 1 || granted != 0 {
		t.Fatalf("payment failure audit create/rejected/query/granted = %d/%d/%d/%d, want 1/1/1/0", createFailures, rejectedNotifications, queryFailures, granted)
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
			notification, err := provider.Transition(provider.ExternalOrderID(createdBody.Data.Order.ID), test.status)
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
	externalOrderID := provider.ExternalOrderID(firstBody.Data.Order.ID)
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
