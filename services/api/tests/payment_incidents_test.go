package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	if strings.Contains(list.Body.String(), "rawNotify") || strings.Contains(list.Body.String(), "idempotencyKey") {
		t.Fatalf("admin incident list must not expose raw notify or idempotency key, got %s", list.Body.String())
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

func TestPaymentIncidentManualAlertIsAdminOnlyAndNonFinancial(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.PaymentIncidentAlerts.WebhookSecret = "incident-alert-secret"
	cfg.PaymentIncidentAlerts.TimeoutSeconds = 2
	var alertMu sync.Mutex
	var alertBodies []string
	var alertEvents []string
	var alertSignatures []string
	alertServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		alertMu.Lock()
		defer alertMu.Unlock()
		alertBodies = append(alertBodies, string(body))
		alertEvents = append(alertEvents, r.Header.Get("X-Final-Review-Event"))
		alertSignatures = append(alertSignatures, r.Header.Get("X-Final-Review-Signature"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer alertServer.Close()
	cfg.PaymentIncidentAlerts.WebhookURL = alertServer.URL
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "incident-alert-package", model.StatusPublished)
	buyer := createTestUser(t, db, "incident-alert-buyer@stu.henu.edu.cn", model.RoleUser)
	admin := createTestUser(t, db, "incident-alert-admin@stu.henu.edu.cn", model.RoleAdmin)
	regular := createTestUser(t, db, "incident-alert-user@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	userToken := loginTestUser(t, router, regular.Email)
	order := model.Order{
		UserID:          buyer.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "INCIDENTALERT001",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaying,
		AmountTotal:     1990,
		Currency:        "CNY",
		RiskFlag:        "wechat_amount_mismatch",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	incident := model.PaymentIncident{
		OrderID:        &order.ID,
		Provider:       "wechat_native",
		IncidentType:   "amount_mismatch",
		Severity:       "critical",
		Status:         model.PaymentIncidentOpen,
		OutTradeNo:     order.OutTradeNo,
		TransactionID:  "TX_INCIDENT_ALERT",
		TradeState:     "SUCCESS",
		ExpectedAmount: 1990,
		ActualAmount:   1,
		Message:        "amount mismatch",
		IdempotencyKey: "incident-alert-open",
	}
	if err := db.Create(&incident).Error; err != nil {
		t.Fatal(err)
	}

	unauthenticated := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/alert", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated alert 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	forbidden := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/alert", "", userToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected regular user alert 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	alert := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/alert", "", adminToken)
	if alert.Code != http.StatusOK || !strings.Contains(alert.Body.String(), `"alertSent":true`) || !strings.Contains(alert.Body.String(), `"statusCode":204`) {
		t.Fatalf("expected admin alert success, got %d: %s", alert.Code, alert.Body.String())
	}

	alertMu.Lock()
	alertCount := len(alertBodies)
	alertBody := ""
	alertEvent := ""
	alertSignature := ""
	if alertCount > 0 {
		alertBody = alertBodies[0]
		alertEvent = alertEvents[0]
		alertSignature = alertSignatures[0]
	}
	alertMu.Unlock()
	if alertCount != 1 || alertEvent != "payment_incident.realerted" {
		t.Fatalf("expected one realert webhook, count=%d event=%s body=%s", alertCount, alertEvent, alertBody)
	}
	for _, expected := range []string{`"event":"payment_incident.realerted"`, `"incidentType":"amount_mismatch"`, `"status":"open"`, `"expectedAmount":1990`, `"actualAmount":1`} {
		if !strings.Contains(alertBody, expected) {
			t.Fatalf("expected alert body to contain %s, got %s", expected, alertBody)
		}
	}
	if strings.Contains(alertBody, "rawNotify") || alertSignature == "" || !strings.HasPrefix(alertSignature, "sha256=") {
		t.Fatalf("unexpected alert safety fields body=%s signature=%q", alertBody, alertSignature)
	}

	var storedIncident model.PaymentIncident
	if err := db.First(&storedIncident, "id = ?", incident.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedIncident.Status != model.PaymentIncidentOpen || storedIncident.HandledAt != nil || storedIncident.HandledBy != nil {
		t.Fatalf("manual alert must not handle incident, got %#v", storedIncident)
	}
	var storedOrder model.Order
	if err := db.First(&storedOrder, "id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedOrder.Status != model.OrderPaying || storedOrder.PaidAt != nil || storedOrder.RiskFlag != "wechat_amount_mismatch" {
		t.Fatalf("manual alert must not mark order paid or clear risk, got status=%s paidAt=%v risk=%s", storedOrder.Status, storedOrder.PaidAt, storedOrder.RiskFlag)
	}
	if countPackageGrants(t, db, buyer.ID, coursePackage.ID, order.ID) != 0 {
		t.Fatal("manual alert must not grant package entitlement")
	}
	var auditCount int64
	if err := db.Model(&model.OperationLog{}).Where("target_type = ? AND target_id = ? AND action = ?", "payment_incident", incident.ID, "payment_incident.alert").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("expected one manual alert operation log, got %d", auditCount)
	}

	resolve := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/resolve", `{"status":"ignored"}`, adminToken)
	if resolve.Code != http.StatusOK {
		t.Fatalf("expected resolve before closed alert check, got %d: %s", resolve.Code, resolve.Body.String())
	}
	closedAlert := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/alert", "", adminToken)
	if closedAlert.Code != http.StatusConflict || !strings.Contains(closedAlert.Body.String(), "payment_incident_not_open") {
		t.Fatalf("expected handled incident alert conflict, got %d: %s", closedAlert.Code, closedAlert.Body.String())
	}
	alertMu.Lock()
	alertCount = len(alertBodies)
	alertMu.Unlock()
	if alertCount != 1 {
		t.Fatalf("handled incident alert must not send another webhook, got %d", alertCount)
	}
}

func TestPaymentIncidentManualAlertRequiresConfiguredWebhook(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "incident-alert-config-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, admin.Email)
	incident := model.PaymentIncident{
		Provider:       "wechat_native",
		IncidentType:   "order_not_found",
		Severity:       "high",
		Status:         model.PaymentIncidentOpen,
		OutTradeNo:     "INCIDENT_ALERT_NO_WEBHOOK",
		ActualAmount:   1990,
		Message:        "missing local order",
		IdempotencyKey: "incident-alert-no-webhook",
	}
	if err := db.Create(&incident).Error; err != nil {
		t.Fatal(err)
	}

	alert := performJSON(router, http.MethodPost, "/api/v1/admin/payment-incidents/"+incident.ID+"/alert", "", adminToken)
	if alert.Code != http.StatusConflict || !strings.Contains(alert.Body.String(), "payment_incident_webhook_not_configured") {
		t.Fatalf("expected missing webhook conflict, got %d: %s", alert.Code, alert.Body.String())
	}
	var auditCount int64
	if err := db.Model(&model.OperationLog{}).Where("target_type = ? AND target_id = ? AND action = ?", "payment_incident", incident.ID, "payment_incident.alert").Count(&auditCount).Error; err != nil {
		t.Fatal(err)
	}
	if auditCount != 0 {
		t.Fatalf("failed alert must not write successful operation log, got %d", auditCount)
	}
}

func TestPaymentIncidentListFiltersSeverityAndOverdueReadOnly(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.PaymentIncidentAlerts.OverdueMinutes = 30
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "incident-filter-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, admin.Email)
	incidents := []model.PaymentIncident{
		{
			BaseModel:      model.BaseModel{CreatedAt: time.Now().Add(-2 * time.Hour)},
			Provider:       "wechat_native",
			IncidentType:   "amount_mismatch",
			Severity:       "high",
			Status:         model.PaymentIncidentOpen,
			OutTradeNo:     "FILTER_OVERDUE_HIGH",
			Message:        "overdue high",
			IdempotencyKey: "filter-overdue-high",
		},
		{
			Provider:       "wechat_native",
			IncidentType:   "transaction_conflict",
			Severity:       "high",
			Status:         model.PaymentIncidentOpen,
			OutTradeNo:     "FILTER_FRESH_HIGH",
			Message:        "fresh high",
			IdempotencyKey: "filter-fresh-high",
		},
		{
			BaseModel:      model.BaseModel{CreatedAt: time.Now().Add(-2 * time.Hour)},
			Provider:       "wechat_native",
			IncidentType:   "order_not_found",
			Severity:       "low",
			Status:         model.PaymentIncidentOpen,
			OutTradeNo:     "FILTER_OVERDUE_LOW",
			Message:        "overdue low",
			IdempotencyKey: "filter-overdue-low",
		},
		{
			BaseModel:      model.BaseModel{CreatedAt: time.Now().Add(-2 * time.Hour)},
			Provider:       "wechat_native",
			IncidentType:   "amount_mismatch",
			Severity:       "high",
			Status:         model.PaymentIncidentResolved,
			OutTradeNo:     "FILTER_RESOLVED_HIGH",
			Message:        "resolved high",
			IdempotencyKey: "filter-resolved-high",
		},
	}
	if err := db.Create(&incidents).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.PaymentIncident{}).
		Where("out_trade_no IN ?", []string{"FILTER_OVERDUE_HIGH", "FILTER_OVERDUE_LOW", "FILTER_RESOLVED_HIGH"}).
		Update("created_at", time.Now().UTC().Add(-2*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	filtered := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents?status=all&severity=high&overdue=true", "", adminToken)
	if filtered.Code != http.StatusOK {
		t.Fatalf("expected filtered incident list 200, got %d: %s", filtered.Code, filtered.Body.String())
	}
	body := filtered.Body.String()
	for _, expected := range []string{`"total":1`, "FILTER_OVERDUE_HIGH"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected filtered body to contain %s, got %s", expected, body)
		}
	}
	for _, unexpected := range []string{"FILTER_FRESH_HIGH", "FILTER_OVERDUE_LOW", "FILTER_RESOLVED_HIGH"} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("filtered body should not contain %s, got %s", unexpected, body)
		}
	}

	invalidSeverity := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents?severity=urgent", "", adminToken)
	if invalidSeverity.Code != http.StatusBadRequest || !strings.Contains(invalidSeverity.Body.String(), "invalid_severity") {
		t.Fatalf("expected invalid severity rejection, got %d: %s", invalidSeverity.Code, invalidSeverity.Body.String())
	}
	invalidOverdue := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents?overdue=maybe", "", adminToken)
	if invalidOverdue.Code != http.StatusBadRequest || !strings.Contains(invalidOverdue.Body.String(), "invalid_overdue") {
		t.Fatalf("expected invalid overdue rejection, got %d: %s", invalidOverdue.Code, invalidOverdue.Body.String())
	}
	invalidStatus := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents?status=resolved&overdue=true", "", adminToken)
	if invalidStatus.Code != http.StatusBadRequest || !strings.Contains(invalidStatus.Body.String(), "invalid_overdue_status") {
		t.Fatalf("expected invalid overdue status rejection, got %d: %s", invalidStatus.Code, invalidStatus.Body.String())
	}
	var storedCount int64
	if err := db.Model(&model.PaymentIncident{}).Count(&storedCount).Error; err != nil {
		t.Fatal(err)
	}
	if storedCount != int64(len(incidents)) {
		t.Fatalf("filter endpoint must be read-only, expected %d incidents, got %d", len(incidents), storedCount)
	}
}

func TestPaymentIncidentSummaryIsAdminOnlyAndReadOnly(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.PaymentIncidentAlerts.OverdueMinutes = 60
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "incident-summary-admin@stu.henu.edu.cn", model.RoleAdmin)
	user := createTestUser(t, db, "incident-summary-user@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	userToken := loginTestUser(t, router, user.Email)

	incidents := []model.PaymentIncident{
		{
			BaseModel:      model.BaseModel{CreatedAt: time.Now().Add(-2 * time.Hour)},
			Provider:       "wechat_native",
			IncidentType:   "amount_mismatch",
			Severity:       "critical",
			Status:         model.PaymentIncidentOpen,
			OutTradeNo:     "SUMMARY_OPEN_CRITICAL",
			ExpectedAmount: 1990,
			ActualAmount:   1,
			Message:        "amount mismatch",
			IdempotencyKey: "summary-open-critical",
		},
		{
			Provider:       "wechat_native",
			IncidentType:   "transaction_conflict",
			Severity:       "high",
			Status:         model.PaymentIncidentOpen,
			OutTradeNo:     "SUMMARY_OPEN_HIGH",
			TransactionID:  "TX_SUMMARY_CONFLICT",
			ExpectedAmount: 1990,
			ActualAmount:   1990,
			Message:        "transaction conflict",
			IdempotencyKey: "summary-open-high",
		},
		{
			BaseModel:      model.BaseModel{CreatedAt: time.Now().Add(-2 * time.Hour)},
			Provider:       "wechat_native",
			IncidentType:   "amount_mismatch",
			Severity:       "high",
			Status:         model.PaymentIncidentResolved,
			OutTradeNo:     "SUMMARY_RESOLVED",
			ExpectedAmount: 1990,
			ActualAmount:   1,
			Message:        "resolved mismatch",
			IdempotencyKey: "summary-resolved",
		},
		{
			Provider:       "wechat_native",
			IncidentType:   "order_not_found",
			Severity:       "medium",
			Status:         model.PaymentIncidentIgnored,
			OutTradeNo:     "SUMMARY_IGNORED",
			ExpectedAmount: 0,
			ActualAmount:   1990,
			Message:        "ignored missing order",
			IdempotencyKey: "summary-ignored",
		},
	}
	if err := db.Create(&incidents).Error; err != nil {
		t.Fatal(err)
	}

	unauthenticated := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents/summary", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated incident summary 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	forbidden := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents/summary", "", userToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected user incident summary 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}

	summary := performJSON(router, http.MethodGet, "/api/v1/admin/payment-incidents/summary", "", adminToken)
	body := summary.Body.String()
	if summary.Code != http.StatusOK {
		t.Fatalf("expected admin incident summary 200, got %d: %s", summary.Code, body)
	}
	for _, expected := range []string{
		`"total":4`,
		`"open":2`,
		`"resolved":1`,
		`"ignored":1`,
		`"overdueOpen":1`,
		`"overdueThresholdMinutes":60`,
		`"openCritical":1`,
		`"openHigh":1`,
		`"amount_mismatch":1`,
		`"transaction_conflict":1`,
		`"oldestOpenAt"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected summary body to contain %s, got %s", expected, body)
		}
	}
	if strings.Contains(body, `"order_not_found":1`) {
		t.Fatalf("ignored incidents must not be counted in openByType, got %s", body)
	}

	var storedCount int64
	if err := db.Model(&model.PaymentIncident{}).Count(&storedCount).Error; err != nil {
		t.Fatal(err)
	}
	if storedCount != int64(len(incidents)) {
		t.Fatalf("summary endpoint must be read-only, expected %d incidents, got %d", len(incidents), storedCount)
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
