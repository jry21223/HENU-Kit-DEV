package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAdminPaymentReconciliationReportsLocalAnomalies(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "reconciliation-package", model.StatusPublished)
	user := createTestUser(t, db, "reconciliation-buyer@stu.henu.edu.cn", model.RoleUser)
	admin := createTestUser(t, db, "reconciliation-admin@stu.henu.edu.cn", model.RoleAdmin)
	userToken := loginTestUser(t, router, user.Email)
	adminToken := loginTestUser(t, router, admin.Email)

	cleanPaidOrder := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRRECONCILE_CLEAN",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaid,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&cleanPaidOrder).Error; err != nil {
		t.Fatal(err)
	}
	cleanGrant := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "order", OrderID: &cleanPaidOrder.ID}
	if err := db.Create(&cleanGrant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PaymentRecord{OrderID: cleanPaidOrder.ID, Provider: "wechat_native", TransactionID: "TX_RECON_CLEAN", TradeState: "SUCCESS", AmountTotal: 1990, IdempotencyKey: "reconcile-clean"}).Error; err != nil {
		t.Fatal(err)
	}

	paidMissingRecord := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRRECONCILE_MISSING_RECORD",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaid,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&paidMissingRecord).Error; err != nil {
		t.Fatal(err)
	}

	paidAmountMismatch := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRRECONCILE_AMOUNT",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaid,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&paidAmountMismatch).Error; err != nil {
		t.Fatal(err)
	}
	mismatchGrant := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "order", OrderID: &paidAmountMismatch.ID}
	if err := db.Create(&mismatchGrant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PaymentRecord{OrderID: paidAmountMismatch.ID, Provider: "wechat_native", TransactionID: "TX_RECON_AMOUNT", TradeState: "SUCCESS", AmountTotal: 1, IdempotencyKey: "reconcile-amount"}).Error; err != nil {
		t.Fatal(err)
	}

	unpaidWithGrant := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRRECONCILE_UNPAID_GRANT",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaying,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&unpaidWithGrant).Error; err != nil {
		t.Fatal(err)
	}
	unsafeGrant := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "order", OrderID: &unpaidWithGrant.ID}
	if err := db.Create(&unsafeGrant).Error; err != nil {
		t.Fatal(err)
	}

	riskyOrder := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRRECONCILE_RISK",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaying,
		AmountTotal:     1990,
		Currency:        "CNY",
		RiskFlag:        "wechat_amount_mismatch",
	}
	if err := db.Create(&riskyOrder).Error; err != nil {
		t.Fatal(err)
	}

	duplicateOne := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRRECONCILE_DUP_ONE",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaid,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	duplicateTwo := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRRECONCILE_DUP_TWO",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPaid,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&duplicateOne).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&duplicateTwo).Error; err != nil {
		t.Fatal(err)
	}
	dupGrantOne := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "order", OrderID: &duplicateOne.ID}
	dupGrantTwo := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "order", OrderID: &duplicateTwo.ID}
	if err := db.Create(&dupGrantOne).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&dupGrantTwo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PaymentRecord{OrderID: duplicateOne.ID, Provider: "wechat_native", TransactionID: "TX_RECON_DUP", TradeState: "SUCCESS", AmountTotal: 1990, IdempotencyKey: "reconcile-dup-one"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PaymentRecord{OrderID: duplicateTwo.ID, Provider: "wechat_native", TransactionID: "TX_RECON_DUP", TradeState: "SUCCESS", AmountTotal: 1990, IdempotencyKey: "reconcile-dup-two"}).Error; err != nil {
		t.Fatal(err)
	}

	incident := model.PaymentIncident{
		OrderID:        &riskyOrder.ID,
		Provider:       "wechat_native",
		IncidentType:   "amount_mismatch",
		Severity:       "high",
		Status:         model.PaymentIncidentOpen,
		OutTradeNo:     riskyOrder.OutTradeNo,
		TransactionID:  "TX_RECON_INCIDENT",
		ExpectedAmount: 1990,
		ActualAmount:   1,
		Message:        "callback amount does not match local order amount",
	}
	if err := db.Create(&incident).Error; err != nil {
		t.Fatal(err)
	}

	unauthenticated := performJSON(router, http.MethodGet, "/api/v1/admin/payment-reconciliation", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated reconciliation 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	forbidden := performJSON(router, http.MethodGet, "/api/v1/admin/payment-reconciliation", "", userToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected user reconciliation 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	invalidSeverity := performJSON(router, http.MethodGet, "/api/v1/admin/payment-reconciliation?severity=urgent", "", adminToken)
	if invalidSeverity.Code != http.StatusBadRequest || !strings.Contains(invalidSeverity.Body.String(), "invalid_severity") {
		t.Fatalf("expected invalid severity rejection, got %d: %s", invalidSeverity.Code, invalidSeverity.Body.String())
	}

	report := performJSON(router, http.MethodGet, "/api/v1/admin/payment-reconciliation", "", adminToken)
	if report.Code != http.StatusOK {
		t.Fatalf("expected reconciliation report 200, got %d: %s", report.Code, report.Body.String())
	}
	body := report.Body.String()
	for _, expected := range []string{
		"paid_order_missing_payment_record",
		"paid_order_missing_entitlement",
		"paid_order_amount_record_mismatch",
		"unpaid_order_has_entitlement",
		"order_risk_flag",
		"duplicate_transaction_id",
		"open_payment_incident",
		`"critical"`,
		`"high"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected reconciliation report to include %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, cleanPaidOrder.OutTradeNo) {
		t.Fatalf("clean paid order must not be reported, got %s", body)
	}

	filtered := performJSON(router, http.MethodGet, "/api/v1/admin/payment-reconciliation?issueType=unpaid_order_has_entitlement&severity=critical", "", adminToken)
	if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), unpaidWithGrant.OutTradeNo) || strings.Contains(filtered.Body.String(), paidMissingRecord.OutTradeNo) {
		t.Fatalf("expected filtered reconciliation report to isolate unpaid grant issue, got %d: %s", filtered.Code, filtered.Body.String())
	}
}
