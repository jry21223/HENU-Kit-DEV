package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestPaymentIncidentCreatedForAmountMismatchAndResolvedByAdmin(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.WeChatPay.APIV3Key = "mock-notify-secret"
	var alertMu sync.Mutex
	var alertBodies []string
	var alertSignature string
	alertServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		alertMu.Lock()
		defer alertMu.Unlock()
		alertBodies = append(alertBodies, string(body))
		alertSignature = r.Header.Get("X-Final-Review-Signature")
		if r.Header.Get("X-Final-Review-Event") != "payment_incident.opened" {
			t.Errorf("unexpected alert event header: %s", r.Header.Get("X-Final-Review-Event"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer alertServer.Close()
	cfg.PaymentIncidentAlerts.WebhookURL = alertServer.URL
	cfg.PaymentIncidentAlerts.WebhookSecret = "incident-alert-secret"
	cfg.PaymentIncidentAlerts.TimeoutSeconds = 2
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "incident-package", model.StatusPublished)
	user := createTestUser(t, db, "incident-buyer@stu.henu.edu.cn", model.RoleUser)
	admin := createTestUser(t, db, "incident-admin@stu.henu.edu.cn", model.RoleAdmin)
	userToken := loginTestUser(t, router, user.Email)
	adminToken := loginTestUser(t, router, admin.Email)
	order := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "INCIDENTORDER001",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaying,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	body := `{"outTradeNo":"INCIDENTORDER001","transactionId":"TX_INCIDENT_001","tradeState":"SUCCESS","amountTotal":1}`
	mismatch := performSignedMockNotify(router, body, cfg.WeChatPay.APIV3Key)
	if mismatch.Code != http.StatusBadRequest || !strings.Contains(mismatch.Body.String(), "amount_mismatch") {
		t.Fatalf("expected amount mismatch rejection, got %d: %s", mismatch.Code, mismatch.Body.String())
	}
	repeated := performSignedMockNotify(router, body, cfg.WeChatPay.APIV3Key)
	if repeated.Code != http.StatusBadRequest || !strings.Contains(repeated.Body.String(), "amount_mismatch") {
		t.Fatalf("expected repeated amount mismatch rejection, got %d: %s", repeated.Code, repeated.Body.String())
	}
	if countPaymentIncidents(t, db, "amount_mismatch") != 1 {
		t.Fatalf("expected idempotent amount mismatch incident, got %d", countPaymentIncidents(t, db, "amount_mismatch"))
	}
	alertMu.Lock()
	alertCount := len(alertBodies)
	alertBody := ""
	if alertCount > 0 {
		alertBody = alertBodies[0]
	}
	signature := alertSignature
	alertMu.Unlock()
	if alertCount != 1 {
		t.Fatalf("expected one webhook alert for idempotent incident creation, got %d", alertCount)
	}
	for _, expected := range []string{`"event":"payment_incident.opened"`, `"incidentType":"amount_mismatch"`, `"expectedAmount":1990`, `"actualAmount":1`} {
		if !strings.Contains(alertBody, expected) {
			t.Fatalf("expected alert body to contain %s, got %s", expected, alertBody)
		}
	}
	if strings.Contains(alertBody, "rawNotify") || signature == "" || !strings.HasPrefix(signature, "sha256=") {
		t.Fatalf("unexpected alert safety fields body=%s signature=%q", alertBody, signature)
	}
	var incident model.PaymentIncident
	if err := db.First(&incident, "incident_type = ?", "amount_mismatch").Error; err != nil {
		t.Fatal(err)
	}
	if incident.OrderID == nil || *incident.OrderID != order.ID || incident.Status != model.PaymentIncidentOpen || incident.ExpectedAmount != 1990 || incident.ActualAmount != 1 {
		t.Fatalf("unexpected payment incident: %#v", incident)
	}

	unauthenticated := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated incident list 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	forbidden := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents", "", userToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected user incident list 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	list := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents?incidentType=amount_mismatch&outTradeNo=INCIDENTORDER", "", adminToken)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), incident.ID) || !strings.Contains(list.Body.String(), `"status":"open"`) || !strings.Contains(list.Body.String(), `"total":1`) {
		t.Fatalf("expected admin incident list to include open incident, got %d: %s", list.Code, list.Body.String())
	}

	resolve := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/resolve", `{"status":"ignored","handleNote":"duplicate test mismatch"}`, adminToken)
	if resolve.Code != http.StatusOK || !strings.Contains(resolve.Body.String(), `"status":"ignored"`) {
		t.Fatalf("expected admin incident resolve 200, got %d: %s", resolve.Code, resolve.Body.String())
	}
	var handled model.PaymentIncident
	if err := db.First(&handled, "id = ?", incident.ID).Error; err != nil {
		t.Fatal(err)
	}
	if handled.Status != model.PaymentIncidentIgnored || handled.HandledBy == nil || *handled.HandledBy != admin.ID || handled.HandledAt == nil {
		t.Fatalf("expected handled incident metadata, got %#v", handled)
	}
	var stored model.Order
	if err := db.First(&stored, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.OrderPaying || stored.PaidAt != nil || stored.RiskFlag != "wechat_amount_mismatch" {
		t.Fatalf("incident handling must not mark order paid or clear risk, got status=%s paidAt=%v risk=%s", stored.Status, stored.PaidAt, stored.RiskFlag)
	}
	if countPackageGrants(t, db, user.ID, coursePackage.ID, order.ID) != 0 {
		t.Fatal("incident handling must not grant package entitlement")
	}
	openAfterResolve := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents?status=open", "", adminToken)
	if openAfterResolve.Code != http.StatusOK || !strings.Contains(openAfterResolve.Body.String(), `"total":0`) {
		t.Fatalf("expected no open incidents after handling, got %d: %s", openAfterResolve.Code, openAfterResolve.Body.String())
	}

	repeatResolve := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/resolve", `{"status":"resolved"}`, adminToken)
	if repeatResolve.Code != http.StatusConflict || !strings.Contains(repeatResolve.Body.String(), "payment_incident_not_open") {
		t.Fatalf("expected repeated incident resolve conflict, got %d: %s", repeatResolve.Code, repeatResolve.Body.String())
	}

	var auditCount int64
	if err := db.Model(&model.OperationLog{}).Where("target_type = ? AND target_id = ? AND action = ?", "payment_incident", incident.ID, "payment_incident.ignored").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one payment incident audit log, got %d", auditCount)
	}
}

