package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	loginDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+loginMaterial.ID+"/download", "", "")
	if loginDenied.Code != http.StatusUnauthorized {
		t.Fatalf("expected login_required unauthenticated 401, got %d: %s", loginDenied.Code, loginDenied.Body.String())
	}

	token := loginTestUser(t, router, "student@stu.henu.edu.cn")
	loginAllowed := performJSON(router, http.MethodGet, "/api/v1/materials/"+loginMaterial.ID+"/download", "", token)
	if loginAllowed.Code != http.StatusOK {
		t.Fatalf("expected login_required authenticated 200, got %d: %s", loginAllowed.Code, loginAllowed.Body.String())
	}

	paidDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if paidDenied.Code != http.StatusForbidden {
		t.Fatalf("expected paid without grant 403, got %d: %s", paidDenied.Code, paidDenied.Body.String())
	}

	var user model.User
	if err := db.First(&user, "email = ?", "student@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
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
