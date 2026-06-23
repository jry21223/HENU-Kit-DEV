package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestCoursePackageOrderCreateAndStatus(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	coursePackage := createTestPackage(t, db, course, "order-package", model.StatusPublished)
	if err := db.Model(&coursePackage).Updates(map[string]interface{}{"price_fen": int64(2990), "currency": "CNY"}).Error; err != nil {
		t.Fatal(err)
	}
	user := createTestUser(t, db, "buyer@stu.henu.edu.cn", model.RoleUser)
	other := createTestUser(t, db, "other-buyer@stu.henu.edu.cn", model.RoleUser)
	admin := createTestUser(t, db, "order-admin@stu.henu.edu.cn", model.RoleAdmin)
	userToken := loginTestUser(t, router, user.Email)
	otherToken := loginTestUser(t, router, other.Email)
	adminToken := loginTestUser(t, router, admin.Email)

	body := `{"packageId":"` + coursePackage.ID + `","amountTotal":1,"paymentProvider":"fake"}`
	unauthorized := performJSON(router, http.MethodPost, "/api/v1/orders", body, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated order create 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	created := performJSON(router, http.MethodPost, "/api/v1/orders", body, userToken)
	if created.Code != http.StatusOK {
		t.Fatalf("expected order create 200, got %d: %s", created.Code, created.Body.String())
	}
	var payload struct {
		Data struct {
			Order              model.Order         `json:"order"`
			Package            model.CoursePackage `json:"package"`
			AlreadyOwned       bool                `json:"alreadyOwned"`
			AlreadyPending     bool                `json:"alreadyPending"`
			EntitlementGranted bool                `json:"entitlementGranted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	order := payload.Data.Order
	if order.ID == "" || order.Status != model.OrderPending || order.ProductType != "course_package" || order.ProductID != coursePackage.ID {
		t.Fatalf("unexpected order payload: %#v", order)
	}
	if order.AmountTotal != 2990 || order.Currency != "CNY" || order.PaymentProvider != "wechat_native" {
		t.Fatalf("expected server package price/provider, got amount=%d currency=%s provider=%s", order.AmountTotal, order.Currency, order.PaymentProvider)
	}
	if payload.Data.AlreadyOwned || payload.Data.AlreadyPending || payload.Data.EntitlementGranted {
		t.Fatalf("unexpected create flags: %#v", payload.Data)
	}
	if countOrders(t, db, user.ID, coursePackage.ID) != 1 {
		t.Fatal("expected one order after first create")
	}

	duplicate := performJSON(router, http.MethodPost, "/api/v1/orders", body, userToken)
	if duplicate.Code != http.StatusOK {
		t.Fatalf("expected duplicate pending order 200, got %d: %s", duplicate.Code, duplicate.Body.String())
	}
	if !strings.Contains(duplicate.Body.String(), `"alreadyPending":true`) || !strings.Contains(duplicate.Body.String(), order.ID) {
		t.Fatalf("expected duplicate create to return existing pending order, got %s", duplicate.Body.String())
	}
	if countOrders(t, db, user.ID, coursePackage.ID) != 1 {
		t.Fatal("expected duplicate pending order not to create another row")
	}

	status := performJSON(router, http.MethodGet, "/api/v1/orders/"+order.ID+"/status", "", userToken)
	if status.Code != http.StatusOK {
		t.Fatalf("expected order status 200, got %d: %s", status.Code, status.Body.String())
	}
	for _, expected := range []string{order.ID, `"status":"pending"`, `"entitlementGranted":false`, `"amountTotal":2990`, `"paymentProvider":"wechat_native"`} {
		if !strings.Contains(status.Body.String(), expected) {
			t.Fatalf("expected status response to contain %q, got %s", expected, status.Body.String())
		}
	}
	otherDenied := performJSON(router, http.MethodGet, "/api/v1/orders/"+order.ID+"/status", "", otherToken)
	if otherDenied.Code != http.StatusForbidden {
		t.Fatalf("expected other user order status 403, got %d: %s", otherDenied.Code, otherDenied.Body.String())
	}
	adminAllowed := performJSON(router, http.MethodGet, "/api/v1/orders/"+order.ID+"/status", "", adminToken)
	if adminAllowed.Code != http.StatusOK {
		t.Fatalf("expected admin order status 200, got %d: %s", adminAllowed.Code, adminAllowed.Body.String())
	}
}

func TestCoursePackageOrderRejectsDraftAndSkipsOwnedPackage(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	draftPackage := createTestPackage(t, db, course, "draft-order-package", model.StatusDraft)
	publishedPackage := createTestPackage(t, db, course, "owned-order-package", model.StatusPublished)
	user := createTestUser(t, db, "owned-buyer@stu.henu.edu.cn", model.RoleUser)
	token := loginTestUser(t, router, user.Email)

	draft := performJSON(router, http.MethodPost, "/api/v1/orders", `{"packageId":"`+draftPackage.ID+`"}`, token)
	if draft.Code != http.StatusNotFound {
		t.Fatalf("expected draft package order 404, got %d: %s", draft.Code, draft.Body.String())
	}

	grant := model.MaterialAccessGrant{UserID: user.ID, PackageID: &publishedPackage.ID, Source: "test_owned"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	owned := performJSON(router, http.MethodPost, "/api/v1/orders", `{"packageId":"`+publishedPackage.ID+`"}`, token)
	if owned.Code != http.StatusOK {
		t.Fatalf("expected owned package order request 200, got %d: %s", owned.Code, owned.Body.String())
	}
	for _, expected := range []string{`"alreadyOwned":true`, `"entitlementGranted":true`, publishedPackage.ID} {
		if !strings.Contains(owned.Body.String(), expected) {
			t.Fatalf("expected owned response to contain %q, got %s", expected, owned.Body.String())
		}
	}
	if countOrders(t, db, user.ID, publishedPackage.ID) != 0 {
		t.Fatal("expected already-owned package not to create an order")
	}
}

func countOrders(t *testing.T, db *gorm.DB, userID string, packageID string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.Order{}).
		Where("user_id = ? AND product_type = ? AND product_id = ?", userID, "course_package", packageID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}
