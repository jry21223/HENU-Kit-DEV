package smoke

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunnerPassesInternalSmokeFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}},
		})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_free", "title": "Sample", "accessLevel": "free", "status": "published"},
				{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"},
			},
			"items": []map[string]string{{"resourceType": "material", "resourceId": "mat_paid"}},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "token_123"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke@stu.henu.edu.cn", "role": "user"})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["amount"]; ok {
			t.Fatal("smoke order request must not send amount")
		}
		if body["packageId"] != "pkg_1" {
			t.Fatalf("unexpected package id: %#v", body)
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"order": map[string]interface{}{"id": "ord_1", "status": "pending", "amountTotal": 1990, "currency": "CNY"},
		})
	})
	mux.HandleFunc("/orders/ord_1/status", func(w http.ResponseWriter, r *http.Request) {
		requireBearer(t, w, r)
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"orderId": "ord_1", "status": "pending", "entitlementGranted": false})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{BaseURL: server.URL, Email: "smoke@stu.henu.edu.cn", CreateOrder: true})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected smoke pass, got %#v", result)
	}
	if result.PackageID != "pkg_1" || result.PaidMaterialID != "mat_paid" {
		t.Fatalf("unexpected smoke ids: %#v", result)
	}
	if len(result.Checks) < 10 {
		t.Fatalf("expected full smoke checks, got %#v", result.Checks)
	}
}

func TestRunnerCanGrantPackageAndVerifyPaidDownload(t *testing.T) {
	grantCreated := false
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}},
		})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"},
			},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		token := "student_token"
		if body["email"] == "admin@stu.henu.edu.cn" {
			token = "admin_token"
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": token})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer student_token":
			writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke@stu.henu.edu.cn", "role": "user"})
		case "Bearer admin_token":
			writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "admin_1", "email": "admin@stu.henu.edu.cn", "role": "admin"})
		default:
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
		}
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		if !grantCreated {
			writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("paid content")); err != nil {
			t.Fatal(err)
		}
	})
	mux.HandleFunc("/admin/access-grants", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin_token" {
			writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "forbidden", nil)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["userId"] != "user_1" || body["packageId"] != "pkg_1" {
			t.Fatalf("unexpected grant body: %#v", body)
		}
		if _, ok := body["amount"]; ok {
			t.Fatal("smoke grant request must not send amount")
		}
		grantCreated = true
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"grant": map[string]string{"id": "grant_1"}, "alreadyGranted": false})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{
		BaseURL:            server.URL,
		Email:              "smoke@stu.henu.edu.cn",
		AdminEmail:         "admin@stu.henu.edu.cn",
		ExpectPaidDenied:   true,
		GrantPackageAccess: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected smoke pass, got %#v", result)
	}
	if !grantCreated {
		t.Fatal("expected smoke runner to create a manual package grant")
	}
	wantChecks := []string{"paid download denied before entitlement", "admin login", "manual package grant", "paid download after grant"}
	for _, want := range wantChecks {
		if !hasPassedCheck(result, want) {
			t.Fatalf("expected passed check %q in %#v", want, result.Checks)
		}
	}
}

