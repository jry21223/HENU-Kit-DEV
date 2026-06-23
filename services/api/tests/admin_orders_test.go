package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAdminOrdersListIsAdminOnlyAndFilterable(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "admin-order-package", model.StatusPublished)
	user := createTestUser(t, db, "buyer-admin-orders@stu.henu.edu.cn", model.RoleUser)
	admin := createTestUser(t, db, "orders-admin@stu.henu.edu.cn", model.RoleAdmin)
	userToken := loginTestUser(t, router, user.Email)
	adminToken := loginTestUser(t, router, admin.Email)

	order := model.Order{
		UserID:          user.ID,
		ProductType:     "course_package",
		ProductID:       coursePackage.ID,
		OutTradeNo:      "FRADMINORDER001",
		PaymentProvider: "wechat_native",
		Status:          model.OrderPending,
		AmountTotal:     1990,
		Currency:        "CNY",
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	grant := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "manual_test", OrderID: &order.ID}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}

	unauthenticated := performJSON(router, http.MethodGet, "/api/v1/admin/orders", "", "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated admin order list 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	forbidden := performJSON(router, http.MethodGet, "/api/v1/admin/orders", "", userToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected user admin order list 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	invalidStatus := performJSON(router, http.MethodGet, "/api/v1/admin/orders?status=settled", "", adminToken)
	if invalidStatus.Code != http.StatusBadRequest || !strings.Contains(invalidStatus.Body.String(), "invalid_status") {
		t.Fatalf("expected invalid status rejection, got %d: %s", invalidStatus.Code, invalidStatus.Body.String())
	}

	list := performJSON(router, http.MethodGet, "/api/v1/admin/orders?status=pending&userEmail=buyer-admin-orders&outTradeNo=ADMINORDER", "", adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected admin order list 200, got %d: %s", list.Code, list.Body.String())
	}
	for _, expected := range []string{
		order.ID,
		"FRADMINORDER001",
		user.Email,
		coursePackage.ID,
		`"paymentProvider":"wechat_native"`,
		`"entitlementGranted":true`,
	} {
		if !strings.Contains(list.Body.String(), expected) {
			t.Fatalf("expected admin order list to contain %q, got %s", expected, list.Body.String())
		}
	}

	noMatch := performJSON(router, http.MethodGet, "/api/v1/admin/orders?userEmail=missing-order-user", "", adminToken)
	if noMatch.Code != http.StatusOK || strings.Contains(noMatch.Body.String(), order.ID) {
		t.Fatalf("expected empty list for missing email, got %d: %s", noMatch.Code, noMatch.Body.String())
	}
}
