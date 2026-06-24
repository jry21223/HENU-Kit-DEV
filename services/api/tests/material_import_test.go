package tests

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/materialimport"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestMaterialManifestImportCreatesPackageMaterialsAndIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	uploadDir := t.TempDir()
	writeUploadFile(t, uploadDir, "materials/discrete-math/knowledge-note.pdf", "knowledge")
	writeUploadFile(t, uploadDir, "materials/discrete-math/mock-1.pdf", "mock")

	priceFen := int64(1990)
	manifest := []materialimport.ManifestEntry{
		{
			School:             "河南大学",
			College:            "软件学院",
			Major:              "网络工程",
			MajorSlug:          "network-engineering",
			Grade:              "2023级",
			CourseSlug:         "discrete-math",
			CourseName:         "离散数学",
			CourseDescription:  "离散数学复习课程。",
			ExamScope:          "命题逻辑、集合、关系、图论。",
			PackageSlug:        "henu-software-2023-discrete-math-final",
			PackageTitle:       "离散数学期末复习包",
			PackageDescription: "适合河南大学软件学院 2023 级网络工程使用。",
			PackageStatus:      model.StatusPublished,
			PriceFen:           &priceFen,
			Currency:           "CNY",
			Materials: []materialimport.ManifestMaterial{
				{
					Title:          "离散数学重点知识点讲义",
					Type:           "knowledge_note",
					Description:    "覆盖命题逻辑、集合、关系、图论、树等期末重点。",
					FilePath:       "uploads/materials/discrete-math/knowledge-note.pdf",
					AccessLevel:    model.MaterialAccessLoginRequired,
					Status:         model.StatusPublished,
					PreviewContent: "本讲义整理离散数学期末高频知识点。",
				},
				{
					Title:          "离散数学模拟卷一",
					Type:           "mock_paper",
					Description:    "按期末题型整理的第一套模拟卷。",
					FilePath:       "uploads/materials/discrete-math/mock-1.pdf",
					AccessLevel:    model.MaterialAccessPaid,
					Status:         model.StatusPublished,
					PreviewContent: "包含选择题、判断题、计算证明题和综合应用题。",
				},
			},
		},
	}

	result, err := materialimport.New(db, uploadDir).Import(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if result.CoursesCreated != 1 || result.PackagesCreated != 1 || result.MaterialsCreated != 2 || result.PackageItemsAdded != 2 {
		t.Fatalf("unexpected first import result: %#v", result)
	}
	assertMaterialImportCounts(t, db, 2, 1, 2)

	var material model.Material
	if err := db.First(&material, "title = ?", "离散数学重点知识点讲义").Error; err != nil {
		t.Fatal(err)
	}
	if material.StorageKey != "materials/discrete-math/knowledge-note.pdf" || material.FileSize != int64(len("knowledge")) {
		t.Fatalf("unexpected imported material file metadata: storage=%s size=%d", material.StorageKey, material.FileSize)
	}
	if material.AccessLevel != model.MaterialAccessLoginRequired || material.Status != model.StatusPublished {
		t.Fatalf("unexpected imported material policy: access=%s status=%s", material.AccessLevel, material.Status)
	}

	second, err := materialimport.New(db, uploadDir).Import(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if second.MaterialsCreated != 0 || second.PackageItemsAdded != 0 || second.PackageItemsKept != 2 {
		t.Fatalf("expected idempotent second import, got %#v", second)
	}
	assertMaterialImportCounts(t, db, 2, 1, 2)
}

func TestMaterialManifestImportRejectsUnsafeAndMissingFiles(t *testing.T) {
	t.Run("unsafe path rolls back", func(t *testing.T) {
		db := newTestDB(t)
		uploadDir := t.TempDir()
		secretPath := filepath.Join(uploadDir, "..", "secret.pdf")
		if err := os.WriteFile(secretPath, []byte("secret"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := materialimport.New(db, uploadDir).Import([]materialimport.ManifestEntry{manifestEntryWithFile("../../secret.pdf")})
		if !errors.Is(err, materialimport.ErrUnsafeFilePath) {
			t.Fatalf("expected unsafe path error, got %v", err)
		}
		assertMaterialImportCounts(t, db, 0, 0, 0)
	})

	t.Run("missing file rolls back", func(t *testing.T) {
		db := newTestDB(t)
		uploadDir := t.TempDir()
		_, err := materialimport.New(db, uploadDir).Import([]materialimport.ManifestEntry{manifestEntryWithFile("uploads/materials/missing.pdf")})
		if !errors.Is(err, materialimport.ErrMissingFile) {
			t.Fatalf("expected missing file error, got %v", err)
		}
		assertMaterialImportCounts(t, db, 0, 0, 0)
	})
}

func TestMaterialManifestImportSmokeCoversPaidDownloadDelivery(t *testing.T) {
	db := newTestDB(t)
	uploadDir := t.TempDir()
	writeUploadFile(t, uploadDir, "materials/import-smoke/free.txt", "free import content")
	writeUploadFile(t, uploadDir, "materials/import-smoke/login.txt", "login import content")
	writeUploadFile(t, uploadDir, "materials/import-smoke/paid.txt", "paid import content")

	priceFen := int64(1990)
	manifest := []materialimport.ManifestEntry{
		{
			School:        "Henan Test University",
			College:       "Software College",
			Major:         "Network Engineering",
			MajorSlug:     "network-engineering",
			Grade:         "2023",
			CourseSlug:    "import-smoke-course",
			CourseName:    "Import Smoke Course",
			PackageSlug:   "import-smoke-final-package",
			PackageTitle:  "Import Smoke Final Package",
			PackageStatus: model.StatusPublished,
			PriceFen:      &priceFen,
			Currency:      "CNY",
			Materials: []materialimport.ManifestMaterial{
				{
					Title:       "Import Smoke Free Sample",
					Type:        "other",
					FilePath:    "uploads/materials/import-smoke/free.txt",
					AccessLevel: model.MaterialAccessFree,
					Status:      model.StatusPublished,
				},
				{
					Title:       "Import Smoke Login Note",
					Type:        "knowledge_note",
					FilePath:    "uploads/materials/import-smoke/login.txt",
					AccessLevel: model.MaterialAccessLoginRequired,
					Status:      model.StatusPublished,
				},
				{
					Title:       "Import Smoke Paid Paper",
					Type:        "mock_paper",
					FilePath:    "uploads/materials/import-smoke/paid.txt",
					AccessLevel: model.MaterialAccessPaid,
					Status:      model.StatusPublished,
				},
			},
		},
	}
	if _, err := materialimport.New(db, uploadDir).Import(manifest); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig()
	cfg.LocalUploadDir = uploadDir
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	var coursePackage model.CoursePackage
	if err := db.First(&coursePackage, "slug = ?", "import-smoke-final-package").Error; err != nil {
		t.Fatal(err)
	}
	var freeMaterial model.Material
	if err := db.First(&freeMaterial, "title = ?", "Import Smoke Free Sample").Error; err != nil {
		t.Fatal(err)
	}
	var loginMaterial model.Material
	if err := db.First(&loginMaterial, "title = ?", "Import Smoke Login Note").Error; err != nil {
		t.Fatal(err)
	}
	var paidMaterial model.Material
	if err := db.First(&paidMaterial, "title = ?", "Import Smoke Paid Paper").Error; err != nil {
		t.Fatal(err)
	}

	packageDetail := performJSON(router, http.MethodGet, "/api/v1/packages/"+coursePackage.ID, "", "")
	if packageDetail.Code != http.StatusOK || !strings.Contains(packageDetail.Body.String(), paidMaterial.ID) || strings.Contains(packageDetail.Body.String(), paidMaterial.StorageKey) {
		t.Fatalf("expected public package detail to include paid material id without storage key, got %d: %s", packageDetail.Code, packageDetail.Body.String())
	}

	freeDownload := performJSON(router, http.MethodGet, "/api/v1/materials/"+freeMaterial.ID+"/download", "", "")
	if freeDownload.Code != http.StatusOK || freeDownload.Body.String() != "free import content" {
		t.Fatalf("expected imported free material download, got %d: %s", freeDownload.Code, freeDownload.Body.String())
	}

	loginDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+loginMaterial.ID+"/download", "", "")
	if loginDenied.Code != http.StatusUnauthorized {
		t.Fatalf("expected imported login material to require auth, got %d: %s", loginDenied.Code, loginDenied.Body.String())
	}

	token := loginTestUser(t, router, "import-smoke-buyer@stu.henu.edu.cn")
	loginAllowed := performJSON(router, http.MethodGet, "/api/v1/materials/"+loginMaterial.ID+"/download", "", token)
	if loginAllowed.Code != http.StatusOK || loginAllowed.Body.String() != "login import content" {
		t.Fatalf("expected imported login material authenticated download, got %d: %s", loginAllowed.Code, loginAllowed.Body.String())
	}
	paidDenied := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if paidDenied.Code != http.StatusForbidden || !strings.Contains(paidDenied.Body.String(), "entitlement_required") {
		t.Fatalf("expected imported paid material to require entitlement, got %d: %s", paidDenied.Code, paidDenied.Body.String())
	}

	var user model.User
	if err := db.First(&user, "email = ?", "import-smoke-buyer@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 0 {
		t.Fatal("expected denied imported paid download to create no audit log")
	}
	grant := model.MaterialAccessGrant{UserID: user.ID, PackageID: &coursePackage.ID, Source: "test_import_smoke"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	paidAllowed := performJSON(router, http.MethodGet, "/api/v1/materials/"+paidMaterial.ID+"/download", "", token)
	if paidAllowed.Code != http.StatusOK || paidAllowed.Body.String() != "paid import content" {
		t.Fatalf("expected imported paid material download after package grant, got %d: %s", paidAllowed.Code, paidAllowed.Body.String())
	}
	if countDownloadLogs(t, db, paidMaterial.ID, user.ID) != 1 {
		t.Fatal("expected imported paid download to create one audit log after grant")
	}
}

func manifestEntryWithFile(filePath string) materialimport.ManifestEntry {
	return materialimport.ManifestEntry{
		School:      "河南大学",
		College:     "软件学院",
		Major:       "网络工程",
		MajorSlug:   "network-engineering",
		Grade:       "2023级",
		CourseSlug:  "discrete-math",
		CourseName:  "离散数学",
		PackageSlug: "henu-software-2023-discrete-math-final",
		Materials: []materialimport.ManifestMaterial{
			{
				Title:       "离散数学重点知识点讲义",
				Type:        "knowledge_note",
				FilePath:    filePath,
				AccessLevel: model.MaterialAccessLoginRequired,
				Status:      model.StatusPublished,
			},
		},
	}
}

func assertMaterialImportCounts(t *testing.T, db *gorm.DB, wantMaterials int64, wantPackages int64, wantItems int64) {
	t.Helper()
	var materials int64
	if err := db.Model(&model.Material{}).Count(&materials).Error; err != nil {
		t.Fatal(err)
	}
	if materials != wantMaterials {
		t.Fatalf("expected %d materials, got %d", wantMaterials, materials)
	}
	var packages int64
	if err := db.Model(&model.CoursePackage{}).Count(&packages).Error; err != nil {
		t.Fatal(err)
	}
	if packages != wantPackages {
		t.Fatalf("expected %d packages, got %d", wantPackages, packages)
	}
	var items int64
	if err := db.Model(&model.CoursePackageItem{}).Count(&items).Error; err != nil {
		t.Fatal(err)
	}
	if items != wantItems {
		t.Fatalf("expected %d package items, got %d", wantItems, items)
	}
}
