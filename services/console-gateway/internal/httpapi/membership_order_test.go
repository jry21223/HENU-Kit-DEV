package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"henukit.dev/console-gateway/internal/accountportfolio"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

const (
	orderOperatorID = "171f1c6f-7b10-4c92-91a2-b39bf5af5302"
	orderID         = "9f1c2a44-6f3d-4c2b-9a71-2b6d5e8c4a10"
	orderRefundID   = "HNRABCDEFGHIJKLMNOPQRSTUVWXYZ234"
	orderExchange   = "exchange_token_with_at_least_32_characters"
)

func newOrderServer(t *testing.T, owner *fakeAccountPortfolio) (*httptest.Server, *fakePlatform, string) {
	t.Helper()
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: orderExchange}}
	handler, _ := New(
		"https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback",
		platform, nil, fakeOverview{}, redisClient, codec,
		slog.New(slog.NewJSONHandler(io.Discard, nil)), nil, nil, owner,
	)
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	encoded, _ := codec.Encode(session.Value{
		UserID: orderOperatorID, ExchangeToken: orderExchange, ExpiresAt: time.Now().Add(time.Minute),
	})
	return server, platform, encoded
}

func orderRequest(t *testing.T, server *httptest.Server, method, path, encoded, body, key string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func orderEnvelope(status string) json.RawMessage {
	return json.RawMessage(`{"order":{"id":"` + orderID + `","plan":"lifetime","amount_cents":990,"status":"` + status +
		`","version":2,"created_at":"2026-07-30T00:00:00Z","updated_at":"2026-07-30T01:00:00Z"}}`)
}

func refundEnvelope(orderStatus, refundStatus string) json.RawMessage {
	return json.RawMessage(`{"order":{"id":"` + orderID + `","plan":"lifetime","amount_cents":990,"status":"` + orderStatus +
		`","version":3,"created_at":"2026-07-30T00:00:00Z","updated_at":"2026-07-30T01:00:00Z"},` +
		`"refund":{"id":"` + orderRefundID + `","status":"` + refundStatus + `","amount_cents":990}}`)
}

func TestCloseMembershipOrderUsesTheCloseScopeAndTheVerifiedSessionOperator(t *testing.T) {
	owner := &fakeAccountPortfolio{order: orderEnvelope("closed")}
	server, platform, encoded := newOrderServer(t, owner)

	body := `{"reason":"Buyer abandoned the order.","expected_version":2}`
	response := orderRequest(t, server, http.MethodPost,
		"/api/v1/account/membership-orders/"+orderID+"/closures", encoded, body, "idem_order_close")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("close status=%d, want 200", response.StatusCode)
	}
	if owner.closeOrderCalls != 1 || owner.orderID != orderID || owner.key != "idem_order_close" {
		t.Fatalf("close calls/order/key=%d/%s/%q", owner.closeOrderCalls, owner.orderID, owner.key)
	}
	// The operator is the verified session actor, never a browser-supplied one.
	if owner.actor != orderOperatorID {
		t.Fatalf("actor=%s, want the session operator", owner.actor)
	}
	if strings.Join(platform.accountPermissions, ",") != "account.orders.close" {
		t.Fatalf("permissions=%v, want exactly account.orders.close", platform.accountPermissions)
	}
}

func TestRefundMembershipOrderReportsAcceptedAndCarriesNoAmount(t *testing.T) {
	owner := &fakeAccountPortfolio{order: refundEnvelope("paid", "processing")}
	server, platform, encoded := newOrderServer(t, owner)

	body := `{"reason":"Approved support refund.","expected_version":2}`
	response := orderRequest(t, server, http.MethodPost,
		"/api/v1/account/membership-orders/"+orderID+"/refunds", encoded, body, "idem_order_refund")

	// A refund may still be settling, so the Console must not claim it completed.
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("refund status=%d, want 202", response.StatusCode)
	}
	if owner.refundOrderCalls != 1 {
		t.Fatalf("refund calls=%d, want 1", owner.refundOrderCalls)
	}
	// The forwarded command must not be able to choose an amount.
	var forwarded map[string]any
	if err := json.Unmarshal(owner.orderCommandBody, &forwarded); err != nil {
		t.Fatalf("decode forwarded command: %v", err)
	}
	for _, field := range []string{"amount", "amount_cents", "money", "out_trade_no", "merchant_order_id"} {
		if _, present := forwarded[field]; present {
			t.Fatalf("refund command must not carry %q: %v", field, forwarded)
		}
	}
	if strings.Join(platform.accountPermissions, ",") != "account.orders.refund" {
		t.Fatalf("permissions=%v, want exactly account.orders.refund", platform.accountPermissions)
	}
}

