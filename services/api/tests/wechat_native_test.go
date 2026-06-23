package tests

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/payment"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	"final-review-platform/services/api/pkg/config"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestWeChatNativeMockCreatesPayingOrder(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "wechat-native-package", model.StatusPublished)
	if err := db.Model(&coursePackage).Updates(map[string]interface{}{"price_fen": int64(1990), "currency": "CNY"}).Error; err != nil {
		t.Fatal(err)
	}
	user := createTestUser(t, db, "wechat-native-buyer@stu.henu.edu.cn", model.RoleUser)
	other := createTestUser(t, db, "wechat-native-other@stu.henu.edu.cn", model.RoleUser)
	userToken := loginTestUser(t, router, user.Email)
	otherToken := loginTestUser(t, router, other.Email)

	created := performJSON(router, http.MethodPost, "/api/v1/orders", `{"packageId":"`+coursePackage.ID+`"}`, userToken)
	if created.Code != http.StatusOK {
		t.Fatalf("expected order create 200, got %d: %s", created.Code, created.Body.String())
	}
	var orderPayload struct {
		Data struct {
			Order model.Order `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &orderPayload); err != nil {
		t.Fatal(err)
	}
	order := orderPayload.Data.Order

	unauthorized := performJSON(router, http.MethodPost, "/api/v1/payments/wechat/native", `{"orderId":"`+order.ID+`"}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated native create 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	otherDenied := performJSON(router, http.MethodPost, "/api/v1/payments/wechat/native", `{"orderId":"`+order.ID+`"}`, otherToken)
	if otherDenied.Code != http.StatusForbidden {
		t.Fatalf("expected other user native create 403, got %d: %s", otherDenied.Code, otherDenied.Body.String())
	}

	native := performJSON(router, http.MethodPost, "/api/v1/payments/wechat/native", `{"orderId":"`+order.ID+`"}`, userToken)
	if native.Code != http.StatusOK {
		t.Fatalf("expected native create 200, got %d: %s", native.Code, native.Body.String())
	}
	for _, expected := range []string{order.ID, `"status":"paying"`, `"amountTotal":1990`, `"mock":true`, "weixin://wxpay/mock/"} {
		if !strings.Contains(native.Body.String(), expected) {
			t.Fatalf("expected native response to contain %q, got %s", expected, native.Body.String())
		}
	}
	var stored model.Order
	if err := db.First(&stored, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.OrderPaying {
		t.Fatalf("expected order status paying after native create, got %s", stored.Status)
	}
	if stored.PaidAt != nil {
		t.Fatal("native code URL creation must not mark order paid")
	}
	if stored.Metadata == nil || !strings.Contains(string(stored.Metadata), "wechatNative") {
		t.Fatalf("expected native metadata to be persisted, got %s", string(stored.Metadata))
	}

	status := performJSON(router, http.MethodGet, "/api/v1/orders/"+order.ID+"/status", "", userToken)
	if status.Code != http.StatusOK {
		t.Fatalf("expected order status 200, got %d: %s", status.Code, status.Body.String())
	}
	if !strings.Contains(status.Body.String(), `"status":"paying"`) || !strings.Contains(status.Body.String(), `"entitlementGranted":false`) {
		t.Fatalf("expected paying status without entitlement, got %s", status.Body.String())
	}

	repeated := performJSON(router, http.MethodPost, "/api/v1/payments/wechat/native", `{"orderId":"`+order.ID+`"}`, userToken)
	if repeated.Code != http.StatusOK {
		t.Fatalf("expected repeated native create for paying order 200, got %d: %s", repeated.Code, repeated.Body.String())
	}

	duplicateOrder := performJSON(router, http.MethodPost, "/api/v1/orders", `{"packageId":"`+coursePackage.ID+`"}`, userToken)
	if duplicateOrder.Code != http.StatusOK {
		t.Fatalf("expected duplicate order create after paying 200, got %d: %s", duplicateOrder.Code, duplicateOrder.Body.String())
	}
	if !strings.Contains(duplicateOrder.Body.String(), `"alreadyPending":true`) || !strings.Contains(duplicateOrder.Body.String(), order.ID) {
		t.Fatalf("expected duplicate order create to reuse paying order, got %s", duplicateOrder.Body.String())
	}
	if countOrders(t, db, user.ID, coursePackage.ID) != 1 {
		t.Fatal("expected paying order to be reused instead of creating a duplicate")
	}
}

func TestWeChatNativeRejectsUnpayableOrders(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "wechat-paid-package", model.StatusPublished)
	user := createTestUser(t, db, "wechat-paid-buyer@stu.henu.edu.cn", model.RoleUser)
	token := loginTestUser(t, router, user.Email)
	order := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "PAIDORDER001",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaid,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	response := performJSON(router, http.MethodPost, "/api/v1/payments/wechat/native", `{"orderId":"`+order.ID+`"}`, token)
	if response.Code != http.StatusConflict {
		t.Fatalf("expected paid order native create 409, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "order_not_payable") {
		t.Fatalf("expected order_not_payable, got %s", response.Body.String())
	}
}

func TestWeChatNativeConfigValidation(t *testing.T) {
	if err := payment.ValidateWeChatNativeConfig("test", config.WeChatPayConfig{Mode: "mock"}); err != nil {
		t.Fatalf("expected test mock config to pass, got %v", err)
	}
	if err := payment.ValidateWeChatNativeConfig("production", config.WeChatPayConfig{Mode: "mock"}); !errors.Is(err, payment.ErrWeChatMockForbiddenProduction) {
		t.Fatalf("expected production mock config rejection, got %v", err)
	}
	if err := payment.ValidateWeChatNativeConfig("test", config.WeChatPayConfig{Mode: "unexpected"}); !errors.Is(err, payment.ErrInvalidWeChatMode) {
		t.Fatalf("expected invalid mode rejection, got %v", err)
	}
	if err := payment.ValidateWeChatNativeConfig("test", config.WeChatPayConfig{Mode: "live"}); !errors.Is(err, payment.ErrWeChatLiveConfigMissing) {
		t.Fatalf("expected live missing config rejection, got %v", err)
	}
	live := config.WeChatPayConfig{
		Mode:                "live",
		AppID:               "wx-test",
		MchID:               "mch-test",
		APIV3Key:            "api-v3-key",
		MerchantSerialNo:    "serial",
		MerchantPrivateKey:  "private-key",
		NotifyURL:           "https://example.com/api/v1/payments/wechat/notify",
		NativeExpireMinutes: 15,
	}
	if err := payment.ValidateWeChatNativeConfig("production", live); err != nil {
		t.Fatalf("expected complete live config to pass validation, got %v", err)
	}
}

func TestWeChatNotifyMockRejectsBypassAttempts(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.WeChatPay.APIV3Key = "mock-notify-secret"
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "wechat-notify-bypass-package", model.StatusPublished)
	user := createTestUser(t, db, "wechat-notify-bypass@stu.henu.edu.cn", model.RoleUser)
	order := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "BYPASSORDER001",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaying,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"outTradeNo":"BYPASSORDER001","transactionId":"TX_BYPASS_001","tradeState":"SUCCESS","amountTotal":1990}`
	badSignature := performJSON(router, http.MethodPost, "/api/v1/payments/wechat/notify", body, "")
	if badSignature.Code != http.StatusBadRequest || !strings.Contains(badSignature.Body.String(), "invalid_signature") {
		t.Fatalf("expected invalid signature rejection, got %d: %s", badSignature.Code, badSignature.Body.String())
	}

	mismatchBody := `{"outTradeNo":"BYPASSORDER001","transactionId":"TX_BYPASS_002","tradeState":"SUCCESS","amountTotal":1}`
	mismatch := performSignedMockNotify(router, mismatchBody, cfg.WeChatPay.APIV3Key)
	if mismatch.Code != http.StatusBadRequest || !strings.Contains(mismatch.Body.String(), "amount_mismatch") {
		t.Fatalf("expected amount mismatch rejection, got %d: %s", mismatch.Code, mismatch.Body.String())
	}

	missingBody := `{"outTradeNo":"MISSING_ORDER","transactionId":"TX_BYPASS_003","tradeState":"SUCCESS","amountTotal":1990}`
	missing := performSignedMockNotify(router, missingBody, cfg.WeChatPay.APIV3Key)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "order_not_found") {
		t.Fatalf("expected missing order rejection, got %d: %s", missing.Code, missing.Body.String())
	}

	failedOrder := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FAILEDORDER001",
		PaymentProvider: "wechat_native",
		Status:          model.OrderFailed,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&failedOrder).Error; err != nil {
		t.Fatal(err)
	}
	failedBody := `{"outTradeNo":"FAILEDORDER001","transactionId":"TX_BYPASS_004","tradeState":"SUCCESS","amountTotal":1990}`
	failed := performSignedMockNotify(router, failedBody, cfg.WeChatPay.APIV3Key)
	if failed.Code != http.StatusBadRequest || !strings.Contains(failed.Body.String(), "order_not_payable") {
		t.Fatalf("expected terminal order rejection, got %d: %s", failed.Code, failed.Body.String())
	}

	var stored model.Order
	if err := db.First(&stored, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.OrderPaying || stored.PaidAt != nil {
		t.Fatalf("bypass attempts must not mark order paid, got status=%s paidAt=%v", stored.Status, stored.PaidAt)
	}
	var grants int64
	db.Model(&model.MaterialAccessGrant{}).Where("user_id = ? AND package_id = ?", user.ID, coursePackage.ID).Count(&grants)
	if grants != 0 {
		t.Fatalf("bypass attempts must not grant entitlement, got %d grants", grants)
	}
}

func TestWeChatNotifyMockMarksPaidAndGrantsEntitlementIdempotently(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.WeChatPay.APIV3Key = "mock-notify-secret"
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "wechat-notify-success-package", model.StatusPublished)
	if err := db.Model(&coursePackage).Updates(map[string]interface{}{"price_fen": int64(1990), "currency": "CNY"}).Error; err != nil {
		t.Fatal(err)
	}
	user := createTestUser(t, db, "wechat-notify-success@stu.henu.edu.cn", model.RoleUser)
	token := loginTestUser(t, router, user.Email)

	created := performJSON(router, http.MethodPost, "/api/v1/orders", `{"packageId":"`+coursePackage.ID+`"}`, token)
	if created.Code != http.StatusOK {
		t.Fatalf("expected order create 200, got %d: %s", created.Code, created.Body.String())
	}
	var orderPayload struct {
		Data struct {
			Order model.Order `json:"order"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &orderPayload); err != nil {
		t.Fatal(err)
	}
	order := orderPayload.Data.Order

	body := `{"outTradeNo":"` + order.OutTradeNo + `","transactionId":"TX_SUCCESS_001","tradeState":"SUCCESS","amountTotal":1990}`
	notify := performSignedMockNotify(router, body, cfg.WeChatPay.APIV3Key)
	if notify.Code != http.StatusOK || !strings.Contains(notify.Body.String(), `"code":"SUCCESS"`) {
		t.Fatalf("expected successful notify, got %d: %s", notify.Code, notify.Body.String())
	}

	var stored model.Order
	if err := db.First(&stored, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.OrderPaid || stored.PaidAt == nil {
		t.Fatalf("expected order paid with paidAt, got status=%s paidAt=%v", stored.Status, stored.PaidAt)
	}
	assertPaymentDeliveryCounts(t, db, user.ID, coursePackage.ID, order.ID, 1, 1)

	repeated := performSignedMockNotify(router, body, cfg.WeChatPay.APIV3Key)
	if repeated.Code != http.StatusOK || !strings.Contains(repeated.Body.String(), `"code":"SUCCESS"`) {
		t.Fatalf("expected repeated notify success, got %d: %s", repeated.Code, repeated.Body.String())
	}
	assertPaymentDeliveryCounts(t, db, user.ID, coursePackage.ID, order.ID, 1, 1)

	status := performJSON(router, http.MethodGet, "/api/v1/orders/"+order.ID+"/status", "", token)
	if status.Code != http.StatusOK {
		t.Fatalf("expected order status 200, got %d: %s", status.Code, status.Body.String())
	}
	if !strings.Contains(status.Body.String(), `"status":"paid"`) || !strings.Contains(status.Body.String(), `"entitlementGranted":true`) {
		t.Fatalf("expected paid status with entitlement, got %s", status.Body.String())
	}

	owned := performJSON(router, http.MethodPost, "/api/v1/orders", `{"packageId":"`+coursePackage.ID+`"}`, token)
	if owned.Code != http.StatusOK || !strings.Contains(owned.Body.String(), `"alreadyOwned":true`) {
		t.Fatalf("expected paid package to be already owned, got %d: %s", owned.Code, owned.Body.String())
	}
}

