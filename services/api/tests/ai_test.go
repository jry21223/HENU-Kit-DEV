package tests

import (
	"net/http"
	"strings"
	"testing"

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
