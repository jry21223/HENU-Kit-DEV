package accountportfolio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	refundTestPID      = "2001"
	refundTestSecret   = "test-henukit-tenant-secret"
	refundTestMerchant = "HNKABCDEFGHIJKLMNOPQRSTUVWXYZ234"
	refundTestRefundID = "HNRABCDEFGHIJKLMNOPQRSTUVWXYZ234"
)

// refundGateway is a signing stand-in for the EasyPay gateway's refund routes.
type refundGateway struct {
	mu       sync.Mutex
	status   string
	refunds  []map[string]string
	queries  []map[string]string
	override func(path string, form map[string]string) (map[string]string, bool)
}

func newRefundGateway(t *testing.T, status string) (*refundGateway, *httptest.Server) {
	t.Helper()
	state := &refundGateway{status: status}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		form := map[string]string{}
		for key := range r.PostForm {
			form[key] = r.PostForm.Get(key)
		}
		if !easyPayVerify(form, refundTestSecret) {
			t.Fatalf("gateway received an unsigned request: %#v", form)
		}
		state.mu.Lock()
		switch r.URL.Path {
		case "/api/refund.php":
			state.refunds = append(state.refunds, form)
		case "/api/refund-query.php":
			state.queries = append(state.queries, form)
		}
		override := state.override
		status := state.status
		state.mu.Unlock()

		if override != nil {
			if payload, handled := override(r.URL.Path, form); handled {
				// An override that supplies its own signature keeps it, so a
				// case can deliberately answer with an unusable one.
				if _, signed := payload["sign"]; !signed {
					payload["sign"] = easyPaySign(payload, refundTestSecret)
				}
				payload["sign_type"] = "MD5"
				_ = json.NewEncoder(w).Encode(payload)
				return
			}
		}
		payload := map[string]string{
			"pid":           refundTestPID,
			"out_trade_no":  form["out_trade_no"],
			"out_refund_no": form["out_refund_no"],
			"refund_status": status,
			"money":         "9.90",
		}
		payload["sign"] = easyPaySign(payload, refundTestSecret)
		payload["sign_type"] = "MD5"
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(server.Close)
	return state, server
}

func newRefundProvider(t *testing.T, baseURL string) *EasyPayProvider {
	t.Helper()
	provider, err := NewEasyPayProvider(EasyPayConfig{
		BaseURL:   baseURL,
		PID:       refundTestPID,
		Key:       refundTestSecret,
		NotifyURL: "https://henukit.cn/api/v1/payment-providers/easypay/notifications",
		ReturnURL: "https://henukit.cn/account/membership",
	})
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	provider.now = func() time.Time { return time.Unix(1785000000, 0).UTC() }
	return provider
}

func TestEasyPayRefundDerivesADeterministicRefundCorrelationAndSendsNoAmount(t *testing.T) {
	state, server := newRefundGateway(t, "succeeded")
	provider := newRefundProvider(t, server.URL)

	refund, err := provider.Refund(context.Background(), refundTestMerchant)
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refund.RefundID != refundTestRefundID {
		t.Fatalf("refund id = %q, want %q", refund.RefundID, refundTestRefundID)
	}
	if !refund.Settled || refund.Notification.Status != MembershipOrderRefunded {
		t.Fatalf("settled refund should be refunded: %+v", refund)
	}
	if refund.Notification.AmountCents != lifetimeMembershipAmountCents {
		t.Fatalf("amount = %d", refund.Notification.AmountCents)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.refunds) != 1 {
		t.Fatalf("expected one refund call, got %d", len(state.refunds))
	}
	// The caller must never be able to influence the refunded amount.
	for _, field := range []string{"money", "amount", "refund_fee", "total_fee"} {
		if _, present := state.refunds[0][field]; present {
			t.Fatalf("refund request must not carry %q: %#v", field, state.refunds[0])
		}
	}
	if state.refunds[0]["out_refund_no"] != refundTestRefundID {
		t.Fatalf("out_refund_no = %q", state.refunds[0]["out_refund_no"])
	}
}

func TestEasyPayRefundIsStableAcrossRetries(t *testing.T) {
	_, server := newRefundGateway(t, "succeeded")
	provider := newRefundProvider(t, server.URL)

	first, err := provider.Refund(context.Background(), refundTestMerchant)
	if err != nil {
		t.Fatalf("first refund: %v", err)
	}
	second, err := provider.Refund(context.Background(), refundTestMerchant)
	if err != nil {
		t.Fatalf("retried refund: %v", err)
	}
	// A retry reuses the same correlation, so the gateway settles on one refund.
	if first.RefundID != second.RefundID {
		t.Fatalf("refund correlation drifted: %q vs %q", first.RefundID, second.RefundID)
	}
}

func TestEasyPayRefundLeavesTheOrderPaidUntilItSettles(t *testing.T) {
	_, server := newRefundGateway(t, "processing")
	provider := newRefundProvider(t, server.URL)

	refund, err := provider.Refund(context.Background(), refundTestMerchant)
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refund.Settled {
		t.Fatal("a processing refund must not report as settled")
	}
	if refund.Status != MembershipRefundProcessing {
		t.Fatalf("status = %q, want processing", refund.Status)
	}
	// An unsettled refund is not a refund fact and must not revoke anything.
	if refund.Notification.Status != MembershipOrderPaid {
		t.Fatalf("status = %q, want %q", refund.Notification.Status, MembershipOrderPaid)
	}
}

