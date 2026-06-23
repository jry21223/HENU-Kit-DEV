package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAdminAccessGrantsCreateListAndRevoke(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	paidMaterial := createTestMaterial(t, db, course.ID, "Manual paid note", model.MaterialAccessPaid, "materials/manual-paid.txt")
	freeMaterial := createTestMaterial(t, db, course.ID, "Free note", model.MaterialAccessFree, "materials/free-note.txt")
	draftMaterial := createTestMaterial(t, db, course.ID, "Draft paid note", model.MaterialAccessPaid, "materials/draft-paid.txt")
	if err := db.Model(&draftMaterial).Update("status", model.StatusDraft).Error; err != nil {
		t.Fatal(err)
	}

	admin := createTestUser(t, db, "grant-admin@stu.henu.edu.cn", model.RoleAdmin)
	target := createTestUser(t, db, "grant-target@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	targetToken := loginTestUser(t, router, target.Email)

	unauthenticated := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","materialId":"`+paidMaterial.ID+`"}`, "")
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated grant 401, got %d: %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	forbidden := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","materialId":"`+paidMaterial.ID+`"}`, targetToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected student grant 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	invalidResource := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","materialId":"`+paidMaterial.ID+`","packageId":"`+course.ID+`"}`, adminToken)
	if invalidResource.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid resource selection 400, got %d: %s", invalidResource.Code, invalidResource.Body.String())
	}
	freeGrant := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","materialId":"`+freeMaterial.ID+`"}`, adminToken)
	if freeGrant.Code != http.StatusBadRequest {
		t.Fatalf("expected free material grant 400, got %d: %s", freeGrant.Code, freeGrant.Body.String())
	}
	draftGrant := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","materialId":"`+draftMaterial.ID+`"}`, adminToken)
	if draftGrant.Code != http.StatusBadRequest {
		t.Fatalf("expected draft material grant 400, got %d: %s", draftGrant.Code, draftGrant.Body.String())
	}

	createGrant := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","materialId":"`+paidMaterial.ID+`"}`, adminToken)
	if createGrant.Code != http.StatusOK {
		t.Fatalf("expected grant create 200, got %d: %s", createGrant.Code, createGrant.Body.String())
	}
	var payload struct {
		Data struct {
			Grant          model.MaterialAccessGrant `json:"grant"`
			AlreadyGranted bool                      `json:"alreadyGranted"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createGrant.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.AlreadyGranted || payload.Data.Grant.ID == "" || payload.Data.Grant.Source != "manual_admin" {
		t.Fatalf("unexpected grant payload: %#v", payload.Data)
	}
	if countOperationLogs(t, db, "access_grant.create", "access_grant", payload.Data.Grant.ID, admin.ID) != 1 {
		t.Fatal("expected access_grant.create operation log")
	}

	duplicateGrant := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","materialId":"`+paidMaterial.ID+`"}`, adminToken)
	if duplicateGrant.Code != http.StatusOK {
		t.Fatalf("expected duplicate grant 200, got %d: %s", duplicateGrant.Code, duplicateGrant.Body.String())
	}
	if !strings.Contains(duplicateGrant.Body.String(), `"alreadyGranted":true`) {
		t.Fatalf("expected duplicate grant to be idempotent, got %s", duplicateGrant.Body.String())
	}
	if countOperationLogs(t, db, "access_grant.create", "access_grant", payload.Data.Grant.ID, admin.ID) != 1 {
		t.Fatal("expected duplicate grant not to write another create log")
	}

	list := performJSON(router, http.MethodGet, "/api/v1/admin/access-grants?userId="+target.ID+"&active=true", "", adminToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected grant list 200, got %d: %s", list.Code, list.Body.String())
	}
	for _, expected := range []string{paidMaterial.ID, target.Email, `"active":true`} {
		if !strings.Contains(list.Body.String(), expected) {
			t.Fatalf("expected grant list to contain %q, got %s", expected, list.Body.String())
		}
	}

	entitlementsBeforeRevoke := performJSON(router, http.MethodGet, "/api/v1/me/entitlements", "", targetToken)
	if entitlementsBeforeRevoke.Code != http.StatusOK || !strings.Contains(entitlementsBeforeRevoke.Body.String(), paidMaterial.ID) {
		t.Fatalf("expected manual grant in user entitlements, got %d: %s", entitlementsBeforeRevoke.Code, entitlementsBeforeRevoke.Body.String())
	}

	revoke := performJSON(router, http.MethodDelete, "/api/v1/admin/access-grants/"+payload.Data.Grant.ID, "", adminToken)
	if revoke.Code != http.StatusOK {
		t.Fatalf("expected grant revoke 200, got %d: %s", revoke.Code, revoke.Body.String())
	}
	if countOperationLogs(t, db, "access_grant.revoke", "access_grant", payload.Data.Grant.ID, admin.ID) != 1 {
		t.Fatal("expected access_grant.revoke operation log")
	}
	entitlementsAfterRevoke := performJSON(router, http.MethodGet, "/api/v1/me/entitlements", "", targetToken)
	if strings.Contains(entitlementsAfterRevoke.Body.String(), paidMaterial.ID) {
		t.Fatalf("expected revoked grant to disappear from entitlements, got %s", entitlementsAfterRevoke.Body.String())
	}
}

func TestAdminAccessGrantForPackageUnlocksPackageMaterials(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	packageMaterial := createTestMaterial(t, db, course.ID, "Package-only paper", model.MaterialAccessPaid, "materials/package-only.txt")
	coursePackage := createTestPackage(t, db, course, "manual-package", model.StatusPublished)
	if err := db.Create(&model.CoursePackageItem{PackageID: coursePackage.ID, ResourceType: "material", ResourceID: packageMaterial.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	draftPackage := createTestPackage(t, db, course, "manual-draft-package", model.StatusDraft)

	admin := createTestUser(t, db, "grant-package-admin@stu.henu.edu.cn", model.RoleAdmin)
	target := createTestUser(t, db, "grant-package-target@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	targetToken := loginTestUser(t, router, target.Email)

	rejected := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","packageId":"`+draftPackage.ID+`"}`, adminToken)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("expected draft package grant 400, got %d: %s", rejected.Code, rejected.Body.String())
	}
	created := performJSON(router, http.MethodPost, "/api/v1/admin/access-grants", `{"userId":"`+target.ID+`","packageId":"`+coursePackage.ID+`"}`, adminToken)
	if created.Code != http.StatusOK {
		t.Fatalf("expected package grant 200, got %d: %s", created.Code, created.Body.String())
	}
	entitlements := performJSON(router, http.MethodGet, "/api/v1/me/entitlements", "", targetToken)
	if entitlements.Code != http.StatusOK {
		t.Fatalf("expected entitlement list 200, got %d: %s", entitlements.Code, entitlements.Body.String())
	}
	for _, expected := range []string{coursePackage.ID, packageMaterial.ID, `"packageGrants":1`, `"unlockedMaterials":1`} {
		if !strings.Contains(entitlements.Body.String(), expected) {
			t.Fatalf("expected package entitlement to contain %q, got %s", expected, entitlements.Body.String())
		}
	}
}
