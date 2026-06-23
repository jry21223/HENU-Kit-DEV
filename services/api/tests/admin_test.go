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
	forbiddenList := performJSON(router, http.MethodGet, "/api/v1/admin/courses", "", studentToken)
	if forbiddenList.Code != http.StatusForbidden {
		t.Fatalf("expected student admin course list 403, got %d: %s", forbiddenList.Code, forbiddenList.Body.String())
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

	invalidStatusBody := `{"schoolId":"` + school.ID + `","collegeId":"` + college.ID + `","majorId":"` + major.ID + `","grade":"2023","name":"Invalid Status","slug":"invalid-status","status":"pending_review"}`
	invalidStatusResponse := performJSON(router, http.MethodPost, "/api/v1/admin/courses", invalidStatusBody, adminToken)
	if invalidStatusResponse.Code != http.StatusBadRequest || !strings.Contains(invalidStatusResponse.Body.String(), "invalid_status") {
		t.Fatalf("expected invalid course status rejection, got %d: %s", invalidStatusResponse.Code, invalidStatusResponse.Body.String())
	}

	draftBody := `{"schoolId":"` + school.ID + `","collegeId":"` + college.ID + `","majorId":"` + major.ID + `","grade":"2023","name":"Draft Course","slug":"draft-course","status":"draft"}`
	draftResponse := performJSON(router, http.MethodPost, "/api/v1/admin/courses", draftBody, adminToken)
	if draftResponse.Code != http.StatusOK {
		t.Fatalf("expected draft course create 200, got %d: %s", draftResponse.Code, draftResponse.Body.String())
	}
	adminList := performJSON(router, http.MethodGet, "/api/v1/admin/courses", "", adminToken)
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), "draft-course") {
		t.Fatalf("expected admin course list to include draft course, got %d: %s", adminList.Code, adminList.Body.String())
	}
	var draftCourse model.Course
	if err := db.First(&draftCourse, "slug = ?", "draft-course").Error; err != nil {
		t.Fatal(err)
	}
	publicList := performJSON(router, http.MethodGet, "/api/v1/courses", "", "")
	if publicList.Code != http.StatusOK {
		t.Fatalf("expected public course list 200, got %d: %s", publicList.Code, publicList.Body.String())
	}
	if strings.Contains(publicList.Body.String(), "draft-course") {
		t.Fatalf("public course list must not include draft courses: %s", publicList.Body.String())
	}
	publicDraftDetail := performJSON(router, http.MethodGet, "/api/v1/courses/"+draftCourse.ID, "", "")
	if publicDraftDetail.Code != http.StatusNotFound {
		t.Fatalf("public course detail must not expose draft courses, got %d: %s", publicDraftDetail.Code, publicDraftDetail.Body.String())
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
	invalidPatch := performJSON(router, http.MethodPatch, "/api/v1/admin/courses/"+course.ID, `{"status":"pending_review"}`, adminToken)
	if invalidPatch.Code != http.StatusBadRequest || !strings.Contains(invalidPatch.Body.String(), "invalid_status") {
		t.Fatalf("expected invalid course status patch rejection, got %d: %s", invalidPatch.Code, invalidPatch.Body.String())
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

	filesBeforeInvalidStatus := countRegularFiles(t, cfg.LocalUploadDir)
	invalidStatus := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId": course.ID,
		"title":    "Invalid Status",
		"status":   "live",
	}, "file", "note.txt", []byte("plain text"))
	if invalidStatus.Code != http.StatusBadRequest || !strings.Contains(invalidStatus.Body.String(), "invalid_status") {
		t.Fatalf("expected invalid status rejection, got %d: %s", invalidStatus.Code, invalidStatus.Body.String())
	}
	if countRegularFiles(t, cfg.LocalUploadDir) != filesBeforeInvalidStatus {
		t.Fatal("expected invalid status upload to leave no file on disk")
	}

	filesBeforeInvalidAccess := countRegularFiles(t, cfg.LocalUploadDir)
	invalidAccess := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId":    course.ID,
		"title":       "Invalid Access",
		"accessLevel": "internal_only",
	}, "file", "access.txt", []byte("plain text"))
	if invalidAccess.Code != http.StatusBadRequest || !strings.Contains(invalidAccess.Body.String(), "invalid_access_level") {
		t.Fatalf("expected invalid access rejection, got %d: %s", invalidAccess.Code, invalidAccess.Body.String())
	}
	if countRegularFiles(t, cfg.LocalUploadDir) != filesBeforeInvalidAccess {
		t.Fatal("expected invalid access upload to leave no file on disk")
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

func TestAdminMaterialStatusFlow(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	createTestUser(t, db, "material-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "material-admin@stu.henu.edu.cn")
	studentToken := loginTestUser(t, router, "material-student@stu.henu.edu.cn")

	createBody := `{"courseId":"` + course.ID + `","title":"Draft Material","storageKey":"materials/draft-material.txt","accessLevel":"free"}`
	createResponse := performJSON(router, http.MethodPost, "/api/v1/admin/materials", createBody, adminToken)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("expected draft material create 200, got %d: %s", createResponse.Code, createResponse.Body.String())
	}

	var material model.Material
	if err := db.First(&material, "title = ?", "Draft Material").Error; err != nil {
		t.Fatal(err)
	}
	if material.Status != model.StatusDraft {
		t.Fatalf("expected missing status to default to draft, got %s", material.Status)
	}

	publicList := performJSON(router, http.MethodGet, "/api/v1/materials", "", "")
	if publicList.Code != http.StatusOK || strings.Contains(publicList.Body.String(), material.ID) {
		t.Fatalf("expected draft material hidden from public list, got %d: %s", publicList.Code, publicList.Body.String())
	}
	publicDetail := performJSON(router, http.MethodGet, "/api/v1/materials/"+material.ID, "", "")
	if publicDetail.Code != http.StatusNotFound {
		t.Fatalf("expected draft material detail 404, got %d: %s", publicDetail.Code, publicDetail.Body.String())
	}

	adminList := performJSON(router, http.MethodGet, "/api/v1/admin/materials", "", adminToken)
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), material.ID) {
		t.Fatalf("expected admin list to include draft material, got %d: %s", adminList.Code, adminList.Body.String())
	}
	studentDenied := performJSON(router, http.MethodGet, "/api/v1/admin/materials", "", studentToken)
	if studentDenied.Code != http.StatusForbidden {
		t.Fatalf("expected student admin material list 403, got %d: %s", studentDenied.Code, studentDenied.Body.String())
	}

	invalidAccess := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID, `{"accessLevel":"internal_only"}`, adminToken)
	if invalidAccess.Code != http.StatusBadRequest || !strings.Contains(invalidAccess.Body.String(), "invalid_access_level") {
		t.Fatalf("expected invalid access rejection, got %d: %s", invalidAccess.Code, invalidAccess.Body.String())
	}

	invalidType := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID, `{"type":"leaked_exam"}`, adminToken)
	if invalidType.Code != http.StatusBadRequest || !strings.Contains(invalidType.Body.String(), "invalid_material_type") {
		t.Fatalf("expected invalid material type rejection, got %d: %s", invalidType.Code, invalidType.Body.String())
	}

	updateBody := `{"title":"Edited Material","type":"answer","description":"Edited description","previewContent":"Edited preview","accessLevel":"paid","status":"pending"}`
	update := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID, updateBody, adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("expected material edit 200, got %d: %s", update.Code, update.Body.String())
	}
	if err := db.First(&material, "id = ?", material.ID).Error; err != nil {
		t.Fatal(err)
	}
	if material.Title != "Edited Material" || material.Type != "answer" || material.Description != "Edited description" || material.PreviewContent != "Edited preview" || material.AccessLevel != model.MaterialAccessPaid || material.Status != model.StatusPending {
		t.Fatalf("unexpected edited material: %#v", material)
	}

	invalidStatus := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID+"/status", `{"status":"live"}`, adminToken)
	if invalidStatus.Code != http.StatusBadRequest || !strings.Contains(invalidStatus.Body.String(), "invalid_status") {
		t.Fatalf("expected invalid status rejection, got %d: %s", invalidStatus.Code, invalidStatus.Body.String())
	}

	publish := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID+"/status", `{"status":"published"}`, adminToken)
	if publish.Code != http.StatusOK {
		t.Fatalf("expected publish status update 200, got %d: %s", publish.Code, publish.Body.String())
	}
	publicListAfterPublish := performJSON(router, http.MethodGet, "/api/v1/materials", "", "")
	if publicListAfterPublish.Code != http.StatusOK || !strings.Contains(publicListAfterPublish.Body.String(), material.ID) {
		t.Fatalf("expected published material in public list, got %d: %s", publicListAfterPublish.Code, publicListAfterPublish.Body.String())
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

func countRegularFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 0
	}
	if err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return count
}