func TestReadMembershipOrderRefundUsesTheReadScope(t *testing.T) {
	owner := &fakeAccountPortfolio{order: refundEnvelope("refunded", "succeeded")}
	server, platform, encoded := newOrderServer(t, owner)

	response := orderRequest(t, server, http.MethodGet,
		"/api/v1/account/membership-orders/"+orderID+"/refunds/"+orderRefundID, encoded, "", "")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("refund read status=%d, want 200", response.StatusCode)
	}
	if owner.refundReadCalls != 1 || owner.refundID != orderRefundID {
		t.Fatalf("refund read calls/id=%d/%s", owner.refundReadCalls, owner.refundID)
	}
	if strings.Join(platform.accountPermissions, ",") != "account.orders.read" {
		t.Fatalf("permissions=%v, want exactly account.orders.read", platform.accountPermissions)
	}
}

func TestMembershipOrderCommandsRejectInvalidBodiesBeforeReachingTheOwner(t *testing.T) {
	for name, body := range map[string]string{
		"missing reason":     `{"expected_version":2}`,
		"blank reason":       `{"reason":"   ","expected_version":2}`,
		"missing revision":   `{"reason":"Approved support refund."}`,
		"zero revision":      `{"reason":"Approved support refund.","expected_version":0}`,
		"negative revision":  `{"reason":"Approved support refund.","expected_version":-1}`,
		"caller-set amount":  `{"reason":"Approved refund.","expected_version":2,"amount_cents":990}`,
		"merchant order no":  `{"reason":"Approved refund.","expected_version":2,"out_trade_no":"HNKABCDEFGHIJKLMNOPQRSTUVWXYZ234"}`,
	} {
		t.Run(name, func(t *testing.T) {
			owner := &fakeAccountPortfolio{order: refundEnvelope("paid", "processing")}
			server, _, encoded := newOrderServer(t, owner)
			response := orderRequest(t, server, http.MethodPost,
				"/api/v1/account/membership-orders/"+orderID+"/refunds", encoded, body, "idem_order_refund_bad")
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400", response.StatusCode)
			}
			if owner.refundOrderCalls != 0 {
				t.Fatalf("an invalid command must never reach the owner, calls=%d", owner.refundOrderCalls)
			}
		})
	}
}

func TestMembershipOrderConflictSurfacesAsConflict(t *testing.T) {
	owner := &fakeAccountPortfolio{order: refundEnvelope("paid", "processing"), err: accountportfolio.ErrConflict}
	server, _, encoded := newOrderServer(t, owner)

	response := orderRequest(t, server, http.MethodPost,
		"/api/v1/account/membership-orders/"+orderID+"/refunds", encoded,
		`{"reason":"Stale retry.","expected_version":1}`, "idem_order_refund_conflict")

	// A stale expected_version must not read as a successful refund.
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status=%d, want 409", response.StatusCode)
	}
}

func TestMembershipOrderCommandsRejectAnUnroutableOrderID(t *testing.T) {
	owner := &fakeAccountPortfolio{order: orderEnvelope("closed"), err: accountportfolio.ErrInvalid}
	server, _, encoded := newOrderServer(t, owner)

	response := orderRequest(t, server, http.MethodPost,
		"/api/v1/account/membership-orders/not-a-uuid/closures", encoded,
		`{"reason":"Buyer abandoned the order.","expected_version":2}`, "idem_order_close_bad")

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", response.StatusCode)
	}
}
