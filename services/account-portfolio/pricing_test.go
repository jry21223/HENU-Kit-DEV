package accountportfolio

import (
	"context"
	"net/url"
	"testing"
)

// TestConfigureMembershipPricing verifies that the configured product pricing
// flows into both checkout params and every provider-side amount check, and
// that an unset configuration keeps the documented defaults.
func TestConfigureMembershipPricing(t *testing.T) {
	if lifetimeMembershipAmountCents != 990 || lifetimeMembershipProductName != "HENU Kit 终身会员" {
		t.Fatalf("default pricing changed: amount=%d name=%q", lifetimeMembershipAmountCents, lifetimeMembershipProductName)
	}

	provider, err := NewEasyPayProvider(EasyPayConfig{
		BaseURL:   "https://pay.example.test/epay",
		PID:       "2001",
		Key:       "test-secret",
		NotifyURL: "https://henukit.cn/api/v1/payment-providers/easypay/notifications",
		ReturnURL: "https://henukit.cn/account/membership",
	})
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}

	merchant, err := newMembershipMerchantOrderID()
	if err != nil {
		t.Fatalf("merchant id: %v", err)
	}

	ConfigureMembershipPricing(1290, "HENU Kit 终身会员 Plus")
	defer ConfigureMembershipPricing(990, "HENU Kit 终身会员")

	if _, err := provider.Sign(context.Background(), PaymentOrderRequest{
		MerchantOrderID: merchant,
		AmountCents:     990, // 旧价格必须被拒绝，防止下单与校验不一致
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
	}); err == nil {
		t.Fatal("Sign accepted the old default price after pricing was configured")
	}

	signed, err := provider.Sign(context.Background(), PaymentOrderRequest{
		MerchantOrderID: merchant,
		AmountCents:     1290,
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
	})
	if err != nil {
		t.Fatalf("Sign rejected the configured price: %v", err)
	}

	params := provider.createParams(signed.Request)
	if params["money"] != "12.90" || params["name"] != "HENU Kit 终身会员 Plus" {
		t.Fatalf("checkout params not configured: money=%q name=%q", params["money"], params["name"])
	}

	// 回调金额校验同样使用配置后的价格
	callbackParams := map[string]string{
		"pid":          provider.pid,
		"trade_no":     "42000000000000000001",
		"out_trade_no": merchant,
		"type":         "wxpay",
		"name":         "HENU Kit 终身会员 Plus",
		"money":        "12.90",
		"trade_status": "TRADE_SUCCESS",
		"sign_type":    "MD5",
	}
	callbackParams["sign"] = easyPaySign(callbackParams, provider.key)
	if _, err := provider.VerifyNotification(context.Background(), []byte(encodeValues(callbackParams))); err != nil {
		t.Fatalf("notification with configured price rejected: %v", err)
	}
	callbackParams["money"] = "12.91"
	callbackParams["sign"] = easyPaySign(callbackParams, provider.key)
	if _, err := provider.VerifyNotification(context.Background(), []byte(encodeValues(callbackParams))); err == nil {
		t.Fatal("notification with mismatched amount was accepted")
	}
}

func encodeValues(params map[string]string) string {
	values := url.Values{}
	for name, value := range params {
		values.Set(name, value)
	}
	return values.Encode()
}
