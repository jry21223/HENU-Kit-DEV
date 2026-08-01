package accountportfolio

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestEasyPayProviderCreatesQueriesAndVerifiesHENUTenantOrders(t *testing.T) {
	const (
		pid      = "2001"
		secret   = "test-henukit-tenant-secret"
		merchant = "HNKABCDEFGHIJKLMNOPQRSTUVWXYZ234"
	)
	var gateway *httptest.Server
	gateway = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/submit.php":
			var params map[string]string
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				t.Fatalf("decode create request: %v", err)
			}
			if params["pid"] != pid || params["out_trade_no"] != merchant ||
				params["notify_url"] != "https://henukit.cn/api/v1/payment-providers/easypay/notifications" ||
				params["return_url"] != "https://henukit.cn/account/membership" ||
				params["money"] != "9.90" || params["sign_type"] != "MD5" ||
				!easyPayVerify(params, secret) {
				t.Fatalf("invalid create params: %#v", params)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":     1,
				"msg":      "success",
				"code_url": gateway.URL + "/pay/" + merchant,
			})
		case "/api/query.php":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse query form: %v", err)
			}
			params := firstFormValues(r.Form)
			if params["pid"] != pid || params["out_trade_no"] != merchant || !easyPayVerify(params, secret) {
				t.Fatalf("invalid query params: %#v", params)
			}
			response := map[string]string{
				"pid":          pid,
				"out_trade_no": merchant,
				"trade_no":     "42000000000000000001",
				"trade_status": "TRADE_SUCCESS",
				"type":         "wxpay",
				"name":         "HENU Kit 终身会员",
				"money":        "9.90",
				"paid_at":      "2026-07-30T13:43:08Z",
				"sign_type":    "MD5",
			}
			response["sign"] = easyPaySign(response, secret)
			_ = json.NewEncoder(w).Encode(response)
		default:
			http.NotFound(w, r)
		}
	}))
	defer gateway.Close()

	provider, err := NewEasyPayProvider(EasyPayConfig{
		BaseURL:    gateway.URL,
		PID:        pid,
		Key:        secret,
		NotifyURL:  "https://henukit.cn/api/v1/payment-providers/easypay/notifications",
		ReturnURL:  "https://henukit.cn/account/membership",
		HTTPClient: gateway.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := PaymentOrderRequest{
		MerchantOrderID: merchant,
		AmountCents:     lifetimeMembershipAmountCents,
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
	}
	signed, err := provider.Sign(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	created, err := provider.CreateOrder(context.Background(), signed)
	if err != nil {
		t.Fatal(err)
	}
	if created.ExternalOrderID != merchant || created.Status != MembershipOrderPendingPayment {
		t.Fatalf("created order = %+v, want HENU pending order", created)
	}

	queried, err := provider.QueryOrder(context.Background(), merchant)
	if err != nil {
		t.Fatal(err)
	}
	if queried.ExternalOrderID != merchant || queried.MerchantOrderID != merchant ||
		queried.Status != MembershipOrderPaid || queried.AmountCents != 990 {
		t.Fatalf("queried order = %+v, want authoritative paid HENU order", queried)
	}

	callback := url.Values{
		"pid":          {pid},
		"trade_no":     {"42000000000000000001"},
		"out_trade_no": {merchant},
		"type":         {"wxpay"},
		"name":         {"HENU Kit 终身会员"},
		"money":        {"9.90"},
		"trade_status": {"TRADE_SUCCESS"},
		"paid_at":      {"2026-07-30T13:43:08Z"},
		"sign_type":    {"MD5"},
	}
	callback.Set("sign", easyPaySign(firstFormValues(callback), secret))
	verified, err := provider.VerifyNotification(context.Background(), []byte(callback.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if verified.EventID != "easypay:42000000000000000001:paid" ||
		verified.ExternalOrderID != merchant || verified.MerchantOrderID != merchant ||
		verified.Status != MembershipOrderPaid || verified.Sequence != 1 ||
		!verified.OccurredAt.Equal(time.Date(2026, 7, 30, 13, 43, 8, 0, time.UTC)) {
		t.Fatalf("verified notification = %+v", verified)
	}
}

func TestEasyPayProviderRejectsWrongTenantPrefixAmountAndSignature(t *testing.T) {
	provider, err := NewEasyPayProvider(EasyPayConfig{
		BaseURL:   "https://pay.example.test/epay",
		PID:       "2001",
		Key:       "test-secret",
		NotifyURL: "https://henukit.cn/api/v1/payment-providers/easypay/notifications",
		ReturnURL: "https://henukit.cn/account/membership",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"pid=1001&trade_no=meta&out_trade_no=mva123&money=9.90&trade_status=TRADE_SUCCESS&sign=bad&sign_type=MD5",
		"pid=2001&trade_no=henu&out_trade_no=HNKABCDEFGHIJKLMNOPQRSTUVWXYZ234&money=10.00&trade_status=TRADE_SUCCESS&sign=bad&sign_type=MD5",
		"pid=2001&trade_no=henu&out_trade_no=HNKABCDEFGHIJKLMNOPQRSTUVWXYZ234&money=9.90&trade_status=TRADE_SUCCESS&sign=bad&sign_type=MD5",
	} {
		if _, err := provider.VerifyNotification(context.Background(), []byte(raw)); err == nil {
			t.Fatalf("accepted invalid callback %q", raw)
		}
	}
	if _, err := provider.Sign(context.Background(), PaymentOrderRequest{
		MerchantOrderID: "mva31c20e206c3440aa643d536fa787c",
		AmountCents:     990,
		Currency:        "CNY",
		Plan:            "lifetime",
	}); err == nil {
		t.Fatal("accepted a MetaView merchant order")
	}
	invalid := PaymentOrderRequest{
		MerchantOrderID: "HNKABCDEFGHIJKLMNOPQRSTUVWXYZ234",
		AmountCents:     1,
		Currency:        "CNY",
		Plan:            "lifetime",
	}
	if _, err := provider.CreateOrder(context.Background(), SignedPaymentOrder{
		Request:   invalid,
		Signature: easyPaySign(provider.createParams(invalid), "test-secret"),
	}); err == nil {
		t.Fatal("created an EasyPay order with a forged amount")
	}
}

func firstFormValues(values url.Values) map[string]string {
	result := make(map[string]string, len(values))
	for key, candidates := range values {
		if len(candidates) > 0 {
			result[key] = candidates[0]
		}
	}
	return result
}