func TestPaymentIncidentCreatedForUnknownOrder(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.WeChatPay.APIV3Key = "mock-notify-secret"
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	body := `{"outTradeNo":"MISSING_INCIDENT_ORDER","transactionId":"TX_INCIDENT_MISSING","tradeState":"SUCCESS","amountTotal":1990}`
	response := performSignedMockNotify(router, body, cfg.WeChatPay.APIV3Key)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "order_not_found") {
		t.Fatalf("expected unknown order rejection, got %d: %s", response.Code, response.Body.String())
	}
	var incident model.PaymentIncident
	if err := db.First(&incident, "incident_type = ?", "order_not_found").Error; err != nil {
		t.Fatal(err)
	}
	if incident.OrderID != nil || incident.OutTradeNo != "MISSING_INCIDENT_ORDER" || incident.ActualAmount != 1990 {
		t.Fatalf("unexpected unknown-order incident: %#v", incident)
	}
}

func TestPaymentIncidentCreatedForTransactionConflict(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.WeChatPay.APIV3Key = "mock-notify-secret"
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "incident-conflict-package", model.StatusPublished)
	user := createTestUser(t, db, "incident-conflict@stu.henu.edu.cn", model.RoleUser)
	firstOrder := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "INCIDENTCONFLICT001",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaying,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	secondOrder := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "INCIDENTCONFLICT002",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaying,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&firstOrder).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&secondOrder).Error; err != nil {
		t.Fatal(err)
	}

	successBody := `{"outTradeNo":"INCIDENTCONFLICT001","transactionId":"TX_INCIDENT_CONFLICT","tradeState":"SUCCESS","amountTotal":1990}`
	success := performSignedMockNotify(router, successBody, cfg.WeChatPay.APIV3Key)
	if success.Code != http.StatusOK {
		t.Fatalf("expected first notify success, got %d: %s", success.Code, success.Body.String())
	}
	conflictBody := `{"outTradeNo":"INCIDENTCONFLICT002","transactionId":"TX_INCIDENT_CONFLICT","tradeState":"SUCCESS","amountTotal":1990}`
	conflict := performSignedMockNotify(router, conflictBody, cfg.WeChatPay.APIV3Key)
	if conflict.Code != http.StatusBadRequest || !strings.Contains(conflict.Body.String(), "payment_record_conflict") {
		t.Fatalf("expected transaction conflict rejection, got %d: %s", conflict.Code, conflict.Body.String())
	}
	var incident model.PaymentIncident
	if err := db.First(&incident, "incident_type = ?", "transaction_conflict").Error; err != nil {
		t.Fatal(err)
	}
	if incident.OrderID == nil || *incident.OrderID != secondOrder.ID || incident.TransactionID != "TX_INCIDENT_CONFLICT" {
		t.Fatalf("unexpected transaction-conflict incident: %#v", incident)
	}
	var secondStored model.Order
	if err := db.First(&secondStored, "id = ?", secondOrder.ID).Error; err != nil {
		t.Fatal(err)
	}
	if secondStored.Status != model.OrderPaying || secondStored.PaidAt != nil || secondStored.RiskFlag != "wechat_transaction_conflict" {
		t.Fatalf("conflict must not mark second order paid, got status=%s paidAt=%v risk=%s", secondStored.Status, secondStored.PaidAt, secondStored.RiskFlag)
	}
	if countPackageGrants(t, db, user.ID, coursePackage.ID, secondOrder.ID) != 0 {
		t.Fatal("conflicting transaction must not grant second order entitlement")
	}
}

func countPaymentIncidents(t *testing.T, db *gorm.DB, incidentType string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.PaymentIncident{}).Where("incident_type = ?", incidentType).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func countPackageGrants(t *testing.T, db *gorm.DB, userID string, packageID string, orderID string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.MaterialAccessGrant{}).
		Where("user_id = ? AND package_id = ? AND order_id = ? AND source = ?", userID, packageID, orderID, "order").
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}