func TestEasyPayRefundQuerySettlesAPreviouslyProcessingRefund(t *testing.T) {
	state, server := newRefundGateway(t, "succeeded")
	provider := newRefundProvider(t, server.URL)

	refund, err := provider.QueryRefund(context.Background(), refundTestMerchant)
	if err != nil {
		t.Fatalf("query refund: %v", err)
	}
	if !refund.Settled || refund.Notification.Status != MembershipOrderRefunded {
		t.Fatalf("query should settle the refund: %+v", refund)
	}
	if refund.Status != MembershipRefundSucceeded {
		t.Fatalf("status = %q, want succeeded", refund.Status)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.queries) != 1 || len(state.refunds) != 0 {
		t.Fatalf("query must not submit a refund: queries=%d refunds=%d", len(state.queries), len(state.refunds))
	}
}

// abnormal is not an error for the provider: it is persisted and returned
// honestly so the console query surface can show it, but it never counts as
// settled and never implies a completed refund.
func TestEasyPayRefundReportsAbnormalStatusWithoutSettling(t *testing.T) {
	_, server := newRefundGateway(t, "abnormal")
	provider := newRefundProvider(t, server.URL)

	refund, err := provider.Refund(context.Background(), refundTestMerchant)
	if err != nil {
		t.Fatalf("refund must surface the abnormal state, got error: %v", err)
	}
	if refund.Status != MembershipRefundAbnormal || refund.Settled {
		t.Fatalf("abnormal refund = status %q settled %t, want abnormal and unsettled", refund.Status, refund.Settled)
	}
}

func TestEasyPayRefundClosedStatusLeavesTheOrderPaid(t *testing.T) {
	_, server := newRefundGateway(t, "closed")
	provider := newRefundProvider(t, server.URL)

	refund, err := provider.Refund(context.Background(), refundTestMerchant)
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refund.Status != MembershipRefundClosed || refund.Settled {
		t.Fatalf("closed refund = status %q settled %t, want closed and unsettled", refund.Status, refund.Settled)
	}
	if refund.Notification.Status != MembershipOrderPaid {
		t.Fatalf("a closed refund must leave the order paid, got %q", refund.Notification.Status)
	}
}

func TestEasyPayRefundRejectsUnsignedAndMismatchedResponses(t *testing.T) {
	for name, override := range map[string]func(string, map[string]string) (map[string]string, bool){
		"unsigned": func(_ string, form map[string]string) (map[string]string, bool) {
			// Answer without a usable signature by signing with the wrong key.
			return map[string]string{
				"pid":           refundTestPID,
				"out_trade_no":  form["out_trade_no"],
				"out_refund_no": form["out_refund_no"],
				"refund_status": "succeeded",
				"money":         "9.90",
				"sign":          "00000000000000000000000000000000",
			}, true
		},
		"foreign order": func(_ string, form map[string]string) (map[string]string, bool) {
			return map[string]string{
				"pid":           refundTestPID,
				"out_trade_no":  "HNKZZZZZZZZZZZZZZZZZZZZZZZZZZ234",
				"out_refund_no": form["out_refund_no"],
				"refund_status": "succeeded",
				"money":         "9.90",
			}, true
		},
		"wrong amount": func(_ string, form map[string]string) (map[string]string, bool) {
			return map[string]string{
				"pid":           refundTestPID,
				"out_trade_no":  form["out_trade_no"],
				"out_refund_no": form["out_refund_no"],
				"refund_status": "succeeded",
				"money":         "99.90",
			}, true
		},
	} {
		t.Run(name, func(t *testing.T) {
			state, server := newRefundGateway(t, "succeeded")
			state.mu.Lock()
			state.override = override
			state.mu.Unlock()
			provider := newRefundProvider(t, server.URL)

			if _, err := provider.Refund(context.Background(), refundTestMerchant); err == nil {
				t.Fatal("an untrustworthy refund response must be rejected")
			}
		})
	}
}

func TestMembershipRefundOrderIDIsNamespacedAndRejectsInvalidOrders(t *testing.T) {
	if got := membershipRefundOrderID(refundTestMerchant); got != refundTestRefundID {
		t.Fatalf("refund id = %q, want %q", got, refundTestRefundID)
	}
	// A refund number can never collide with an order number.
	if strings.HasPrefix(membershipRefundOrderID(refundTestMerchant), membershipMerchantOrderPrefix) {
		t.Fatal("refund correlation must not reuse the order namespace")
	}
	for _, invalid := range []string{"", "HNK", "MV20260730120000", strings.Repeat("A", 32)} {
		if got := membershipRefundOrderID(invalid); got != "" {
			t.Fatalf("membershipRefundOrderID(%q) = %q, want empty", invalid, got)
		}
	}
}

// A gateway regression that answered with its own checkout page would put the
// private merchant order number in a browser URL. ADR-0019 requires that to be
// refused rather than shown, so the check lives at the provider boundary.
func TestEasyPayRejectsACheckoutHandleThatIsNotBrowserSafe(t *testing.T) {
	for name, codeURL := range map[string]string{
		"gateway checkout page": "https://pay.metaview.top/pay/" + refundTestMerchant,
		"any https page":        "https://pay.metaview.top/checkout/abc",
		"carries the order id":  "weixin://wxpay/bizpayurl?pr=" + refundTestMerchant,
		"empty":                 "",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "msg": "success", "code_url": codeURL})
			}))
			defer server.Close()
			provider := newRefundProvider(t, server.URL)

			signed, err := provider.Sign(context.Background(), PaymentOrderRequest{
				MerchantOrderID: refundTestMerchant,
				AmountCents:     lifetimeMembershipAmountCents,
				Currency:        lifetimeMembershipCurrency,
				Plan:            lifetimeMembershipPlan,
			})
			if err != nil {
				t.Fatalf("sign: %v", err)
			}
			if _, err := provider.CreateOrder(context.Background(), signed); err == nil {
				t.Fatal("an unsafe checkout handle must be refused")
			}
		})
	}
}
