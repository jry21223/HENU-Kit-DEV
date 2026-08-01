package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	accountportfolio "henukit.dev/account-portfolio"
)

const (
	refundOwnerID    = "a2222222-2222-4222-8222-222222222222"
	refundOperatorID = "b2222222-2222-4222-8222-222222222222"
)

// newAccountPortfolioServerWithProviderAndConsole builds a service that can both
// reach a payment provider and accept Console operator commands, which is what
// the operator refund path needs.
func newAccountPortfolioServerWithProviderAndConsole(
	t *testing.T, provider accountportfolio.PaymentProvider,
) (*httptest.Server, *pgxpool.Pool) {
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
		ConsoleClientID: "console-gateway",
		ConsoleKeys:     map[string]string{"console-key": consoleServiceSecret},
		PointCursorKey:  pointCursorTestKey,
		PaymentProvider: provider,
	})
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	return httptest.NewServer(handler), pool
}

// paidMembershipOrder drives one order all the way to paid through the real
// purchase and notification path, so the refund tests start from a genuine
// payment fact rather than a hand-written row.
func paidMembershipOrder(t *testing.T, server *httptest.Server, provider *accountportfolio.FakePaymentProvider) (string, int) {
	t.Helper()
	created := sendOwnerJSON(t, server.URL, http.MethodPost, refundOwnerID,
		"/api/v1/account/membership-orders", "nonce-refund-create", "idem_refund_create", `{}`)
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create order status = %d: %s", created.StatusCode, responseText(t, created))
	}
	var envelope struct {
		Data struct {
			Order struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(responseText(t, created)), &envelope); err != nil {
		t.Fatal(err)
	}

	notification, err := provider.Transition(providerOrderIDFor(t, provider), accountportfolio.MembershipOrderPaid)
	if err != nil {
		t.Fatalf("transition fake order to paid: %v", err)
	}
	// The fake provider HMAC-signs its notifications, so the payload has to come
	// from the provider rather than be assembled by hand.
	paid := sendFakePaymentNotification(t, server.URL, "fake", provider.NotificationPayload(notification))
	if paid.StatusCode != http.StatusOK {
		t.Fatalf("paid notification status = %d: %s", paid.StatusCode, responseText(t, paid))
	}
	// Paying bumps the order revision, so the version a caller must supply is the
	// one the payment produced, not the one creation returned.
	var applied struct {
		Data struct {
			Order struct {
				ID      string `json:"id"`
				Version int    `json:"version"`
			} `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(responseText(t, paid)), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Data.Order.ID != envelope.Data.Order.ID || applied.Data.Order.Version < 1 {
		t.Fatalf("paid notification returned an unexpected order: %+v", applied.Data.Order)
	}
	return applied.Data.Order.ID, applied.Data.Order.Version
}

func providerOrderIDFor(t *testing.T, provider *accountportfolio.FakePaymentProvider) string {
	t.Helper()
	ids := provider.OrderIDs()
	if len(ids) != 1 {
		t.Fatalf("fake provider holds %d orders, want exactly 1", len(ids))
	}
	return ids[0]
}

func TestOperatorRefundIsExactlyOnceUnderConcurrentCommands(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithProviderAndConsole(t, provider)
	defer server.Close()
	defer pool.Close()

	orderID, version := paidMembershipOrder(t, server, provider)
	body := `{"reason":"Approved support refund.","expected_version":` + strconv.Itoa(version) + `}`
	route := "/api/v1/console/membership-orders/" + orderID + "/refunds"

	// Ten operators retry the same command at once. Exactly one refund may
	// exist, and the entitlement may be revoked exactly once.
	const attempts = 10
	var wait sync.WaitGroup
	statuses := make([]int, attempts)
	bodies := make([]string, attempts)
	for index := range attempts {
		wait.Add(1)
		go func() {
			defer wait.Done()
			response := sendConsoleJSON(t, server.URL, http.MethodPost, refundOperatorID, route,
				"nonce-refund-concurrent-"+strconv.Itoa(index), "idem_refund_concurrent", body)
			statuses[index] = response.StatusCode
			bodies[index] = responseText(t, response)
		}()
	}
	wait.Wait()

	accepted := 0
	for index, status := range statuses {
		switch status {
		case http.StatusAccepted, http.StatusOK:
			accepted++
			// A success must carry the real order. An empty success would mean a
			// duplicate was answered with the not-yet-filled idempotency claim.
			var envelope struct {
				Data struct {
					Order struct{ ID string } `json:"order"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(bodies[index]), &envelope); err != nil {
				t.Fatalf("decode concurrent refund body: %v", err)
			}
			if envelope.Data.Order.ID != orderID {
				t.Fatalf("a successful refund returned no order: %s", bodies[index])
			}
		case http.StatusConflict, http.StatusServiceUnavailable:
		default:
			t.Fatalf("unexpected concurrent refund status %d (all: %v)", status, statuses)
		}
	}
	if accepted == 0 {
		t.Fatalf("no concurrent refund succeeded: %v", statuses)
	}

	var refundFacts, revocations int
	var plan string
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT count(*) FROM account_portfolio_payment_facts WHERE status='refunded'),
			(SELECT count(*) FROM account_portfolio_membership_events WHERE user_id=$1 AND to_plan='free'),
			(SELECT plan FROM account_portfolio_memberships WHERE user_id=$1)
	`, refundOwnerID).Scan(&refundFacts, &revocations, &plan); err != nil {
		t.Fatal(err)
	}
	if refundFacts != 1 {
		t.Fatalf("refund facts = %d, want exactly 1", refundFacts)
	}
	if revocations != 1 {
		t.Fatalf("entitlement revocations = %d, want exactly 1", revocations)
	}
	if plan != "free" {
		t.Fatalf("membership plan = %q, want free after a settled refund", plan)
	}
}

func TestOperatorRefundRetryReplaysWithoutASecondRefund(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithProviderAndConsole(t, provider)
	defer server.Close()
	defer pool.Close()

	orderID, version := paidMembershipOrder(t, server, provider)
	body := `{"reason":"Approved support refund.","expected_version":` + strconv.Itoa(version) + `}`
	route := "/api/v1/console/membership-orders/" + orderID + "/refunds"

	first := sendConsoleJSON(t, server.URL, http.MethodPost, refundOperatorID, route, "nonce-refund-first", "idem_refund_retry", body)
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first refund status = %d: %s", first.StatusCode, responseText(t, first))
	}
	retry := sendConsoleJSON(t, server.URL, http.MethodPost, refundOperatorID, route, "nonce-refund-retry", "idem_refund_retry", body)
	if retry.StatusCode != http.StatusOK && retry.StatusCode != http.StatusAccepted {
		t.Fatalf("retried refund status = %d: %s", retry.StatusCode, responseText(t, retry))
	}

	var refundFacts int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM account_portfolio_payment_facts WHERE status='refunded'`).Scan(&refundFacts); err != nil {
		t.Fatal(err)
	}
	if refundFacts != 1 {
		t.Fatalf("refund facts after retry = %d, want exactly 1", refundFacts)
	}
}

