package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

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
	if err := db.Model(&model.User{}).Where("email = ?", "student@stu.henu.edu.cn").Update("points_balance", 20).Error; err != nil {
		t.Fatal(err)
	}

	createBody := `{"courseId":"` + course.ID + `","type":"wrong_question_analysis","input":{"wrongQuestionCount":2}}`
	createResponse := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", createBody, studentToken)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("expected AI task create 200, got %d: %s", createResponse.Code, createResponse.Body.String())
	}
	if !strings.Contains(createResponse.Body.String(), `"status":"pending"`) {
		t.Fatalf("expected pending task, got %s", createResponse.Body.String())
	}
	if !strings.Contains(createResponse.Body.String(), `"pointsCost":5`) || !strings.Contains(createResponse.Body.String(), `"balanceAfter":15`) {
		t.Fatalf("expected AI task to deduct points, got %s", createResponse.Body.String())
	}

	var task model.AITask
	if err := db.First(&task, "type = ?", "wrong_question_analysis").Error; err != nil {
		t.Fatal(err)
	}
	var student model.User
	if err := db.First(&student, "email = ?", "student@stu.henu.edu.cn").Error; err != nil {
		t.Fatal(err)
	}
	if student.PointsBalance != 15 {
		t.Fatalf("expected AI task to leave 15 points, got %d", student.PointsBalance)
	}
	var usage model.AIUsageLog
	if err := db.First(&usage, "task_id = ? AND model = ?", task.ID, "quota").Error; err != nil {
		t.Fatal(err)
	}
	if usage.Source != "points" || usage.PointsCost != 5 {
		t.Fatalf("unexpected AI quota usage log: %#v", usage)
	}
	var pointsLogCount int64
	if err := db.Model(&model.PointsLog{}).Where("user_id = ? AND reason = ? AND reference_type = ? AND reference_id = ? AND delta = ?", student.ID, "ai_task_usage", "ai_task", task.ID, -5).Count(&pointsLogCount).Error; err != nil {
		t.Fatal(err)
	}
	if pointsLogCount != 1 {
		t.Fatalf("expected one AI points deduction log, got %d", pointsLogCount)
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

func TestAITaskQuotaMembershipAndPoints(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)

	noPointsToken := loginTestUser(t, router, "ai-no-points@stu.henu.edu.cn")
	insufficient := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", `{"type":"paper_generation","input":{"topic":"graphs"}}`, noPointsToken)
	if insufficient.Code != http.StatusBadRequest || !strings.Contains(insufficient.Body.String(), "insufficient_ai_points") {
		t.Fatalf("expected insufficient points for paper generation, got %d: %s", insufficient.Code, insufficient.Body.String())
	}
	var taskCount int64
	if err := db.Model(&model.AITask{}).Where("type = ?", "paper_generation").Count(&taskCount).Error; err != nil {
		t.Fatal(err)
	}
	if taskCount != 0 {
		t.Fatalf("insufficient points must rollback AI task creation, got %d tasks", taskCount)
	}

	tier2User := createTestUser(t, db, "ai-tier2@stu.henu.edu.cn", model.RoleUser)
	expiresAt := time.Now().Add(24 * time.Hour)
	if err := db.Create(&model.Membership{UserID: tier2User.ID, PlanCode: "tier2", Status: "active", Source: "test", ExpiresAt: &expiresAt}).Error; err != nil {
		t.Fatal(err)
	}
	tier2Token := loginTestUser(t, router, tier2User.Email)
	tier2Create := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", `{"type":"paper_generation","input":{"topic":"sets"}}`, tier2Token)
	if tier2Create.Code != http.StatusOK || !strings.Contains(tier2Create.Body.String(), `"source":"membership_tier2"`) || !strings.Contains(tier2Create.Body.String(), `"pointsCost":0`) {
		t.Fatalf("expected tier2 membership to create paper task without points, got %d: %s", tier2Create.Code, tier2Create.Body.String())
	}
	var tier2Payload struct {
		Data struct {
			Task model.AITask `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(tier2Create.Body.Bytes(), &tier2Payload); err != nil {
		t.Fatal(err)
	}
	var tier2Usage model.AIUsageLog
	if err := db.First(&tier2Usage, "task_id = ? AND source = ?", tier2Payload.Data.Task.ID, "membership_tier2").Error; err != nil {
		t.Fatal(err)
	}
	if tier2Usage.PointsCost != 0 {
		t.Fatalf("expected free tier2 usage, got %#v", tier2Usage)
	}

	tier1User := createTestUser(t, db, "ai-tier1@stu.henu.edu.cn", model.RoleUser)
	if err := db.Model(&model.User{}).Where("id = ?", tier1User.ID).Update("points_balance", 20).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Membership{UserID: tier1User.ID, PlanCode: "tier1", Status: "active", Source: "test", ExpiresAt: &expiresAt}).Error; err != nil {
		t.Fatal(err)
	}
	tier1Token := loginTestUser(t, router, tier1User.Email)
	wrongAnalysis := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", `{"type":"wrong_question_analysis","input":{"count":1}}`, tier1Token)
	if wrongAnalysis.Code != http.StatusOK || !strings.Contains(wrongAnalysis.Body.String(), `"source":"membership_tier1"`) || !strings.Contains(wrongAnalysis.Body.String(), `"pointsCost":0`) {
		t.Fatalf("expected tier1 wrong-question analysis to be free, got %d: %s", wrongAnalysis.Code, wrongAnalysis.Body.String())
	}
	paper := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", `{"type":"paper_generation","input":{"topic":"relations"}}`, tier1Token)
	if paper.Code != http.StatusOK || !strings.Contains(paper.Body.String(), `"source":"membership_tier1_discount"`) || !strings.Contains(paper.Body.String(), `"pointsCost":15`) || !strings.Contains(paper.Body.String(), `"balanceAfter":5`) {
		t.Fatalf("expected tier1 paper generation discount, got %d: %s", paper.Code, paper.Body.String())
	}

	admin := createTestUser(t, db, "ai-role-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, admin.Email)
	adminCreate := performJSON(router, http.MethodPost, "/api/v1/ai/tasks", `{"type":"paper_generation","input":{"topic":"admin"}}`, adminToken)
	if adminCreate.Code != http.StatusOK || !strings.Contains(adminCreate.Body.String(), `"source":"role_exempt"`) || !strings.Contains(adminCreate.Body.String(), `"pointsCost":0`) {
		t.Fatalf("expected admin AI task quota exemption, got %d: %s", adminCreate.Code, adminCreate.Body.String())
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

	admin := createTestUser(t, db, "ai-admin@stu.henu.edu.cn", model.RoleAdmin)
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
	if countOperationLogs(t, db, "ai_draft.approved", "ai_draft", draft.ID, reviewer.ID) != 1 {
		t.Fatal("expected AI draft approve operation log")
	}
	reviewApprovedAgain := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+draft.ID+"/reject", `{"reviewReason":"overwrite attempt"}`, adminToken)
	if reviewApprovedAgain.Code != http.StatusConflict || !strings.Contains(reviewApprovedAgain.Body.String(), "draft_not_reviewable") {
		t.Fatalf("expected approved draft repeat review 409, got %d: %s", reviewApprovedAgain.Code, reviewApprovedAgain.Body.String())
	}
	if countOperationLogs(t, db, "ai_draft.rejected", "ai_draft", draft.ID, admin.ID) != 0 {
		t.Fatal("repeat review on approved draft must not write reject operation log")
	}
	var stillApproved model.AIDraft
	if err := db.First(&stillApproved, "id = ?", draft.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stillApproved.Status != model.StatusApproved || stillApproved.ReviewReason != "checked ok" || stillApproved.ReviewerID == nil || *stillApproved.ReviewerID != reviewer.ID {
		t.Fatalf("repeat review mutated approved draft: %#v", stillApproved)
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
	if countOperationLogs(t, db, "ai_draft.rejected", "ai_draft", secondDraft.ID, admin.ID) != 1 {
		t.Fatal("expected AI draft reject operation log")
	}
	reviewRejectedAgain := performJSON(router, http.MethodPost, "/api/v1/admin/ai/drafts/"+secondDraft.ID+"/approve", `{"reviewReason":"second attempt"}`, reviewerToken)
	if reviewRejectedAgain.Code != http.StatusConflict || !strings.Contains(reviewRejectedAgain.Body.String(), "draft_not_reviewable") {
		t.Fatalf("expected rejected draft repeat review 409, got %d: %s", reviewRejectedAgain.Code, reviewRejectedAgain.Body.String())
	}
	if countOperationLogs(t, db, "ai_draft.approved", "ai_draft", secondDraft.ID, reviewer.ID) != 0 {
		t.Fatal("repeat review on rejected draft must not write approve operation log")
	}
	var stillRejected model.AIDraft
	if err := db.First(&stillRejected, "id = ?", secondDraft.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stillRejected.Status != model.StatusRejected || stillRejected.ReviewReason != "answer is incomplete" {
		t.Fatalf("repeat review mutated rejected draft: %#v", stillRejected)
	}
}
