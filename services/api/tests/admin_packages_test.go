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

func TestAdminCoursePackagesCreateUpdateItemsAndArchive(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	material := createTestMaterial(t, db, course.ID, "Package published material", model.MaterialAccessPaid, "materials/package-published.txt")
	draftMaterial := createTestMaterial(t, db, course.ID, "Package draft material", model.MaterialAccessPaid, "materials/package-draft.txt")
	if err := db.Model(&draftMaterial).Update("status", model.StatusDraft).Error; err != nil {
		t.Fatal(err)
	}

	admin := createTestUser(t, db, "package-admin@stu.henu.edu.cn", model.RoleAdmin)
	user := createTestUser(t, db, "package-user@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	userToken := loginTestUser(t, router, user.Email)

	createBody := `{"schoolId":"` + course.SchoolID + `","collegeId":"` + course.CollegeID + `","majorId":"` + course.MajorID + `","courseId":"` + course.ID + `","grade":"` + course.Grade + `","title":"Discrete Math Package","slug":"admin-discrete-package","description":"Package","priceFen":1990,"currency":"CNY"}`
	unauthorized := performJSON(router, http.MethodPost, "/api/v1/admin/packages", createBody, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated package create 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	forbidden := performJSON(router, http.MethodPost, "/api/v1/admin/packages", createBody, userToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected student package create 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
	invalidStatus := performJSON(router, http.MethodPost, "/api/v1/admin/packages", strings.Replace(createBody, `"currency":"CNY"`, `"currency":"CNY","status":"pending"`, 1), adminToken)
	if invalidStatus.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid package status 400, got %d: %s", invalidStatus.Code, invalidStatus.Body.String())
	}
	created := performJSON(router, http.MethodPost, "/api/v1/admin/packages", createBody, adminToken)
	if created.Code != http.StatusOK {
		t.Fatalf("expected package create 200, got %d: %s", created.Code, created.Body.String())
	}
	var createPayload struct {
		Data struct {
			Package model.CoursePackage `json:"package"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createPayload); err != nil {
		t.Fatal(err)
	}
	coursePackage := createPayload.Data.Package
	if coursePackage.ID == "" || coursePackage.Status != model.StatusDraft || coursePackage.PriceFen != 1990 {
		t.Fatalf("unexpected created package: %#v", coursePackage)
	}
	if countOperationLogs(t, db, "course_package.create", "course_package", coursePackage.ID, admin.ID) != 1 {
		t.Fatal("expected course_package.create operation log")
	}

	publicDraftList := performJSON(router, http.MethodGet, "/api/v1/packages?courseId="+course.ID, "", "")
	if strings.Contains(publicDraftList.Body.String(), coursePackage.ID) {
		t.Fatalf("public package list exposed draft package: %s", publicDraftList.Body.String())
	}
	adminList := performJSON(router, http.MethodGet, "/api/v1/admin/packages?status=draft", "", adminToken)
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), coursePackage.ID) {
		t.Fatalf("expected admin package list to include draft package, got %d: %s", adminList.Code, adminList.Body.String())
	}

	addItem := performJSON(router, http.MethodPost, "/api/v1/admin/packages/"+coursePackage.ID+"/items", `{"resourceType":"material","resourceId":"`+material.ID+`","sortOrder":1}`, adminToken)
	if addItem.Code != http.StatusOK {
		t.Fatalf("expected package item create 200, got %d: %s", addItem.Code, addItem.Body.String())
	}
	var itemPayload struct {
		Data struct {
			Item          model.CoursePackageItem `json:"item"`
			AlreadyExists bool                    `json:"alreadyExists"`
		} `json:"data"`
	}
	if err := json.Unmarshal(addItem.Body.Bytes(), &itemPayload); err != nil {
		t.Fatal(err)
	}
	if itemPayload.Data.AlreadyExists || itemPayload.Data.Item.ID == "" {
		t.Fatalf("unexpected package item payload: %#v", itemPayload.Data)
	}
	if countOperationLogs(t, db, "course_package_item.create", "course_package_item", itemPayload.Data.Item.ID, admin.ID) != 1 {
		t.Fatal("expected course_package_item.create operation log")
	}
	duplicateItem := performJSON(router, http.MethodPost, "/api/v1/admin/packages/"+coursePackage.ID+"/items", `{"resourceType":"material","resourceId":"`+material.ID+`","sortOrder":2}`, adminToken)
	if duplicateItem.Code != http.StatusOK || !strings.Contains(duplicateItem.Body.String(), `"alreadyExists":true`) {
		t.Fatalf("expected duplicate package item to be idempotent, got %d: %s", duplicateItem.Code, duplicateItem.Body.String())
	}
	if countPackageItems(t, db, coursePackage.ID, material.ID) != 1 {
		t.Fatal("expected duplicate package item not to create another row")
	}
	addDraftItem := performJSON(router, http.MethodPost, "/api/v1/admin/packages/"+coursePackage.ID+"/items", `{"resourceType":"material","resourceId":"`+draftMaterial.ID+`","sortOrder":2}`, adminToken)
	if addDraftItem.Code != http.StatusOK {
		t.Fatalf("expected draft material item to be stageable by admin, got %d: %s", addDraftItem.Code, addDraftItem.Body.String())
	}
	adminItems := performJSON(router, http.MethodGet, "/api/v1/admin/packages/"+coursePackage.ID+"/items", "", adminToken)
	if adminItems.Code != http.StatusOK || !strings.Contains(adminItems.Body.String(), material.ID) || !strings.Contains(adminItems.Body.String(), draftMaterial.ID) {
		t.Fatalf("expected admin package items to include staged materials, got %d: %s", adminItems.Code, adminItems.Body.String())
	}

	publishedUpdate := performJSON(router, http.MethodPatch, "/api/v1/admin/packages/"+coursePackage.ID, `{"status":"published","priceFen":0}`, adminToken)
	if publishedUpdate.Code != http.StatusOK || !strings.Contains(publishedUpdate.Body.String(), `"priceFen":0`) {
		t.Fatalf("expected package publish/update 200, got %d: %s", publishedUpdate.Code, publishedUpdate.Body.String())
	}
	if countOperationLogs(t, db, "course_package.update", "course_package", coursePackage.ID, admin.ID) != 1 {
		t.Fatal("expected course_package.update operation log")
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/packages/"+coursePackage.ID, "", "")
	if publicDetail.Code != http.StatusOK || !strings.Contains(publicDetail.Body.String(), material.ID) {
		t.Fatalf("expected public package detail to include published material, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}
	if strings.Contains(publicDetail.Body.String(), draftMaterial.ID) {
		t.Fatalf("public package detail leaked draft package item: %s", publicDetail.Body.String())
	}

	deleteItem := performJSON(router, http.MethodDelete, "/api/v1/admin/packages/"+coursePackage.ID+"/items/"+itemPayload.Data.Item.ID, "", adminToken)
	if deleteItem.Code != http.StatusOK {
		t.Fatalf("expected package item delete 200, got %d: %s", deleteItem.Code, deleteItem.Body.String())
	}
	if countOperationLogs(t, db, "course_package_item.delete", "course_package_item", itemPayload.Data.Item.ID, admin.ID) != 1 {
		t.Fatal("expected course_package_item.delete operation log")
	}
	var stillExists model.Material
	if err := db.First(&stillExists, "id = ?", material.ID).Error; err != nil {
		t.Fatal("expected deleting package item not to delete material")
	}
	readdItem := performJSON(router, http.MethodPost, "/api/v1/admin/packages/"+coursePackage.ID+"/items", `{"resourceType":"material","resourceId":"`+material.ID+`","sortOrder":3}`, adminToken)
	if readdItem.Code != http.StatusOK || strings.Contains(readdItem.Body.String(), `"alreadyExists":true`) {
		t.Fatalf("expected removed package item to be addable again, got %d: %s", readdItem.Code, readdItem.Body.String())
	}

	archive := performJSON(router, http.MethodDelete, "/api/v1/admin/packages/"+coursePackage.ID, "", adminToken)
	if archive.Code != http.StatusOK {
		t.Fatalf("expected package archive 200, got %d: %s", archive.Code, archive.Body.String())
	}
	if countOperationLogs(t, db, "course_package.archive", "course_package", coursePackage.ID, admin.ID) != 1 {
		t.Fatal("expected course_package.archive operation log")
	}
	publicArchivedDetail := performJSON(router, http.MethodGet, "/api/v1/packages/"+coursePackage.ID, "", "")
	if publicArchivedDetail.Code != http.StatusNotFound {
		t.Fatalf("expected archived package not found publicly, got %d: %s", publicArchivedDetail.Code, publicArchivedDetail.Body.String())
	}
}

func TestAdminPackageRejectsMismatchedMaterialScope(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	otherCourse := model.Course{
		SchoolID:    course.SchoolID,
		CollegeID:   course.CollegeID,
		MajorID:     course.MajorID,
		Grade:       course.Grade,
		Name:        "Other Course",
		Slug:        "other-course",
		Description: "Other test course",
		Status:      model.StatusPublished,
	}
	if err := db.Create(&otherCourse).Error; err != nil {
		t.Fatal(err)
	}
	material := createTestMaterial(t, db, otherCourse.ID, "Other course material", model.MaterialAccessPaid, "materials/other-course.txt")
	coursePackage := createTestPackage(t, db, course, "scope-package", model.StatusDraft)

	admin := createTestUser(t, db, "package-scope-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, admin.Email)

	response := performJSON(router, http.MethodPost, "/api/v1/admin/packages/"+coursePackage.ID+"/items", `{"resourceType":"material","resourceId":"`+material.ID+`"}`, adminToken)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected mismatched package material 400, got %d: %s", response.Code, response.Body.String())
	}
}

func countPackageItems(t *testing.T, db *gorm.DB, packageID string, resourceID string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.CoursePackageItem{}).
		Where("package_id = ? AND resource_type = ? AND resource_id = ?", packageID, "material", resourceID).
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}
