package main

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestIsPlaceholderSecret(t *testing.T) {
	tests := []struct {
		secret string
		want   bool
	}{
		{secret: "replace-account-portfolio-client-secret-32bytes-min!!", want: true},
		{secret: "change-me-to-a-random-secret-with-at-least-32-bytes", want: true},
		{secret: "example-secret-with-at-least-32-random-looking-bytes", want: true},
		{secret: "correct-horse-battery-staple-with-entropy-123", want: false},
	}
	for _, test := range tests {
		if got := isPlaceholderSecret(test.secret); got != test.want {
			t.Fatalf("isPlaceholderSecret(%q) = %v, want %v", test.secret, got, test.want)
		}
	}
}

func TestEasyPayProviderRemainsDisabledUnlessTheExplicitGateAndTenantConfigArePresent(t *testing.T) {
	for _, name := range []string{
		"ACCOUNT_PORTFOLIO_EASYPAY_ENABLED",
		"ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL",
		"ACCOUNT_PORTFOLIO_EASYPAY_PID",
		"ACCOUNT_PORTFOLIO_EASYPAY_KEY",
		"ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL",
		"ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL",
	} {
		t.Setenv(name, "")
	}
	provider, err := paymentProviderFromEnv()
	if err != nil || provider != nil {
		t.Fatalf("disabled EasyPay provider = %T, %v; want nil, nil", provider, err)
	}

	t.Setenv("ACCOUNT_PORTFOLIO_EASYPAY_ENABLED", "1")
	if _, err := paymentProviderFromEnv(); err == nil {
		t.Fatal("enabled EasyPay accepted incomplete HENU tenant configuration")
	}

	t.Setenv("ACCOUNT_PORTFOLIO_EASYPAY_BASE_URL", "https://metaview.top/epay")
	t.Setenv("ACCOUNT_PORTFOLIO_EASYPAY_PID", "2001")
	t.Setenv("ACCOUNT_PORTFOLIO_EASYPAY_KEY", "independent-henukit-tenant-secret")
	t.Setenv("ACCOUNT_PORTFOLIO_EASYPAY_NOTIFY_URL", "https://henukit.cn/api/v1/payment-providers/easypay/notifications")
	t.Setenv("ACCOUNT_PORTFOLIO_EASYPAY_RETURN_URL", "https://henukit.cn/account/membership")
	provider, err = paymentProviderFromEnv()
	if err != nil || provider == nil || provider.Name() != "easypay" {
		t.Fatalf("enabled EasyPay provider = %T, %v; want easypay", provider, err)
	}
}

func TestPointCursorKeyFromEnvRequiresAnIndependentStrongAESKey(t *testing.T) {
	valid := base64.StdEncoding.EncodeToString([]byte("account-portfolio-cursor-key-123"))
	placeholder := base64.StdEncoding.EncodeToString(append([]byte("replace-"), bytes.Repeat([]byte("x"), 24)...))
	wrongLength := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 31))
	zero := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, test := range []struct {
		name, encoded, strong string
		wantErr               bool
	}{
		{name: "valid", encoded: valid},
		{name: "missing", wantErr: true},
		{name: "not base64", encoded: "not-a-base64-key", wantErr: true},
		{name: "wrong decoded length", encoded: wrongLength, wantErr: true},
		{name: "all zero", encoded: zero, wantErr: true},
		{name: "production placeholder", encoded: placeholder, strong: "1", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ACCOUNT_PORTFOLIO_POINT_CURSOR_KEY", test.encoded)
			t.Setenv("ACCOUNT_PORTFOLIO_REQUIRE_STRONG_SECRET", test.strong)
			key, err := pointCursorKeyFromEnv()
			if test.wantErr {
				if err == nil {
					t.Fatal("pointCursorKeyFromEnv() accepted an invalid cursor key")
				}
				return
			}
			if err != nil || !bytes.Equal(key, []byte("account-portfolio-cursor-key-123")) {
				t.Fatalf("pointCursorKeyFromEnv() key=%q err=%v", key, err)
			}
		})
	}
}
