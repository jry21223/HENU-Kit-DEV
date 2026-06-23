package tests

import (
	"net/http"
	"strings"
	"testing"

	"gorm.io/datatypes"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAITaskCreateQueryAndIsolation(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)

	unauthorized := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", `{"type":"wrong_question_analysis"}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated AI task create 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	studentToken := loginTestUser(t, router, "student@stu.henu.edu.cn")
	invalidTypeResponse := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", `{"type":"auto_publish_answers"}`, studentToken)
	if invalidTypeResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported AI task type 400, got %d: %s", invalidTypeResponse.Code, invalidTypeResponse.Body.String())
	}

	createBody := `{"courseId":"` + course.ID + `","type":"wrong_question_analysis","input":{"wrongQuestionCount":2}}`
	createResponse := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", createBody, studentToken)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("expected AI task create 200, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	if !strings.Contains(createResponse.Body.String(), `"status":"pending"`) {
		t.Fatalf("expected pending task, got %s", createResponse.Body.String())
	}

	var task model.AITask
	if err := db.First(&task, "type = ?", "wrong_question_analysis").Error; err != nil {
		t.Fatal(err)
	}

	queryResponse := performJSON(router, http.MethodGet, "/api/v1/ai/tasks/"+task.ID, "", studentToken)
	if queryResponse.Code != http.StatusOK {
		t.Fatalf("expected owner AI task query 200, got %d: %s", queryResponse.Code, queryResponse.Body.String())
	}

	otherToken := loginTestUser(t, router, "other@stu.henu.edu.cn")
	forbidden := performJSON(router, http.MethodGet, "/api/v1/ai/tasks/"+task.ID, "", otherToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected other user AI task query 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}

	createTestUser(t, db, "admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "admin@stu.henu.edu.cn")
	adminList := performJSON(router, http.MethodGet, "/api/v1/admin/ai/tasks", "", adminToken)
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), task.ID) {
		t.Fatalf("expected admin AI task list, got %d: %s", adminList.Code, adminList.Body.String())
	}
}

func TestAdminAIDraftReviewRequiresAdminAndDoesNotPublish(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	student := createTestUser(t, db, "student-owner@stu.henu.edu.cn", model.RoleUser)

	task := model.AITask{
		UserID:   &student.ID,
		CourseID: &course.ID,
		Type:     "paper_generation",
		Status:   model.AITaskCompleted,
		Input:    datatypes.JSON([]byte(`{"topic":"sets"}`)),
		Result:   datatypes.JSON([]byte(`{"title":"Mock paper"}`)),
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	draft := model.AIDraft{
		TaskID:       task.ID,
		CourseID:     &course.ID,
		OutputType:   "paper_generation",
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		DraftContent: datatypes.JSON([]byte(`{"title":"AI draft must be reviewed"}`)),
	}
	if err := db.Create(&draft).Error; err != nil {
		t.Fatal(err)
	}

	studentToken := loginTestUser(t, router, "student-owner@stu.henu.edu.cn")
	forbiddenList := performJSON(router, http.MethodGet, "/api/v1/admin/ai/drafts", "", studentToken)
	if forbiddenList.Code != http.StatusForbidden {
		t.Fatalf("expected student draft list 403, got %d: %s", forbiddenList.Code, forbiddenList.Body.String())
	}
	forbiddenApprove := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+draft.ID+"/approve", "", studentToken)
	if forbiddenApprove.Code != http.StatusForbidden {
		t.Fatalf("expected student draft approve 403, got %d: %s", forbiddenApprove.Code, forbiddenApprove.Body.String())
	}

	admin := createTestUser(t, db, "ai-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "ai-admin@stu.henu.edu.cn")
	adminList := performJSON(router, http.MethodGet, "/api/v1/admin/ai/drafts", "", adminToken)
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), draft.ID) {
		t.Fatalf("expected admin AI draft list, got %d: %s", adminList.Code, adminList.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+draft.ID+"/approve", "", adminToken)
	if approve.Code != http.StatusOK || !strings.Contains(approve.Body.String(), model.StatusApproved) {
		t.Fatalf("expected admin draft approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approved model.AIDraft
	if err := db.First(&approved, "id = ?", draft.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approved.Status != model.StatusApproved {
		t.Fatalf("expected approved draft, got %s", approved.Status)
	}
	if approved.ReviewerID == nil || *approved.ReviewerID != admin.ID {
		t.Fatalf("expected reviewer id %s, got %#v", admin.ID, approved.ReviewerID)
	}
	if approved.PublishedID != nil || approved.Status == model.StatusPublished {
		t.Fatalf("AI draft review must not auto-publish generated content: %#v", approved)
	}

	secondDraft := model.AIDraft{
		TaskID:       task.ID,
		CourseID:     &course.ID,
		OutputType:   "targeted_question",
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		DraftContent: datatypes.JSON([]byte(`{"stem":"Mock question"}`)),
	}
	if err := db.Create(&secondDraft).Error; err != nil {
		t.Fatal(err)
	}
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+secondDraft.ID+"/reject", "", adminToken)
	if reject.Code != http.StatusOK || !strings.Contains(reject.Body.String(), model.StatusRejected) {
		t.Fatalf("expected admin draft reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
}