func TestWeChatNotifyMockRequiresExplicitSecret(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	body := `{"outTradeNo":"ANY","transactionId":"TX_NO_SECRET","tradeState":"SUCCESS","amountTotal":1990}`
	response := performSignedMockNotify(router, body, "mock-notify-secret")
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), payment.ErrWeChatMockNotifySecretMissing.Error()) {
		t.Fatalf("expected missing mock notify secret rejection, got %d: %s", response.Code, response.Body.String())
	}
}

func performSignedMockNotify(router http.Handler, body string, secret string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/payments/wechat/notify", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-WeChat-Mock-Signature", mockNotifySignature(body, secret))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func mockNotifySignature(body string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func assertPaymentDeliveryCounts(t *testing.T, db *gorm.DB, userID string, packageID string, orderID string, wantGrants int64, wantRecords int64) {
	t.Helper()
	var grants int64
	db.Model(&model.MaterialAccessGrant{}).Where("user_id = ? AND package_id = ? AND order_id = ? AND source = ?", userID, packageID, orderID, "order").Count(&grants)
	if grants != wantGrants {
		t.Fatalf("expected %d payment grants, got %d", wantGrants, grants)
	}
	var records int64
	db.Model(&model.PaymentRecord{}).Where("order_id = ? AND provider = ? AND transaction_id = ?", orderID, "wechat_native", "TX_SUCCESS_001").Count(&records)
	if records != wantRecords {
		t.Fatalf("expected %d payment records, got %d", wantRecords, records)
	}
}
