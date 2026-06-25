package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"final-review-platform/services/api/internal/smoke"
)

func main() {
	var cfg smoke.Config
	var timeoutSeconds int
	flag.StringVar(&cfg.BaseURL, "base-url", env("SMOKE_API_BASE_URL", "http://localhost:8080/api/v1"), "API base URL, including /api/v1")
	flag.StringVar(&cfg.Email, "email", env("SMOKE_EMAIL", ""), "student email used for login checks")
	flag.StringVar(&cfg.Code, "code", env("SMOKE_CODE", ""), "email verification code; optional in development when devCode is returned")
	flag.StringVar(&cfg.Name, "name", env("SMOKE_NAME", "Smoke Tester"), "display name for login")
	flag.StringVar(&cfg.AdminEmail, "admin-email", env("SMOKE_ADMIN_EMAIL", ""), "admin email used when -grant-package-access is enabled")
	flag.StringVar(&cfg.AdminCode, "admin-code", env("SMOKE_ADMIN_CODE", ""), "admin verification code; optional in development when devCode is returned")
	flag.StringVar(&cfg.AdminName, "admin-name", env("SMOKE_ADMIN_NAME", "Smoke Admin"), "admin display name for login")
	flag.StringVar(&cfg.PackageID, "package-id", env("SMOKE_PACKAGE_ID", ""), "course package id to test; defaults to first published package")
	flag.StringVar(&cfg.OrderID, "order-id", env("SMOKE_ORDER_ID", ""), "order id used by -verify-paid-order")
	flag.BoolVar(&cfg.SkipLogin, "skip-login", boolEnv("SMOKE_SKIP_LOGIN", false), "skip login, paid-download, and order checks")
	flag.BoolVar(&cfg.CreateOrder, "create-order", boolEnv("SMOKE_CREATE_ORDER", false), "create a pending order and read its status")
	flag.BoolVar(&cfg.MockWeChatPay, "mock-wechat-pay", boolEnv("SMOKE_MOCK_WECHAT_PAY", false), "development/test only: create an order, request mock WeChat Native payment, send signed mock notify, and verify entitlement")
	flag.StringVar(&cfg.MockWeChatSecret, "mock-wechat-secret", env("SMOKE_MOCK_WECHAT_SECRET", env("WECHAT_PAY_API_V3_KEY", "")), "mock WeChat notify HMAC secret; must match WECHAT_PAY_API_V3_KEY on the API")
	flag.BoolVar(&cfg.WeChatLiveNative, "wechat-live-native", boolEnv("SMOKE_WECHAT_LIVE_NATIVE", false), "live/staging only: create an order, request a non-mock WeChat Native codeUrl, close the order, and verify no entitlement was granted")
	flag.BoolVar(&cfg.VerifyPaidOrder, "verify-paid-order", boolEnv("SMOKE_VERIFY_PAID_ORDER", false), "verify an existing paid order after real notify: checks paid status, entitlement, package material, and paid download without changing payment state")
	flag.BoolVar(&cfg.ExpectPaidDenied, "expect-paid-denied", boolEnv("SMOKE_EXPECT_PAID_DENIED", true), "expect paid package material download to be denied before entitlement")
	flag.BoolVar(&cfg.GrantPackageAccess, "grant-package-access", boolEnv("SMOKE_GRANT_PACKAGE_ACCESS", false), "login as admin, manually grant the selected package to the smoke user, then verify paid download succeeds")
	flag.BoolVar(&cfg.PaymentOpsReadiness, "payment-ops-readiness", boolEnv("SMOKE_PAYMENT_OPS_READINESS", false), "login as admin and fail if payment reconciliation or incident summaries contain critical/high/overdue open risks")
	flag.IntVar(&timeoutSeconds, "timeout-seconds", intEnv("SMOKE_TIMEOUT_SECONDS", 15), "HTTP client timeout in seconds")
	flag.Parse()

	cfg.Timeout = time.Duration(timeoutSeconds) * time.Second
	runner, err := smoke.NewRunner(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result := runner.Run(context.Background())
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(string(output))
	if !result.Passed {
		os.Exit(1)
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func boolEnv(key string, fallback bool) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}

func intEnv(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