func TestOperatorRefundRejectsAStaleExpectedVersion(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithProviderAndConsole(t, provider)
	defer server.Close()
	defer pool.Close()

	orderID, version := paidMembershipOrder(t, server, provider)
	stale := `{"reason":"Stale retry.","expected_version":` + strconv.Itoa(version-1) + `}`
	response := sendConsoleJSON(t, server.URL, http.MethodPost, refundOperatorID,
		"/api/v1/console/membership-orders/"+orderID+"/refunds", "nonce-refund-stale", "idem_refund_stale", stale)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("stale refund status = %d, want 409: %s", response.StatusCode, responseText(t, response))
	}

	var refundFacts int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM account_portfolio_payment_facts WHERE status='refunded'`).Scan(&refundFacts); err != nil {
		t.Fatal(err)
	}
	if refundFacts != 0 {
		t.Fatalf("a stale command must not refund anything, facts = %d", refundFacts)
	}
}

func TestPortalSessionCannotReachOperatorRefund(t *testing.T) {
	provider := accountportfolio.NewFakePaymentProvider()
	server, pool := newAccountPortfolioServerWithProviderAndConsole(t, provider)
	defer server.Close()
	defer pool.Close()

	orderID, version := paidMembershipOrder(t, server, provider)
	body := `{"reason":"Self refund.","expected_version":` + strconv.Itoa(version) + `}`
	// A Portal owner credential must not be able to drive a Console command.
	response := sendOwnerJSON(t, server.URL, http.MethodPost, refundOwnerID,
		"/api/v1/console/membership-orders/"+orderID+"/refunds", "nonce-refund-portal", "idem_refund_portal", body)
	if response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden {
		t.Fatalf("portal-driven refund status = %d, want 401 or 403: %s", response.StatusCode, responseText(t, response))
	}

	var refundFacts int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM account_portfolio_payment_facts WHERE status='refunded'`).Scan(&refundFacts); err != nil {
		t.Fatal(err)
	}
	if refundFacts != 0 {
		t.Fatalf("a portal-driven refund must change nothing, facts = %d", refundFacts)
	}
}
