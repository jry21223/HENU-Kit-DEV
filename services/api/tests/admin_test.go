package tests

import (
	"bytes"
	"encoding/csv"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	admin := createTestUser(t, db, "admin@stu.henu.edu.cn", model.RoleAdmin)
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
	if countOperationLogs(t, db, "course.create", "course", course.ID, admin.ID) != 1 {
		t.Fatal("expected course create operation log")
	}

	updateResponse := performJSON(router, http.MethodPatch, "/api/v1/admin/courses/"+course.ID, `{"name":"Updated Course"}`, adminToken)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("expected course update 200, got %d: %s", updateResponse.Code, updateResponse.Body.String())
	}
	if countOperationLogs(t, db, "course.update", "course", course.ID, admin.ID) != 1 {
		t.Fatal("expected course update operation log")
	}
	invalidPatch := performJSON(router, http.MethodPatch, "/api/v1/admin/courses/"+course.ID, `{"status":"pending_review"}`, adminToken)
	if invalidPatch.Code != http.StatusBadRequest || !strings.Contains(invalidPatch.Body.String(), "invalid_status") {
		t.Fatalf("expected invalid course status patch rejection, got %d: %s", invalidPatch.Code, invalidPatch.Body.String())
	}
	if countOperationLogs(t, db, "course.update", "course", course.ID, admin.ID) != 1 {
		t.Fatal("invalid course update must not write an operation log")
	}

	archiveResponse := performJSON(router, http.MethodDelete, "/api/v1/admin/courses/"+course.ID, "", adminToken)
	if archiveResponse.Code != http.StatusOK {
		t.Fatalf("expected course archive 200, got %d: %s", archiveResponse.Code, archiveResponse.Body.String())
	}
	if countOperationLogs(t, db, "course.archive", "course", course.ID, admin.ID) != 1 {
		t.Fatal("expected course archive operation log")
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
	admin := createTestUser(t, db, "admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "admin@stu.henu.edu.cn")

	unsafeStorage := performJSON(router, http.MethodPost, "/api/v1/admin/materials", `{"courseId":"`+course.ID+`","title":"Unsafe","storageKey":"../secret.pdf"}`, adminToken)
	if unsafeStorage.Code != http.StatusBadRequest || !strings.Contains(unsafeStorage.Body.String(), "unsafe_storage_key") {
		t.Fatalf("expected unsafe storage key rejection, got %d: %s", unsafeStorage.Code, unsafeStorage.Body.String())
	}

	missingStorage := performJSON(router, http.MethodPost, "/api/v1/admin/materials", `{"courseId":"`+course.ID+`","title":"Missing","storageKey":"materials/missing.pdf"}`, adminToken)
	if missingStorage.Code != http.StatusBadRequest || !strings.Contains(missingStorage.Body.String(), "file_not_found") {
		t.Fatalf("expected missing storage file rejection, got %d: %s", missingStorage.Code, missingStorage.Body.String())
	}

	writeUploadFile(t, cfg.LocalUploadDir, "materials/manual/run.exe", "MZ")
	unsupportedStorage := performJSON(router, http.MethodPost, "/api/v1/admin/materials", `{"courseId":"`+course.ID+`","title":"Executable","storageKey":"materials/manual/run.exe"}`, adminToken)
	if unsupportedStorage.Code != http.StatusBadRequest || !strings.Contains(unsupportedStorage.Body.String(), "unsupported_file_type") {
		t.Fatalf("expected unsupported manual storage type rejection, got %d: %s", unsupportedStorage.Code, unsupportedStorage.Body.String())
	}

	writeUploadFile(t, cfg.LocalUploadDir, "materials/manual/bad.pdf", "not a pdf")
	invalidStorageContent := performJSON(router, http.MethodPost, "/api/v1/admin/materials", `{"courseId":"`+course.ID+`","title":"Bad PDF","storageKey":"materials/manual/bad.pdf"}`, adminToken)
	if invalidStorageContent.Code != http.StatusBadRequest || !strings.Contains(invalidStorageContent.Body.String(), "invalid_file_content") {
		t.Fatalf("expected invalid manual storage content rejection, got %d: %s", invalidStorageContent.Code, invalidStorageContent.Body.String())
	}

	writeUploadFile(t, cfg.LocalUploadDir, "materials/manual/safe.txt", "plain text")
	unsafeManualFileName := performJSON(router, http.MethodPost, "/api/v1/admin/materials", `{"courseId":"`+course.ID+`","title":"Unsafe Name","storageKey":"materials/manual/safe.txt","fileName":"../safe.txt"}`, adminToken)
	if unsafeManualFileName.Code != http.StatusBadRequest || !strings.Contains(unsafeManualFileName.Body.String(), "unsafe_file_name") {
		t.Fatalf("expected unsafe manual fileName rejection, got %d: %s", unsafeManualFileName.Code, unsafeManualFileName.Body.String())
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

	invalidText := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId": course.ID,
		"title":    "Invalid Text",
	}, "file", "binary.txt", []byte{'o', 'k', 0, 'x'})
	if invalidText.Code != http.StatusBadRequest || !strings.Contains(invalidText.Body.String(), "invalid_file_content") {
		t.Fatalf("expected invalid text rejection, got %d: %s", invalidText.Code, invalidText.Body.String())
	}

	invalidDOCX := performMultipart(router, "/api/v1/admin/materials/upload", adminToken, map[string]string{
		"courseId": course.ID,
		"title":    "Invalid DOCX",
	}, "file", "bad.docx", []byte("not a zip"))
	if invalidDOCX.Code != http.StatusBadRequest || !strings.Contains(invalidDOCX.Body.String(), "invalid_file_content") {
		t.Fatalf("expected invalid docx rejection, got %d: %s", invalidDOCX.Code, invalidDOCX.Body.String())
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
	if countOperationLogs(t, db, "material.upload", "material", material.ID, admin.ID) != 1 {
		t.Fatal("expected material upload operation log")
	}
}

func TestAdminMaterialStatusFlow(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.LocalUploadDir = t.TempDir()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	admin := createTestUser(t, db, "material-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "material-admin@stu.henu.edu.cn")
	studentToken := loginTestUser(t, router, "material-student@stu.henu.edu.cn")

	writeUploadFile(t, cfg.LocalUploadDir, "materials/draft-material.txt", "draft content")
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
	if material.FileName != "draft-material.txt" || material.FileSize != int64(len("draft content")) {
		t.Fatalf("expected file metadata derived from storage reference, got fileName=%q fileSize=%d", material.FileName, material.FileSize)
	}
	if countOperationLogs(t, db, "material.create", "material", material.ID, admin.ID) != 1 {
		t.Fatal("expected material create operation log")
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

	originalStorageKey := material.StorageKey
	blockedFileUpdate := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID, `{"storageKey":"materials/other.txt","fileName":"other.txt","fileSize":42}`, adminToken)
	if blockedFileUpdate.Code != http.StatusBadRequest || !strings.Contains(blockedFileUpdate.Body.String(), "material_file_fields_immutable") {
		t.Fatalf("expected material file fields immutable rejection, got %d: %s", blockedFileUpdate.Code, blockedFileUpdate.Body.String())
	}
	blockedSnakeCaseUpdate := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID, `{"storage_key":"materials/other.txt"}`, adminToken)
	if blockedSnakeCaseUpdate.Code != http.StatusBadRequest || !strings.Contains(blockedSnakeCaseUpdate.Body.String(), "material_file_fields_immutable") {
		t.Fatalf("expected snake_case file field immutable rejection, got %d: %s", blockedSnakeCaseUpdate.Code, blockedSnakeCaseUpdate.Body.String())
	}
	if err := db.First(&material, "id = ?", material.ID).Error; err != nil {
		t.Fatal(err)
	}
	if material.StorageKey != originalStorageKey || material.FileName != "draft-material.txt" || material.FileSize != int64(len("draft content")) {
		t.Fatalf("material file fields changed through metadata update: %#v", material)
	}
	if countOperationLogs(t, db, "material.update", "material", material.ID, admin.ID) != 0 {
		t.Fatal("blocked material file update must not write an operation log")
	}

	updateBody := `{"title":"Edited Material","type":"answer","description":"Edited description","previewContent":"Edited preview","accessLevel":"paid","status":"pending"}`
	update := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID, updateBody, adminToken)
	if update.Code != http.StatusOK {
		t.Fatalf("expected material edit 200, got %d: %s", update.Code, update.Body.String())
	}
	if countOperationLogs(t, db, "material.update", "material", material.ID, admin.ID) != 1 {
		t.Fatal("expected material update operation log")
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

	if err := os.Remove(filepath.Join(cfg.LocalUploadDir, filepath.FromSlash(material.StorageKey))); err != nil {
		t.Fatal(err)
	}
	missingFilePublish := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID+"/status", `{"status":"published"}`, adminToken)
	if missingFilePublish.Code != http.StatusBadRequest || !strings.Contains(missingFilePublish.Body.String(), "file_not_found") {
		t.Fatalf("expected missing file publish rejection, got %d: %s", missingFilePublish.Code, missingFilePublish.Body.String())
	}
	if countOperationLogs(t, db, "material.status_update", "material", material.ID, admin.ID) != 0 {
		t.Fatal("failed publish must not write material status operation log")
	}
	writeUploadFile(t, cfg.LocalUploadDir, material.StorageKey, "draft content restored")
	publish := performJSON(router, http.MethodPatch, "/api/v1/admin/materials/"+material.ID+"/status", `{"status":"published"}`, adminToken)
	if publish.Code != http.StatusOK {
		t.Fatalf("expected publish status update 200, got %d: %s", publish.Code, publish.Body.String())
	}
	if countOperationLogs(t, db, "material.status_update", "material", material.ID, admin.ID) != 1 {
		t.Fatal("expected material status operation log")
	}
	publicListAfterPublish := performJSON(router, http.MethodGet, "/api/v1/materials", "", "")
	if publicListAfterPublish.Code != http.StatusOK || !strings.Contains(publicListAfterPublish.Body.String(), material.ID) {
		t.Fatalf("expected published material in public list, got %d: %s", publicListAfterPublish.Code, publicListAfterPublish.Body.String())
	}
}