func TestRunnerCanCompleteMockWeChatPaymentFlow(t *testing.T) {
	const secret = "mock-notify-secret"
	paid := false
	nativeCreated := false
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}},
		})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"},
			},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "student_token"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke@stu.henu.edu.cn", "role": "user"})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		if !paid {
			writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("paid content")); err != nil {
			t.Fatal(err)
		}
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["amount"]; ok {
			t.Fatal("smoke order request must not send amount")
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"order": map[string]interface{}{"id": "ord_1", "outTradeNo": "FR_SMOKE_001", "status": "pending", "amountTotal": 1990, "currency": "CNY"},
		})
	})
	mux.HandleFunc("/orders/ord_1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		status := "pending"
		if nativeCreated {
			status = "paying"
		}
		if paid {
			status = "paid"
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"orderId": "ord_1", "status": status, "entitlementGranted": paid})
	})
	mux.HandleFunc("/payments/wechat/native", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["orderId"] != "ord_1" {
			t.Fatalf("unexpected native body: %#v", body)
		}
		nativeCreated = true
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"orderId":     "ord_1",
			"codeUrl":     "weixin://wxpay/mock/FR_SMOKE_001",
			"status":      "paying",
			"amountTotal": 1990,
			"mock":        true,
		})
	})
	mux.HandleFunc("/payments/wechat/notify", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if r.Header.Get("X-WeChat-Mock-Signature") != mockNotifySignature(body, secret) {
			writeEnvelopeWithMessage(t, w, http.StatusBadRequest, 40000, "invalid_signature", nil)
			return
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["outTradeNo"] != "FR_SMOKE_001" || payload["tradeState"] != "SUCCESS" || int64(payload["amountTotal"].(float64)) != 1990 {
			t.Fatalf("unexpected notify payload: %#v", payload)
		}
		paid = true
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"code":"SUCCESS","message":"success","data":{"processed":true,"paid":true}}`)); err != nil {
			t.Fatal(err)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{
		BaseURL:          server.URL,
		Email:            "smoke@stu.henu.edu.cn",
		ExpectPaidDenied: true,
		MockWeChatPay:    true,
		MockWeChatSecret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected mock wechat smoke pass, got %#v", result)
	}
	for _, want := range []string{"mock wechat native", "mock wechat notify", "paid order status", "paid download after mock payment"} {
		if !hasPassedCheck(result, want) {
			t.Fatalf("expected passed check %q in %#v", want, result.Checks)
		}
	}
}

func TestRunnerCanCheckLiveNativeAndCloseWithoutEntitlement(t *testing.T) {
	nativeCreated := false
	closed := false
	notifyCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}},
		})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"},
			},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "student_token"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke-live@stu.henu.edu.cn", "role": "user"})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["amount"]; ok {
			t.Fatal("smoke order request must not send amount")
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"order": map[string]interface{}{"id": "ord_live_1", "outTradeNo": "FR_LIVE_SMOKE_001", "status": "pending", "amountTotal": 1990, "currency": "CNY"},
		})
	})
	mux.HandleFunc("/orders/ord_live_1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		status := "pending"
		if nativeCreated {
			status = "paying"
		}
		if closed {
			status = "closed"
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"orderId": "ord_live_1", "status": status, "entitlementGranted": false})
	})
	mux.HandleFunc("/payments/wechat/native", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["orderId"] != "ord_live_1" {
			t.Fatalf("unexpected native body: %#v", body)
		}
		nativeCreated = true
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"orderId":     "ord_live_1",
			"codeUrl":     "weixin://wxpay/bizpayurl?pr=liveSmoke",
			"status":      "paying",
			"amountTotal": 1990,
			"mock":        false,
		})
	})
	mux.HandleFunc("/payments/wechat/close", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["orderId"] != "ord_live_1" {
			t.Fatalf("unexpected close body: %#v", body)
		}
		closed = true
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"orderId": "ord_live_1", "status": "closed", "closed": true, "mock": false})
	})
	mux.HandleFunc("/payments/wechat/notify", func(w http.ResponseWriter, r *http.Request) {
		notifyCalled = true
		writeEnvelopeWithMessage(t, w, http.StatusBadRequest, 40000, "notify_not_expected", nil)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{
		BaseURL:          server.URL,
		Email:            "smoke-live@stu.henu.edu.cn",
		ExpectPaidDenied: true,
		WeChatLiveNative: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected live native smoke pass, got %#v", result)
	}
	if !nativeCreated || !closed {
		t.Fatalf("expected live native and close calls, native=%v closed=%v", nativeCreated, closed)
	}
	if notifyCalled {
		t.Fatal("live native smoke must not call notify or simulate payment success")
	}
	for _, want := range []string{"wechat live native", "wechat live close", "wechat live closed order status"} {
		if !hasPassedCheck(result, want) {
			t.Fatalf("expected passed check %q in %#v", want, result.Checks)
		}
	}
}

func TestRunnerCanVerifyPaidOrderAfterOfficialNotify(t *testing.T) {
	downloaded := false
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{
				{"id": "pkg_other", "title": "Other Package", "status": "published", "priceFen": 990},
				{"id": "pkg_paid", "title": "Paid Package", "status": "published", "priceFen": 1990},
			},
		})
	})
	mux.HandleFunc("/packages/pkg_paid", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_paid", "title": "Paid Package", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_paid", "title": "Paid Paper", "accessLevel": "paid", "status": "published"},
			},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "student_token"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_paid", "email": "smoke-paid@stu.henu.edu.cn", "role": "user"})
	})
	mux.HandleFunc("/orders/ord_paid/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"orderId":            "ord_paid",
			"status":             "paid",
			"entitlementGranted": true,
			"paymentProvider":    "wechat_native",
			"productType":        "course_package",
			"productId":          "pkg_paid",
			"amountTotal":        1990,
			"currency":           "CNY",
		})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer student_token" {
			writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
			return
		}
		downloaded = true
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("paid package content")); err != nil {
			t.Fatal(err)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{
		BaseURL:         server.URL,
		Email:           "smoke-paid@stu.henu.edu.cn",
		OrderID:         "ord_paid",
		VerifyPaidOrder: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected paid order verification to pass, got %#v", result)
	}
	if result.PackageID != "pkg_paid" || result.PaidMaterialID != "mat_paid" || !downloaded {
		t.Fatalf("expected derived package and paid download, got result=%#v downloaded=%v", result, downloaded)
	}
	for _, want := range []string{"verify paid order status", "package detail", "paid download after verified order"} {
		if !hasPassedCheck(result, want) {
			t.Fatalf("expected passed check %q in %#v", want, result.Checks)
		}
	}
	if hasPassedCheck(result, "paid download denied before entitlement") {
		t.Fatalf("verify-paid-order should not run unpaid denial check, got %#v", result.Checks)
	}
}

func TestRunnerRejectsUnpaidOrderDuringPaidVerification(t *testing.T) {
	downloadCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"packages": []map[string]interface{}{{"id": "pkg_paid", "title": "Paid Package", "status": "published", "priceFen": 1990}}})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "student_token"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_unpaid", "email": "smoke-unpaid@stu.henu.edu.cn", "role": "user"})
	})
	mux.HandleFunc("/orders/ord_unpaid/status", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"orderId":            "ord_unpaid",
			"status":             "paying",
			"entitlementGranted": false,
			"paymentProvider":    "wechat_native",
			"productType":        "course_package",
			"productId":          "pkg_paid",
			"amountTotal":        1990,
			"currency":           "CNY",
		})
	})
	mux.HandleFunc("/packages/pkg_paid", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package":   map[string]interface{}{"id": "pkg_paid", "title": "Paid Package", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{{"id": "mat_paid", "title": "Paid Paper", "accessLevel": "paid", "status": "published"}},
		})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		downloadCalled = true
		writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{BaseURL: server.URL, Email: "smoke-unpaid@stu.henu.edu.cn", OrderID: "ord_unpaid", VerifyPaidOrder: true})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if result.Passed {
		t.Fatalf("expected unpaid order verification to fail, got %#v", result)
	}
	if downloadCalled {
		t.Fatal("verify-paid-order must not attempt download before paid entitlement status is proven")
	}
	last := result.Checks[len(result.Checks)-1]
	if last.Name != "verify paid order status" || !strings.Contains(last.Detail, "expected paid") {
		t.Fatalf("expected paid order status failure, got %#v", last)
	}
}

func TestRunnerRejectsConflictingPaidOrderVerificationModes(t *testing.T) {
	cases := []Config{
		{BaseURL: "http://example.test/api/v1", OrderID: "ord_1", VerifyPaidOrder: true, CreateOrder: true},
		{BaseURL: "http://example.test/api/v1", OrderID: "ord_1", VerifyPaidOrder: true, MockWeChatPay: true},
		{BaseURL: "http://example.test/api/v1", OrderID: "ord_1", VerifyPaidOrder: true, WeChatLiveNative: true},
		{BaseURL: "http://example.test/api/v1", OrderID: "ord_1", VerifyPaidOrder: true, GrantPackageAccess: true},
		{BaseURL: "http://example.test/api/v1", VerifyPaidOrder: true},
	}
	for _, cfg := range cases {
		if _, err := NewRunner(cfg); err == nil {
			t.Fatalf("expected verify-paid-order config rejection for %#v", cfg)
		}
	}
}

func TestRunnerRejectsConflictingWeChatSmokeModes(t *testing.T) {
	_, err := NewRunner(Config{BaseURL: "http://example.test/api/v1", MockWeChatPay: true, WeChatLiveNative: true})
	if err == nil || !strings.Contains(err.Error(), "cannot be enabled together") {
		t.Fatalf("expected conflicting smoke mode error, got %v", err)
	}
}

func TestRunnerRejectsMockNativeWhenLiveSmokeRequested(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}}})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package":   map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"}},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "student_token"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke-live@stu.henu.edu.cn", "role": "user"})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"order": map[string]interface{}{"id": "ord_mock_1", "outTradeNo": "FR_MOCK_SMOKE_001", "status": "pending", "amountTotal": 1990, "currency": "CNY"},
		})
	})
	mux.HandleFunc("/orders/ord_mock_1/status", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"orderId": "ord_mock_1", "status": "pending", "entitlementGranted": false})
	})
	mux.HandleFunc("/payments/wechat/native", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"orderId":     "ord_mock_1",
			"codeUrl":     "weixin://wxpay/mock/FR_MOCK_SMOKE_001",
			"status":      "paying",
			"amountTotal": 1990,
			"mock":        true,
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{BaseURL: server.URL, Email: "smoke-live@stu.henu.edu.cn", WeChatLiveNative: true})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if result.Passed {
		t.Fatalf("expected live smoke to reject mock native response, got %#v", result)
	}
	last := result.Checks[len(result.Checks)-1]
	if last.Name != "wechat live native" || !strings.Contains(last.Detail, "mock=true") {
		t.Fatalf("expected mock=true live native failure, got %#v", last)
	}
}

func TestRunnerRequiresMockWeChatSecret(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}}})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package":   map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"}},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "student_token"})
	})
	mux.HandleFunc("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"id": "user_1", "email": "smoke@stu.henu.edu.cn", "role": "user"})
	})
	mux.HandleFunc("/materials/mat_paid/download", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "entitlement_required", nil)
	})
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"order": map[string]interface{}{"id": "ord_1", "outTradeNo": "FR_SMOKE_001", "status": "pending", "amountTotal": 1990, "currency": "CNY"},
		})
	})
	mux.HandleFunc("/orders/ord_1/status", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"orderId": "ord_1", "status": "pending", "entitlementGranted": false})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{BaseURL: server.URL, Email: "smoke@stu.henu.edu.cn", MockWeChatPay: true})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if result.Passed {
		t.Fatalf("expected missing secret failure, got %#v", result)
	}
	last := result.Checks[len(result.Checks)-1]
	if last.Name != "mock wechat notify" || !strings.Contains(last.Detail, "mock-wechat-secret") {
		t.Fatalf("expected mock secret failure, got %#v", last)
	}
}

func TestRunnerFailsWhenPackageDetailLeaksStorageKey(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"packages": []map[string]string{{"id": "pkg_1"}}})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package":   map[string]string{"id": "pkg_1"},
			"materials": []map[string]string{{"id": "mat_paid", "accessLevel": "paid", "storageKey": "materials/secret.pdf"}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	runner, err := NewRunner(Config{BaseURL: server.URL, SkipLogin: true})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if result.Passed {
		t.Fatalf("expected smoke failure for storageKey leak, got %#v", result)
	}
	last := result.Checks[len(result.Checks)-1]
	if last.Name != "package detail hides storage keys" || !strings.Contains(last.Detail, "storageKey") {
		t.Fatalf("expected storageKey failure, got %#v", last)
	}
}

func TestRunnerPaymentOpsReadinessPassesWhenNoHighRiskIssues(t *testing.T) {
	server := newPaymentOpsReadinessServer(t, paymentReconciliationSummary{
		Total:  2,
		Medium: 1,
		Low:    1,
		Types:  map[string]int{"medium_manual_review": 1, "low_note": 1},
	}, paymentIncidentSummaryData{
		Open:                    2,
		OpenMedium:              1,
		OpenLow:                 1,
		OverdueThresholdMinutes: 30,
		OpenBySeverity:          map[string]int64{"medium": 1, "low": 1},
		OpenByType:              map[string]int64{"payment_notify_retry": 1},
	})
	defer server.Close()

	runner, err := NewRunner(Config{
		BaseURL:             server.URL,
		SkipLogin:           true,
		AdminEmail:          "admin@example.com",
		PaymentOpsReadiness: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := runner.Run(context.Background())
	if !result.Passed {
		t.Fatalf("expected payment ops readiness to pass, got %#v", result)
	}
	for _, want := range []string{"admin login", "payment reconciliation readiness", "payment incident readiness"} {
		if !hasPassedCheck(result, want) {
			t.Fatalf("expected passed check %q in %#v", want, result.Checks)
		}
	}
}

func TestRunnerPaymentOpsReadinessFailsOnBlockingRisks(t *testing.T) {
	cases := []struct {
		name        string
		recon       paymentReconciliationSummary
		incidents   paymentIncidentSummaryData
		wantCheck   string
		wantMessage string
	}{
		{
			name: "critical reconciliation",
			recon: paymentReconciliationSummary{
				Total:    1,
				Critical: 1,
				Types:    map[string]int{"paid_without_entitlement": 1},
			},
			incidents:   paymentIncidentSummaryData{},
			wantCheck:   "payment reconciliation readiness",
			wantMessage: "critical=1",
		},
		{
			name:  "overdue incident",
			recon: paymentReconciliationSummary{},
			incidents: paymentIncidentSummaryData{
				Open:        1,
				OverdueOpen: 1,
			},
			wantCheck:   "payment incident readiness",
			wantMessage: "overdueOpen=1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newPaymentOpsReadinessServer(t, tc.recon, tc.incidents)
			defer server.Close()

			runner, err := NewRunner(Config{
				BaseURL:             server.URL,
				SkipLogin:           true,
				AdminEmail:          "admin@example.com",
				PaymentOpsReadiness: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			result := runner.Run(context.Background())
			if result.Passed {
				t.Fatalf("expected payment ops readiness to fail, got %#v", result)
			}
			last := result.Checks[len(result.Checks)-1]
			if last.Name != tc.wantCheck || !strings.Contains(last.Detail, tc.wantMessage) {
				t.Fatalf("expected failure %q containing %q, got %#v", tc.wantCheck, tc.wantMessage, last)
			}
		})
	}
}

func newPaymentOpsReadinessServer(t *testing.T, recon paymentReconciliationSummary, incidents paymentIncidentSummaryData) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"ready": true})
	})
	mux.HandleFunc("/schools", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"schools": []map[string]string{{"id": "school_1", "name": "HENU"}}})
	})
	mux.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"packages": []map[string]interface{}{{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990}},
		})
	})
	mux.HandleFunc("/packages/pkg_1", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"package": map[string]interface{}{"id": "pkg_1", "title": "Discrete Math", "status": "published", "priceFen": 1990},
			"materials": []map[string]interface{}{
				{"id": "mat_paid", "title": "Mock Paper", "accessLevel": "paid", "status": "published"},
			},
		})
	})
	mux.HandleFunc("/auth/send-code", func(w http.ResponseWriter, r *http.Request) {
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"devCode": "123456"})
	})
	mux.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != "admin@example.com" {
			t.Fatalf("payment ops readiness should only log in configured admin, got %#v", body)
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{"accessToken": "admin_token"})
	})
	mux.HandleFunc("/admin/payment-reconciliation", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin_token" {
			writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "forbidden", nil)
			return
		}
		if r.URL.Query().Get("limit") != "1000" {
			t.Fatalf("expected reconciliation limit=1000, got %q", r.URL.RawQuery)
		}
		writeEnvelope(t, w, http.StatusOK, map[string]interface{}{
			"issues":  []map[string]interface{}{},
			"total":   recon.Total,
			"summary": recon,
		})
	})
	mux.HandleFunc("/admin/payment-incidents/summary", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin_token" {
			writeEnvelopeWithMessage(t, w, http.StatusForbidden, 40003, "forbidden", nil)
			return
		}
		writeEnvelope(t, w, http.StatusOK, incidents)
	})
	return httptest.NewServer(mux)
}

func hasPassedCheck(result Result, name string) bool {
	for _, check := range result.Checks {
		if check.Name == name && check.Status == "passed" {
			return true
		}
	}
	return false
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, status int, data interface{}) {
	t.Helper()
	writeEnvelopeWithMessage(t, w, status, 0, "ok", data)
}

func writeEnvelopeWithMessage(t *testing.T, w http.ResponseWriter, status int, code int, message string, data interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]interface{}{"code": code, "message": message, "data": data}
	if status >= 400 {
		payload = map[string]interface{}{"code": code, "message": message, "details": data}
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatal(err)
	}
}

func requireBearer(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer token_123" {
		writeEnvelopeWithMessage(t, w, http.StatusUnauthorized, 40001, "unauthorized", nil)
	}
}
