package tests

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAdminCourseCRUDRequiresAdmin(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	studentToken := loginTestUser(t, router, "student@stu.henu.edu.cn")
	forbidden := performJSON(router, http.MethodPost, "/api/v1/admin/courses", `{}`, studentToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected student admin create 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}

	createTestUser(t, db, "admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "admin@stu.henu.edu.cn")

	var school model.School
	if err := db.First(&school, "slug = ?", "henu").Error; err != nil {
		t.Fatal(err)
	}
	collegeBody := `{"schoolId":"` + school.ID + `","name":"Admin College","status":"published"}`
	collegeResponse := performJSON(router, http.MethodPost, "/api/v1/admin/colleges", collegeBody, adminToken)
	if collegeResponse.Code != http.StatusOK {
		t.Fatalf("expected college create 200, got %d: %s", collegeResponse.Code, collegeResponse.Body.String())
	}
	var college model.College
	if err := db.First(&college, "name = ?", "Admin College").Error; err != nil {
		t.Fatal(err)
	}

	majorBody := `{"schoolId":"` + school.ID + `","collegeId":"` + college.ID + `","name":"Admin Major","slug":"admin-major","status":"published"}`
	majorResponse := performJSON(router, http.MethodPost, "/api/v1/admin/majors", majorBody, adminToken)
	if majorResponse.Code != http.StatusOK {
		t.Fatalf("expected major create 200, got %d: %s", majorResponse.Code, majorResponse.Body.String())
	}
	var major model.Major
	if err := db.First(&major, "slug = ?", "admin-major").Error; err != nil {
		t.Fatal(err)
	}

	courseBody := `{"schoolId":"` + school.ID + `","collegeId":"` + college.ID + `","majorId":"` + major.ID + `","grade":"2023","name":"Admin Course","slug":"admin-course","status":"published"}`
	courseResponse := performJSON(router, http.MethodPost, "/api/v1/admin/courses", courseBody, adminToken)
	if courseResponse.Code != http.StatusOK {
		t.Fatalf("expected course create 200, got %d: %s", courseResponse.Code, courseResponse.Body.String())
	}
	var course model.Course
	if err := db.First(&course, "slug = ?", "admin-course").Error; err != nil {
		t.Fatal(err)
	}

	updateResponse := performJSON(router, http.MethodPatch, "/api/v1/admin/courses/"+course.ID, `{"name":"Updated Course"}`, adminToken)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("expected course update 200, got %d: %s", updateResponse.Code, updateResponse.Body.String())
	}

	archiveResponse := performJSON(router, http.MethodDelete, "/api/v1/admin/courses/"+course.ID, "", adminToken)
	if archiveResponse.Code != http.StatusOK {
		t.Fatalf("expected course archive 200, got %d: %s", archiveResponse.Code, archiveResponse.Body.String())
	}
	var archived model.Course
	if err := db.First(&archived, "id = ?", course.ID).Error; err != nil {
		t.Fatal(err)
	}
	if archived.Status != model.StatusArchived {
		t.Fatalf("expected archived course, got %s", archived.Status)
	}
}

func TestAdminMaterialUploadGuards(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.LocalUploadDir = t.TempDir()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	createTestUser(t, db, "admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "admin@stu.henu.edu.cn")

	unsafeStorage := performJSON(router, http.MethodPost, "/api/v1/admin/materials", `{"courseId":"`+course.ID+`","title":"Unsafe","storageKey":"../secret.pdf"}`, adminToken)
	if unsafeStorage.Code != http.StatusBadRequest || !strings.Contains(unsafeStorage.Body.String(), "unsafe_storage_key") {
		t.Fatalf("expected unsafe storage key rejection, got %d: %s", unsafeStorage.Code, unsafeStorage.Body.String())
	}

	unsupported := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId": course.ID,
		"title":    "Unsupported",
	}, "file", "run.exe", []byte("MZ"))
	if unsupported.Code != http.StatusBadRequest || !strings.Contains(unsupported.Body.String(), "unsupported_file_type") {
		t.Fatalf("expected unsupported type rejection, got %d: %s", unsupported.Code, unsupported.Body.String())
	}

	invalidPDF := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId": course.ID,
		"title":    "Invalid PDF",
	}, "file", "bad.pdf", []byte("not a pdf"))
	if invalidPDF.Code != http.StatusBadRequest || !strings.Contains(invalidPDF.Body.String(), "invalid_file_content") {
		t.Fatalf("expected invalid pdf rejection, got %d: %s", invalidPDF.Code, invalidPDF.Body.String())
	}

	tooLarge := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId": course.ID,
		"title":    "Too Large",
	}, "file", "large.txt", bytes.Repeat([]byte("a"), 20*1024*1024+1))
	if tooLarge.Code != http.StatusBadRequest || !strings.Contains(tooLarge.Body.String(), "file_too_large") {
		t.Fatalf("expected large file rejection, got %d: %s", tooLarge.Code, tooLarge.Body.String())
	}

	upload := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId":       course.ID,
		"title":          "Admin Upload",
		"type":           "knowledge_note",
		"accessLevel":    model.MaterialAccessLoginRequired,
		"status":         model.StatusPublished,
		"previewContent": "Uploaded preview",
	}, "file", "note.pdf", []byte("%PDF-1.4\ncontent"))
	if upload.Code != http.StatusOK {
		t.Fatalf("expected upload 200, got %d: %s", upload.Code, upload.Body.String())
	}

	var material model.Material
	if err := db.First(&material, "title = ?", "Admin Upload").Error; err != nil {
		t.Fatal(err)
	}
	if material.StorageKey == "" || strings.Contains(material.StorageKey, "note.pdf") {
		t.Fatalf("expected generated storage key, got %s", material.StorageKey)
	}
	if _, err := os.Stat(filepath.Join(cfg.LocalUploadDir, filepath.FromSlash(material.StorageKey))); err != nil {
		t.Fatalf("expected uploaded file on disk: %v", err)
	}
}

func performMultipart(router http.Handler, path string, token string, fields map[string]string, fileField string, fileName string, content []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		_ = writer.WriteField(key, value)
	}
	part, _ := writer.CreateFormFile(fileField, fileName)
	_, _ = io.Copy(part, bytes.NewReader(content))
	_ = writer.Close()

	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
