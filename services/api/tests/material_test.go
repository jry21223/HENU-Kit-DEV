package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMaterialDownloadPermissions(t *testing.T) {
	db := newTestDB(t)
	uploadDir := t.TempDir()
	cfg := testConfig()
	cfg.LocalUploadDir = uploadDir
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	freeMaterial := createTestMaterial(t, db, course.ID, "Free sample", model.MaterialAccessFree, "materials/free.txt")
	loginMaterial := createTestMaterial(t, db, course.ID, "Login note", model.MaterialAccessLoginRequired, "materials/login.txt")
	paidMaterial := createTestMaterial(t, db, course.ID, "Paid paper", model.MaterialAccessPaid, "materials/paid.txt")
	writeUploadFile(t, uploadDir, freeMaterial.StorageKey, "free")
	writeUploadFile(t, uploadDir, loginMaterial.StorageKey, "login")
	writeUploadFile(t, uploadDir, paidMaterial.StorageKey, "paid")

	detailResponse := performJSON(router, http.MethodGet, "/api/v1/materials/"+freeMaterial.ID, "", "")
	if detailResponse.Code != http.StatusOK {
		t.Fatalf("expected material detail 200, got %d: %s", detailResponse.Code, detailResponse.Body.String())
	}
	if strings.Contains(detailResponse.Body.String(), freeMaterial.StorageKey) {
		t.Fatal("material detail leaked storage key")
	}

	freeResponse := performJSON(router, http.MethodGet, "/api/v1/materials/"+freeMaterial.ID+"/download", "", "")
	if freeResponse.Code != http.StatusOK {
		t.Fatalf("expected free download 200, got %d: %s", freeResponse.Code, freeResponse.Body.String())
	}
	if countDownloadLogs(t, db, freeMaterial.ID, "") != 1 {
		t.Fatal("expected free download to create one anonymous audit log")
	}

	loginDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+loginMaterial.ID+"/download", "", "")
	if loginDenied.Code != http.StatusUnauthorized {
		t.Fatalf("expected login_required unauthenticated 401, got %d: %s", loginDenied.Code, loginDenied.Body.String())
	}
	if countDownloadLogs(t, db, loginMaterial.ID, "") != 0 {
		t.Fatal("expected denied login_required download to create no audit log")
	}

	token := loginTestUser(t, router, "student@stu.henu.edu.cn")
	loginAllowed := performJSON(router, http.MethodGet, "/api/v1/materials/"+loginMaterial.ID+"/download", "", token)
	if loginAllowed.Code != http.StatusOK {
		t.Fatalf("expected login_required authenticated 200, got %d: %s", loginAllowed.Code, loginAllowed.Body.String())
	}

	var user model.User
	if err := db.First(&user, "email = ?", "student@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}
	if countDownloadLogs(t, db, loginMaterial.ID, user.ID) != 1 {
		t.Fatal("expected authenticated download to create one user audit log")
	}

	paidDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if paidDenied.Code != http.StatusForbidden {
		t.Fatalf("expected paid without grant 403, got %d: %s", paidDenied.Code, paidDenied.Body.String())
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 0 {
		t.Fatal("expected denied paid download to create no audit log")
	}
	grant := model.MaterialAccessGrant{
		UserID:     user.ID,
		MaterialID: &paidMaterial.ID,
		Source:     "test",
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}

	paidAllowed := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if paidAllowed.Code != http.StatusOK {
		t.Fatalf("expected paid with grant 200, got %d: %s", paidAllowed.Code, paidAllowed.Body.String())
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 1 {
		t.Fatal("expected paid download to create one user audit log")
	}

	myDownloads := performJSON(router, http.MethodGet, "/api/v1/me/downloads", "", token)
	if myDownloads.Code != http.StatusOK || !strings.Contains(myDownloads.Body.String(), paidMaterial.ID) || strings.Contains(myDownloads.Body.String(), freeMaterial.ID) {
		t.Fatalf("expected my downloads to include only authenticated user logs, got %d: %s", myDownloads.Code, myDownloads.Body.String())
	}

	createTestUser(t, db, "download-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "download-admin@stu.henu.edu.cn")
	adminDownloads := performJSON(router, http.MethodGet, "/api/v1/admin/downloads", "", adminToken)
	if adminDownloads.Code != http.StatusOK || !strings.Contains(adminDownloads.Body.String(), paidMaterial.ID) || !strings.Contains(adminDownloads.Body.String(), freeMaterial.ID) {
		t.Fatalf("expected admin downloads to include all successful logs, got %d: %s", adminDownloads.Code, adminDownloads.Body.String())
	}
	if !strings.Contains(adminDownloads.Body.String(), "userAgent") {
		t.Fatalf("expected admin downloads to include request metadata, got %s", adminDownloads.Body.String())
	}

	studentAdminDenied := performJSON(router, http.MethodGet, "/api/v1/admin/downloads", "", token)
	if studentAdminDenied.Code != http.StatusForbidden {
		t.Fatalf("expected student admin downloads access 403, got %d: %s", studentAdminDenied.Code, studentAdminDenied.Body.String())
	}
}

func TestMaterialDownloadRejectsUnsafeStorageKey(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.LocalUploadDir = t.TempDir()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	material := createTestMaterial(t, db, course.ID, "Unsafe", model.MaterialAccessFree, "../secret.txt")

	response := performJSON(router, http.MethodGet, "/api/v1/materials/"+material.ID+"/download", "", "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected unsafe storage key to be hidden as 404, got %d: %s", response.Code, response.Body.String())
	}
	if countDownloadLogs(t, db, material.ID, "") != 0 {
		t.Fatal("expected unsafe storage key to create no download audit log")
	}
}

func TestPackageGrantUnlocksPaidMaterial(t *testing.T) {
	db := newTestDB(t)
	uploadDir := t.TempDir()
	cfg := testConfig()
	cfg.LocalUploadDir = uploadDir
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	paidMaterial := createTestMaterial(t, db, course.ID, "Package paid paper", model.MaterialAccessPaid, "materials/package-paid.txt")
	writeUploadFile(t, uploadDir, paidMaterial.StorageKey, "paid package material")

	coursePackage := model.CoursePackage{
		SchoolID:    course.SchoolID,
		CollegeID:   course.CollegeID,
		MajorID:     course.MajorID,
		CourseID:    &course.ID,
		Grade:       course.Grade,
		Title:       "Discrete Math Final Package",
		Slug:        "discrete-math-final-package",
		Description: "Test course package",
		PriceFen:    1990,
		Currency:    "CNY",
		Status:      model.StatusPublished,
	}
	if err := db.Create(&coursePackage).Error; err != nil {
		t.Fatal(err)
	}
	item := model.CoursePackageItem{PackageID: coursePackage.ID, ResourceType: "material", ResourceID: paidMaterial.ID, SortOrder: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}

	listResponse := performJSON(router, http.MethodGet, "/api/v1/packages?courseId="+course.ID, "", "")
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), coursePackage.ID) {
		t.Fatalf("expected package list to include course package, got %d: %s", listResponse.Code, listResponse.Body.String())
	}
	detailResponse := performJSON(router, http.MethodGet, "/api/v1/packages/"+coursePackage.ID, "", "")
	if detailResponse.Code != http.StatusOK || !strings.Contains(detailResponse.Body.String(), paidMaterial.ID) {
		t.Fatalf("expected package detail to include material, got %d: %s", detailResponse.Code, detailResponse.Body.String())
	}

	token := loginTestUser(t, router, "package-buyer@stu.henu.edu.cn")
	denied := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected paid material without package grant 403, got %d: %s", denied.Code, denied.Body.String())
	}

	var user model.User
	if err := db.First(&user, "email = ?", "package-buyer@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 0 {
		t.Fatal("expected denied package material download to create no audit log")
	}
	grant := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "test_package"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	allowed := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if allowed.Code != http.StatusOK {
		t.Fatalf("expected paid material with package grant 200, got %d: %s", allowed.Code, allowed.Body.String())
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 1 {
		t.Fatal("expected package-granted paid download to create one audit log")
	}

	otherToken := loginTestUser(t, router, "expired-package@stu.henu.edu.cn")
	var otherUser model.User
	if err := db.First(&otherUser, "email = ?", "expired-package@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}
	expiredAt := time.Now().Add(-time.Hour)
	expiredGrant := model.MaterialAccessGrant{UserID: otherUser.ID, PackageID: &coursePackage.ID, Source: "test_expired_package", ExpiresAt: &expiredAt}
	if err := db.Create(&expiredGrant).Error; err != nil {
		t.Fatal(err)
	}
	expiredDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", otherToken)
	if expiredDenied.Code != http.StatusForbidden {
		t.Fatalf("expected expired package grant 403, got %d: %s", expiredDenied.Code, expiredDenied.Body.String())
	}
	if countDownloadLogs(t, db, paidMaterial.ID, otherUser.ID) != 0 {
		t.Fatal("expected expired package grant denial to create no audit log")
	}
}

func countDownloadLogs(t *testing.T, db *gorm.DB, materialID string, userID string) int64 {
	t.Helper()
	query := db.Model(&model.MaterialDownloadLog{}).Where("material_id = ?", materialID)
	if userID == "" {
		query = query.Where("user_id IS NULL")
	} else {
		query = query.Where("user_id = ?", userID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func writeUploadFile(t *testing.T, uploadDir string, storageKey string, content string) {
	t.Helper()
	path := filepath.Join(uploadDir, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
