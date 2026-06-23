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

func TestAIDraftReviewRequiresReviewerRoleAndDoesNotPublish(t *testing.T) {
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

	reviewer := createTestUser(t, db, "ai-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reviewerToken := loginTestUser(t, router, "ai-reviewer@stu.henu.edu.cn")
	reviewerTaskList := performJSON(router, http.MethodGet, "/api/v1/admin/ai/tasks", "", reviewerToken)
	if reviewerTaskList.Code != http.StatusOK || !strings.Contains(reviewerTaskList.Body.String(), task.ID) {
		t.Fatalf("expected reviewer AI task list, got %d: %s", reviewerTaskList.Code, reviewerTaskList.Body.String())
	}
	reviewerList := performJSON(router, http.MethodGet, "/api/v1/admin/ai/drafts", "", reviewerToken)
	if reviewerList.Code != http.StatusOK || !strings.Contains(reviewerList.Body.String(), draft.ID) {
		t.Fatalf("expected reviewer AI draft list, got %d: %s", reviewerList.Code, reviewerList.Body.String())
	}
	reviewerMaterials := performJSON(router, http.MethodGet, "/api/v1/admin/materials", "", reviewerToken)
	if reviewerMaterials.Code != http.StatusForbidden {
		t.Fatalf("expected reviewer material admin list 403, got %d: %s", reviewerMaterials.Code, reviewerMaterials.Body.String())
	}
	reviewerAnalytics := performJSON(router, http.MethodGet, "/api/v1/admin/analytics/overview", "", reviewerToken)
	if reviewerAnalytics.Code != http.StatusForbidden {
		t.Fatalf("expected reviewer analytics 403, got %d: %s", reviewerAnalytics.Code, reviewerAnalytics.Body.String())
	}

	createTestUser(t, db, "ai-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "ai-admin@stu.henu.edu.cn")
	adminList := performJSON(router, http.MethodGet, "/api/v1/admin/ai/drafts", "", adminToken)
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), draft.ID) {
		t.Fatalf("expected admin AI draft list, got %d: %s", adminList.Code, adminList.Body.String())
	}

	approve := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+draft.ID+"/approve", `{"reviewReason":"checked ok"}`, reviewerToken)
	if approve.Code != http.StatusOK || !strings.Contains(approve.Body.String(), model.StatusApproved) {
		t.Fatalf("expected reviewer draft approve 200, got %d: %s", approve.Code, approve.Body.String())
	}
	var approved model.AIDraft
	if err := db.First(&approved, "id = ?", draft.ID).Error; err != nil {
		t.Fatal(err)
	}
	if approved.Status != model.StatusApproved {
		t.Fatalf("expected approved draft, got %s", approved.Status)
	}
	if approved.ReviewerID == nil || *approved.ReviewerID != reviewer.ID {
		t.Fatalf("expected reviewer id %s, got %#v", reviewer.ID, approved.ReviewerID)
	}
	if approved.ReviewReason != "checked ok" {
		t.Fatalf("expected review reason to persist, got %q", approved.ReviewReason)
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
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+secondDraft.ID+"/reject", "", adminToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected reject without reason 400, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}
	reject := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+secondDraft.ID+"/reject", `{"reviewReason":"answer is incomplete"}`, adminToken)
	if reject.Code != http.StatusOK || !strings.Contains(reject.Body.String(), model.StatusRejected) {
		t.Fatalf("expected admin draft reject 200, got %d: %s", reject.Code, reject.Body.String())
	}
	var rejected model.AIDraft
	if err := db.First(&rejected, "id = ?", secondDraft.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rejected.ReviewReason != "answer is incomplete" {
		t.Fatalf("expected reject reason to persist, got %q", rejected.ReviewReason)
	}
}
