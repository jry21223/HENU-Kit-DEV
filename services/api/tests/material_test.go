package tests

import (
	"bytes"
	"fmt"
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
	if freeResponse.Header().Get("X-Watermark-Applied") != "false" || freeResponse.Body.String() != "free" {
		t.Fatalf("expected non-PDF download without watermark, got header=%q body=%q", freeResponse.Header().Get("X-Watermark-Applied"), freeResponse.Body.String())
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
	if err := db.Model(&paidMaterial).Updates(map[string]interface{}{
		"created_by":    user.ID,
		"reviewer_id":   user.ID,
		"review_reason": "internal download audit note",
	}).Error; err != nil {
		t.Fatal(err)
	}

	paidAllowed := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if paidAllowed.Code != http.StatusOK {
		t.Fatalf("expected paid with grant 200, got %d: %s", paidAllowed.Code, paidAllowed.Body.String())
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 1 {
		t.Fatal("expected paid download to create one user audit log")
	}

	if err := db.Model(&user).Update("status", "frozen").Error; err != nil {
		t.Fatal(err)
	}
	frozenLoginDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+loginMaterial.ID+"/download", "", token)
	if frozenLoginDenied.Code != http.StatusForbidden || !strings.Contains(frozenLoginDenied.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen user login_required download 403, got %d: %s", frozenLoginDenied.Code, frozenLoginDenied.Body.String())
	}
	if countDownloadLogs(t, db, loginMaterial.ID, user.ID) != 1 {
		t.Fatal("expected frozen login_required denial to create no audit log")
	}
	frozenPaidDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if frozenPaidDenied.Code != http.StatusForbidden || !strings.Contains(frozenPaidDenied.Body.String(), "user_frozen") {
		t.Fatalf("expected frozen user paid download 403, got %d: %s", frozenPaidDenied.Code, frozenPaidDenied.Body.String())
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 1 {
		t.Fatal("expected frozen paid denial to create no audit log")
	}

	myDownloads := performJSON(router, http.MethodGet, "/api/v1/me/downloads", "", token)
	if myDownloads.Code != http.StatusOK || !strings.Contains(myDownloads.Body.String(), paidMaterial.ID) || strings.Contains(myDownloads.Body.String(), freeMaterial.ID) {
		t.Fatalf("expected my downloads to include only authenticated user logs, got %d: %s", myDownloads.Code, myDownloads.Body.String())
	}
	for _, hiddenField := range []string{paidMaterial.StorageKey, "createdBy", "reviewerId", "reviewReason", "internal download audit note"} {
		if strings.Contains(myDownloads.Body.String(), hiddenField) {
			t.Fatalf("my downloads leaked internal material field %q: %s", hiddenField, myDownloads.Body.String())
		}
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

func TestPDFDownloadAppliesWatermarkWithoutMutatingSource(t *testing.T) {
	db := newTestDB(t)
	uploadDir := t.TempDir()
	cfg := testConfig()
	cfg.LocalUploadDir = uploadDir
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	material := createTestMaterial(t, db, course.ID, "Watermarked note", model.MaterialAccessLoginRequired, "materials/watermarked.pdf")
	material.FileName = "watermarked.pdf"
	if err := db.Save(&material).Error; err != nil {
		t.Fatal(err)
	}
	original := minimalPDF(t)
	writeUploadBytes(t, uploadDir, material.StorageKey, original)

	token := loginTestUser(t, router, "watermark@stu.henu.edu.cn")
	response := performJSON(router, http.MethodGet, "/api/v1/materials/"+material.ID+"/download", "", token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected watermarked PDF download 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Watermark-Applied") != "true" {
		t.Fatalf("expected watermark header true, got %q", response.Header().Get("X-Watermark-Applied"))
	}
	if !bytes.HasPrefix(response.Body.Bytes(), []byte("%PDF")) {
		t.Fatalf("expected PDF response, got %q", response.Body.String())
	}
	if bytes.Equal(response.Body.Bytes(), original) {
		t.Fatal("expected watermarked PDF response to differ from source")
	}
	sourcePath := filepath.Join(uploadDir, filepath.FromSlash(material.StorageKey))
	after, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Fatal("expected source PDF to remain unchanged after watermarking")
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
	writeUploadBytes(t, uploadDir, storageKey, []byte(content))
}

func writeUploadBytes(t *testing.T, uploadDir string, storageKey string, content []byte) {
	t.Helper()
	path := filepath.Join(uploadDir, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func minimalPDF(t *testing.T) []byte {
	t.Helper()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Length 44 >>\nstream\nBT /F1 24 Tf 72 720 Td (Original) Tj ET\nendstream",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)
	for index, object := range objects {
		offsets = append(offsets, buf.Len())
		buf.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", index+1, object))
	}
	xrefOffset := buf.Len()
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objects)+1))
	buf.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset))
	return buf.Bytes()
}