func TestMaterialReviewWorkflowRequiresReviewerAndRecordsDecision(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.LocalUploadDir = t.TempDir()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	course := createTestCourse(t, db)
	writeUploadFile(t, cfg.LocalUploadDir, "materials/pending-review.txt", "pending review content")
	pendingMaterial := model.Material{
		CourseID:       course.ID,
		Title:          "Pending review material",
		Type:           "knowledge_note",
		StorageKey:     "materials/pending-review.txt",
		FileName:       "pending-review.txt",
		AccessLevel:    model.MaterialAccessFree,
		PreviewContent: "pending preview",
		Status:         model.StatusPending,
	}
	if err := db.Create(&pendingMaterial).Error; err != nil {
		t.Fatal(err)
	}

	studentToken := loginTestUser(t, router, "material-review-student@stu.henu.edu.cn")
	forbiddenList := performJSON(router, http.MethodGet, "/api/v1/admin/material-reviews", "", studentToken)
	if forbiddenList.Code != http.StatusForbidden {
		t.Fatalf("expected student material review list 403, got %d: %s", forbiddenList.Code, forbiddenList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/materials/"+pendingMaterial.ID+"/approve", `{"reviewReason":"ok"}`, studentToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student material approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	reviewer := createTestUser(t, db, "material-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, "material-reviewer@stu.henu.edu.cn")
	reviewerAdminMaterials := performJSON(router, http.MethodGet, "/api/v1/admin/materials", "", reviewerToken)
	if reviewerAdminMaterials.Code != http.StatusForbidden {
		t.Fatalf("expected reviewer to remain blocked from material CRUD list, got %d: %s", reviewerAdminMaterials.Code, reviewerAdminMaterials.Body.String())
	}
	reviewList := performJSON(router, http.MethodGet, "/api/v1/admin/material-reviews", "", reviewerToken)
	if reviewList.Code != http.StatusOK || !strings.Contains(reviewList.Body.String(), pendingMaterial.ID) {
		t.Fatalf("expected reviewer material review list to include pending material, got %d: %s", reviewList.Code, reviewList.Body.String())
	}

	missingFileMaterial := model.Material{
		CourseID:    course.ID,
		Title:       "Missing file review material",
		Type:        "knowledge_note",
		StorageKey:  "materials/missing-review.txt",
		FileName:    "missing-review.txt",
		AccessLevel: model.MaterialAccessFree,
		Status:      model.StatusPending,
	}
	if err := db.Create(&missingFileMaterial).Error; err != nil {
		t.Fatal(err)
	}
	missingApprove := performJSON(router, http.MethodPost, "/api/v1/admin/materials/"+missingFileMaterial.ID+"/approve", `{"reviewReason":"checked"}`, reviewerToken)
	if missingApprove.Code != http.StatusBadRequest || !strings.Contains(missingApprove.Body.String(), "file_not_found") {
		t.Fatalf("expected missing file approve rejection, got %d: %s", missingApprove.Code, missingApprove.Body.String())
	}
	if countOperationLogs(t, db, "material.approved", "material", missingFileMaterial.ID, reviewer.ID) != 0 {
		t.Fatal("failed material approval must not write operation log")
	}

	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/materials/"+pendingMaterial.ID+"/reject", "", reviewerToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected reject reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/materials/"+pendingMaterial.ID+"/approve", `{"reviewReason":"checked ok"}`, reviewerToken)
	if approve.Code != http.StatusOK {
		t.Fatalf("expected reviewer material approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approved model.Material
	if err := db.First(&approved, "id = ?", pendingMaterial.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approved.Status != model.StatusPublished || approved.ReviewerID == nil || *approved.ReviewerID != reviewer.ID || approved.ReviewedAt == nil || approved.ReviewReason != "checked ok" {
		t.Fatalf("expected approved material review metadata, got %#v", approved)
	}
	if countOperationLogs(t, db, "material.approved", "material", pendingMaterial.ID, reviewer.ID) != 1 {
		t.Fatal("expected material approval operation log")
	}
	publicApproved := performJSON(router, http.MethodGet, "/api/v1/materials/"+pendingMaterial.ID, "", "")
	if publicApproved.Code != http.StatusOK {
		t.Fatalf("expected approved material public detail 200, got %d: %s", publicApproved.Code, publicApproved.Body.String())
	}
	for _, hiddenField := range []string{"reviewerId", "reviewedAt", "reviewReason", "createdBy", "storageKey"} {
		if strings.Contains(publicApproved.Body.String(), hiddenField) {
			t.Fatalf("public material detail leaked %s: %s", hiddenField, publicApproved.Body.String())
		}
	}
	reviewApprovedAgain := performJSON(router, http.MethodPost, "/api/v1/admin/materials/"+pendingMaterial.ID+"/reject", `{"reviewReason":"second attempt"}`, reviewerToken)
	if reviewApprovedAgain.Code != http.StatusConflict || !strings.Contains(reviewApprovedAgain.Body.String(), "material_not_reviewable") {
		t.Fatalf("expected reviewed material to reject repeat review, got %d: %s", reviewApprovedAgain.Code, reviewApprovedAgain.Body.String())
	}
	if countOperationLogs(t, db, "material.rejected", "material", pendingMaterial.ID, reviewer.ID) != 0 {
		t.Fatal("repeat review must not write rejected operation log")
	}

	secondMaterial := model.Material{
		CourseID:       course.ID,
		Title:          "Rejected review material",
		Type:           "mock_paper",
		StorageKey:     "materials/rejected-review.txt",
		FileName:       "rejected-review.txt",
		AccessLevel:    model.MaterialAccessPaid,
		PreviewContent: "needs work",
		Status:         model.StatusPending,
	}
	if err := db.Create(&secondMaterial).Error; err != nil {
		t.Fatal(err)
	}
	admin := createTestUser(t, db, "material-review-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "material-review-admin@stu.henu.edu.cn")
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/materials/"+secondMaterial.ID+"/reject", `{"reviewReason":"missing answer key"}`, adminToken)
	if reject.Code != http.StatusOK {
		t.Fatalf("expected admin material reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var rejected model.Material
	if err := db.First(&rejected, "id = ?", secondMaterial.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.Status != model.StatusRejected || rejected.ReviewerID == nil || *rejected.ReviewerID != admin.ID || rejected.ReviewReason != "missing answer key" {
		t.Fatalf("expected rejected material review metadata, got %#v", rejected)
	}
	if countOperationLogs(t, db, "material.rejected", "material", secondMaterial.ID, admin.ID) != 1 {
		t.Fatal("expected material rejection operation log")
	}
	publicRejected := performJSON(router, http.MethodGet, "/api/v1/materials/"+secondMaterial.ID, "", "")
	if publicRejected.Code != http.StatusNotFound {
		t.Fatalf("expected rejected material hidden from public detail, got %d: %s", publicRejected.Code, publicRejected.Body.String())
	}
}

func TestAdminOperationLogsRequiresAdminAndFilters(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "logs-admin@stu.henu.edu.cn", model.RoleAdmin)
	otherAdmin := createTestUser(t, db, "logs-other-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "logs-admin@stu.henu.edu.cn")
	studentToken := loginTestUser(t, router, "logs-student@stu.henu.edu.cn")

	targetID := "course-log-target"
	logs := []model.OperationLog{
		{
			OperatorID: admin.ID,
			Action:     "course.update",
			TargetType: "course",
			TargetID:   targetID,
			IP:         "192.0.2.10",
			UserAgent:  "test-agent",
		},
		{
			OperatorID: otherAdmin.ID,
			Action:     "material.update",
			TargetType: "material",
			TargetID:   "material-log-target",
		},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	denied := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs", "", studentToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("expected student operation logs 403, got %d: %s", denied.Code, denied.Body.String())
	}

	filtered := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs?action=course.update&targetType=course&targetId="+targetID+"&operatorId="+admin.ID+"&limit=10", "", adminToken)
	if filtered.Code != http.StatusOK {
		t.Fatalf("expected operation log filter 200, got %d: %s", filtered.Code, filtered.Body.String())
	}
	body := filtered.Body.String()
	if !strings.Contains(body, `"operationLogs"`) || !strings.Contains(body, logs[0].ID) {
		t.Fatalf("expected filtered operation log in response, got %s", body)
	}
	if strings.Contains(body, logs[1].ID) {
		t.Fatalf("filtered operation logs leaked unrelated row: %s", body)
	}

	invalidLimit := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs?limit=0", "", adminToken)
	if invalidLimit.Code != http.StatusBadRequest || !strings.Contains(invalidLimit.Body.String(), "invalid_limit") {
		t.Fatalf("expected invalid limit rejection, got %d: %s", invalidLimit.Code, invalidLimit.Body.String())
	}
}

func TestAdminOperationLogExportAndRetentionPolicy(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.OperationLogRetentionDays = 30
	cfg.OperationLogExportLimit = 2
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	admin := createTestUser(t, db, "log-export-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "log-export-admin@stu.henu.edu.cn")
	studentToken := loginTestUser(t, router, "log-export-student@stu.henu.edu.cn")

	oldTime := time.Date(2026, 6, 20, 8, 0, 0, 0, time.UTC)
	midTime := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)
	logs := []model.OperationLog{
		{BaseModel: model.BaseModel{CreatedAt: oldTime}, OperatorID: admin.ID, Action: "=danger", TargetType: "material", TargetID: "old-log", IP: "192.0.2.1", UserAgent: "old-agent"},
		{BaseModel: model.BaseModel{CreatedAt: midTime}, OperatorID: admin.ID, Action: "material.approved", TargetType: "material", TargetID: "mid-log", IP: "192.0.2.2", UserAgent: "mid-agent"},
		{BaseModel: model.BaseModel{CreatedAt: newTime}, OperatorID: admin.ID, Action: "material.rejected", TargetType: "material", TargetID: "new-log", IP: "192.0.2.3", UserAgent: "new-agent"},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	deniedExport := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs/export", "", studentToken)
	if deniedExport.Code != http.StatusForbidden {
		t.Fatalf("expected student operation log export 403, got %d: %s", deniedExport.Code, deniedExport.Body.String())
	}
	invalidDate := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs/export?createdFrom=not-a-date", "", adminToken)
	if invalidDate.Code != http.StatusBadRequest || !strings.Contains(invalidDate.Body.String(), "invalid_created_from") {
		t.Fatalf("expected invalid export date rejection, got %d: %s", invalidDate.Code, invalidDate.Body.String())
	}

	exportResponse := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs/export?targetType=material&createdFrom=2026-06-21&limit=10", "", adminToken)
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("expected operation log export 200, got %d: %s", exportResponse.Code, exportResponse.Body.String())
	}
	if !strings.Contains(exportResponse.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("expected csv content type, got %s", exportResponse.Header().Get("Content-Type"))
	}
	if !strings.Contains(exportResponse.Header().Get("Content-Disposition"), "operation-logs-") {
		t.Fatalf("expected operation log attachment header, got %s", exportResponse.Header().Get("Content-Disposition"))
	}
	reader := csv.NewReader(strings.NewReader(exportResponse.Body.String()))
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("expected header plus 2 capped export rows, got %#v", records)
	}
	if records[0][0] != "id" || records[0][3] != "action" {
		t.Fatalf("unexpected csv header: %#v", records[0])
	}
	csvBody := exportResponse.Body.String()
	if strings.Contains(csvBody, "old-log") {
		t.Fatalf("export leaked row before createdFrom filter: %s", csvBody)
	}
	if !strings.Contains(csvBody, "mid-log") || !strings.Contains(csvBody, "new-log") {
		t.Fatalf("expected filtered rows in export, got %s", csvBody)
	}

	formulaExport := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs/export?action==danger&limit=1", "", adminToken)
	if formulaExport.Code != http.StatusOK {
		t.Fatalf("expected formula export 200, got %d: %s", formulaExport.Code, formulaExport.Body.String())
	}
	if !strings.Contains(formulaExport.Body.String(), "'=danger") {
		t.Fatalf("expected formula-like CSV cell to be escaped, got %s", formulaExport.Body.String())
	}

	retention := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs/retention", "", adminToken)
	if retention.Code != http.StatusOK || !strings.Contains(retention.Body.String(), `"retentionDays":30`) || !strings.Contains(retention.Body.String(), `"exportLimit":2`) {
		t.Fatalf("expected configured retention policy, got %d: %s", retention.Code, retention.Body.String())
	}
	studentRetention := performJSON(router, http.MethodGet, "/api/v1/admin/operation-logs/retention", "", studentToken)
	if studentRetention.Code != http.StatusForbidden {
		t.Fatalf("expected student operation log retention 403, got %d: %s", studentRetention.Code, studentRetention.Body.String())
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
