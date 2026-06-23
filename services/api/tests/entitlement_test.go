package tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMyEntitlementsRequireLoginAndSummarizeActiveGrants(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	unauthorized := performJSON(router, http.MethodGet, "/api/v1/me/entitlements", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous entitlement query 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	course := createTestCourse(t, db)
	directMaterial := createTestMaterial(t, db, course.ID, "Direct grant note", model.MaterialAccessPaid, "materials/direct-grant.txt")
	packageMaterial := createTestMaterial(t, db, course.ID, "Package grant paper", model.MaterialAccessPaid, "materials/package-grant.txt")
	expiredMaterial := createTestMaterial(t, db, course.ID, "Expired grant note", model.MaterialAccessPaid, "materials/expired-grant.txt")
	draftDirectMaterial := createTestMaterial(t, db, course.ID, "Draft direct grant note", model.MaterialAccessPaid, "materials/draft-direct-grant.txt")
	if err := db.Model(&draftDirectMaterial).Update("status", model.StatusDraft).Error; err != nil {
		t.Fatal(err)
	}
	draftPackageMaterial := createTestMaterial(t, db, course.ID, "Draft package paper", model.MaterialAccessPaid, "materials/draft-package.txt")

	coursePackage := createTestPackage(t, db, course, "entitlement-package", model.StatusPublished)
	if err := db.Create(&model.CoursePackageItem{PackageID: coursePackage.ID, ResourceType: "material", ResourceID: packageMaterial.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}
	draftPackage := createTestPackage(t, db, course, "draft-entitlement-package", model.StatusDraft)
	if err := db.Create(&model.CoursePackageItem{PackageID: draftPackage.ID, ResourceType: "material", ResourceID: draftPackageMaterial.ID, SortOrder: 1}).Error; err != nil {
		t.Fatal(err)
	}

	token := loginTestUser(t, router, "entitled@stu.henu.edu.cn")
	var user model.User
	if err := db.First(&user, "email = ?", "entitled@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}

	expiredAt := time.Now().Add(-time.Hour)
	grants := []model.MaterialAccessGrant{
		{UserID: user.ID, MaterialID: &directMaterial.ID, Source: "manual"},
		{UserID: user.ID, PackageID: &coursePackage.ID, Source: "order"},
		{UserID: user.ID, MaterialID: &expiredMaterial.ID, Source: "expired", ExpiresAt: &expiredAt},
		{UserID: user.ID, MaterialID: &draftDirectMaterial.ID, Source: "draft_direct"},
		{UserID: user.ID, PackageID: &draftPackage.ID, Source: "draft_package"},
	}
	if err := db.Create(&grants).Error; err != nil {
		t.Fatal(err)
	}

	response := performJSON(router, http.MethodGet, "/api/v1/me/entitlements", "", token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected entitlement query 200, got %d: %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{directMaterial.ID, packageMaterial.ID, coursePackage.ID, `"directMaterialGrants":1`, `"packageGrants":1`, `"unlockedMaterials":2`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected entitlement response to contain %q, got %s", expected, body)
		}
	}
	for _, unexpected := range []string{expiredMaterial.ID, draftDirectMaterial.ID, draftPackage.ID, draftPackageMaterial.ID} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("entitlement response exposed inactive grant %q: %s", unexpected, body)
		}
	}
}

func createTestPackage(t *testing.T, db *gorm.DB, course model.Course, slug string, status string) model.CoursePackage {
	t.Helper()
	pkg := model.CoursePackage{
		SchoolID:    course.SchoolID,
		CollegeID:   course.CollegeID,
		MajorID:     course.MajorID,
		CourseID:    &course.ID,
		Grade:       course.Grade,
		Title:       slug,
		Slug:        slug,
		Description: "Test course package",
		PriceFen:    1990,
		Currency:    "CNY",
		Status:      status,
	}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	return pkg
}
