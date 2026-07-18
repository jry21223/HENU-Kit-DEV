package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAdminNoticeReviewRequiresExpectedVersionAndWritesAudit(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	admin := createTestUser(t, db, "notice-review-admin@stu.henu.edu.cn", model.RoleAdmin)
	token := loginTestUser(t, router, admin.Email)
	notice := model.CampusNotice{SourceID: uuid.NewString(), ExternalID: "review-001", Title: "待审核通知", ContentHash: strings.Repeat("a", 64), Status: "review_pending", DistributionStatus: "not_scheduled", Importance: "normal", Version: 1}
	if err := db.Create(&notice).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.CampusNoticeVersion{NoticeID: notice.ID, Version: 1, Title: notice.Title, Body: "审核通知正文", ContentHash: notice.ContentHash}).Error; err != nil {
		t.Fatal(err)
	}

	approved := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/admin/notices/"+notice.ID+"/reviews", `{"decision":"approve","expected_version":1}`, token, "notice-review-approve-001")
	if approved.Code != http.StatusOK || !strings.Contains(approved.Body.String(), `"status":"approved"`) {
		t.Fatalf("expected notice approval, got %d: %s", approved.Code, approved.Body.String())
	}
	conflict := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/admin/notices/"+notice.ID+"/reviews", `{"decision":"reject","reason":"过期","expected_version":1}`, token, "notice-review-conflict-002")
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "RESOURCE_VERSION_CONFLICT") {
		t.Fatalf("expected version conflict, got %d: %s", conflict.Code, conflict.Body.String())
	}
	if countOperationLogs(t, db, "campus_notice.reviewed", "campus_notice", notice.ID, admin.ID) != 1 {
		t.Fatal("expected one notice review audit log")
	}
}

func TestAdminSessionRevocationInvalidatesExistingJWT(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	admin := createTestUser(t, db, "session-admin@stu.henu.edu.cn", model.RoleAdmin)
	target := createTestUser(t, db, "session-target@stu.henu.edu.cn", model.RoleUser)
	adminToken := loginTestUser(t, router, admin.Email)
	targetToken := loginTestUser(t, router, target.Email)
	revoked := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/admin/users/"+target.ID+"/sessions/revoke", `{"expected_version":1}`, adminToken, "session-revoke-001")
	if revoked.Code != http.StatusOK {
		t.Fatalf("expected session revocation, got %d: %s", revoked.Code, revoked.Body.String())
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+targetToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), "session_revoked") {
		t.Fatalf("expected old JWT rejection, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminFeedbackUpdateClosesUnifiedOperationCase(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	admin := createTestUser(t, db, "feedback-admin@stu.henu.edu.cn", model.RoleAdmin)
	token := loginTestUser(t, router, admin.Email)
	dueAt := time.Now().UTC().Add(24 * time.Hour)
	feedback := model.PlatformFeedback{Category: "product", Summary: "需要处理", Content: "正文", Urgency: model.UrgencyUrgent, Status: "new", DueAt: dueAt, Version: 1}
	if err := db.Create(&feedback).Error; err != nil {
		t.Fatal(err)
	}
	operationCase := model.OperationCase{SourceService: "feedback", SourceType: "platform_feedback", SourceID: feedback.ID, Summary: feedback.Summary, Urgency: feedback.Urgency, Status: "open", DueAt: dueAt, ActionPath: "/feedback", Version: 1}
	if err := db.Create(&operationCase).Error; err != nil {
		t.Fatal(err)
	}

	response := performJSONWithIdempotency(router, http.MethodPatch, "/api/v1/admin/platform-feedback/"+feedback.ID, `{"status":"resolved","expected_version":1}`, token, "feedback-resolve-001")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"resolved"`) {
		t.Fatalf("expected feedback resolution, got %d: %s", response.Code, response.Body.String())
	}
	if err := db.First(&operationCase, "id = ?", operationCase.ID).Error; err != nil {
		t.Fatal(err)
	}
	if operationCase.Status != "resolved" || operationCase.ResolvedAt == nil {
		t.Fatalf("expected unified operation case resolved, got %#v", operationCase)
	}
}

func TestQuizFeedbackResolutionRequiresAllThreeVerifications(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	admin := createTestUser(t, db, "quiz-feedback-admin@stu.henu.edu.cn", model.RoleAdmin)
	token := loginTestUser(t, router, admin.Email)
	report := model.Report{ReviewFields: model.ReviewFields{Status: model.StatusPending}, ReporterID: admin.ID, TargetType: "quiz_question", TargetID: uuid.NewString(), Reason: "answer mismatch", TargetSnapshot: datatypes.JSON(`{"question_id":"snapshot"}`), Version: 1}
	if err := db.Create(&report).Error; err != nil {
		t.Fatal(err)
	}
	operationCase := model.OperationCase{SourceService: "quizcraft", SourceType: "quiz_feedback", SourceID: report.ID, Summary: report.Reason, Urgency: model.UrgencyNormal, Status: "open", DueAt: time.Now().UTC().Add(72 * time.Hour), ActionPath: "/feedback", Version: 1}
	if err := db.Create(&operationCase).Error; err != nil {
		t.Fatal(err)
	}
	blocked := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/admin/quiz-feedback/"+report.ID+"/resolutions", `{"expected_version":1}`, token, "quiz-feedback-blocked-001")
	if blocked.Code != http.StatusConflict || !strings.Contains(blocked.Body.String(), "QUIZ_FEEDBACK_RESOLUTION_BLOCKED") {
		t.Fatalf("expected verification gate, got %d: %s", blocked.Code, blocked.Body.String())
	}
	if err := db.Model(&report).Updates(map[string]any{"json_verified": true, "postgres_verified": true, "api_verified": true}).Error; err != nil {
		t.Fatal(err)
	}
	resolved := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/admin/quiz-feedback/"+report.ID+"/resolutions", `{"expected_version":1}`, token, "quiz-feedback-resolved-002")
	if resolved.Code != http.StatusOK || !strings.Contains(resolved.Body.String(), `"status":"approved"`) {
		t.Fatalf("expected verified resolution, got %d: %s", resolved.Code, resolved.Body.String())
	}
	if err := db.First(&operationCase, "id = ?", operationCase.ID).Error; err != nil {
		t.Fatal(err)
	}
	if operationCase.Status != "resolved" {
		t.Fatalf("expected operation case closed, got %s", operationCase.Status)
	}
}

func TestFoodCandidatePolicyUsesValidVotesOnly(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	admin := createTestUser(t, db, "food-policy-admin@stu.henu.edu.cn", model.RoleAdmin)
	token := loginTestUser(t, router, admin.Email)
	promotedTier := model.FoodTierDefinition{Code: "promoted-tier", Name: "更高档", SortOrder: 10, Enabled: true}
	if err := db.Create(&promotedTier).Error; err != nil {
		t.Fatal(err)
	}
	tier := model.FoodTierDefinition{Code: "test-tier", Name: "测试档", SortOrder: 20, Enabled: true}
	if err := db.Create(&tier).Error; err != nil {
		t.Fatal(err)
	}
	entry := model.FoodEntry{SubmissionID: uuid.NewString(), Name: "测试餐厅", Location: "东门", InitialTierID: tier.ID, CurrentTierID: tier.ID, Version: 1}
	if err := db.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	round := model.FoodCalibrationRound{EntryID: entry.ID, RoundNumber: 1, Status: "open", PolicyVersion: "food_calibration_v1", OpenedAt: time.Now().UTC()}
	if err := db.Create(&round).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 10; index++ {
		position := "about_right"
		if index < 7 {
			position = "underrated"
		}
		vote := model.FoodCalibrationVote{RoundID: round.ID, UserID: uuid.NewString(), Position: position, Status: "valid"}
		if err := db.Create(&vote).Error; err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < 5; index++ {
		vote := model.FoodCalibrationVote{RoundID: round.ID, UserID: uuid.NewString(), Position: "overrated", Status: "suspected"}
		if err := db.Create(&vote).Error; err != nil {
			t.Fatal(err)
		}
	}

	response := performJSON(router, http.MethodGet, "/api/v1/admin/food-operations", "", token)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"decision":"promote_candidate"`) || !strings.Contains(response.Body.String(), `"participants":10`) {
		t.Fatalf("expected 70%% promote candidate from valid denominator, got %d: %s", response.Code, response.Body.String())
	}
	adjusted := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/admin/food-entries/"+entry.ID+"/adjustments", `{"direction":"promote","expected_version":1}`, token, "food-adjust-promote-001")
	if adjusted.Code != http.StatusOK || !strings.Contains(adjusted.Body.String(), promotedTier.ID) {
		t.Fatalf("expected one-tier promotion, got %d: %s", adjusted.Code, adjusted.Body.String())
	}
	var rounds []model.FoodCalibrationRound
	if err := db.Where("entry_id = ?", entry.ID).Order("round_number asc").Find(&rounds).Error; err != nil {
		t.Fatal(err)
	}
	if len(rounds) != 2 || rounds[0].Status != "closed" || rounds[1].Status != "open" || rounds[1].RoundNumber != 2 {
		t.Fatalf("expected old round closed and new round opened, got %#v", rounds)
	}
}

func TestAdminDashboardAlwaysReturnsSixRealDomainCards(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.AdminDashboardV2Enabled = true
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)
	admin := createTestUser(t, db, "dashboard-admin@stu.henu.edu.cn", model.RoleAdmin)
	token := loginTestUser(t, router, admin.Email)

	response := performJSON(router, http.MethodGet, "/api/v1/admin/dashboard-snapshots/latest", "", token)
	if response.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Cards []struct {
				Domain string `json:"domain"`
			} `json:"cards"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	want := []string{"users", "notice", "mail", "feedback", "food", "system"}
	if len(payload.Data.Cards) != len(want) {
		t.Fatalf("expected six cards, got %#v", payload.Data.Cards)
	}
	for index, domain := range want {
		if payload.Data.Cards[index].Domain != domain {
			t.Fatalf("card %d: expected %s, got %s", index, domain, payload.Data.Cards[index].Domain)
		}
	}
	if payload.RequestID == "" || response.Header().Get("X-Request-Id") == "" {
		t.Fatal("expected request_id in envelope and response header")
	}
}

func TestPlatformFeedbackUsesTwoTierSLAAndReplaysIdempotently(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	user := createTestUser(t, db, "feedback-sla@stu.henu.edu.cn", model.RoleUser)
	token := loginTestUser(t, router, user.Email)
	body := `{"category":"product","summary":"紧急反馈","content":"需要尽快处理","urgency":"urgent"}`

	first := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/platform-feedback", body, token, "feedback-urgent-001")
	if first.Code != http.StatusOK {
		t.Fatalf("expected feedback 200, got %d: %s", first.Code, first.Body.String())
	}
	second := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/platform-feedback", body, token, "feedback-urgent-001")
	if second.Code != first.Code || second.Body.String() != first.Body.String() {
		t.Fatalf("expected exact replay, first=%s second=%s", first.Body.String(), second.Body.String())
	}
	if second.Header().Get("X-Idempotency-Replayed") != "true" {
		t.Fatal("expected replay response header")
	}

	var feedback model.PlatformFeedback
	if err := db.First(&feedback, "summary = ?", "紧急反馈").Error; err != nil {
		t.Fatal(err)
	}
	duration := feedback.DueAt.Sub(feedback.CreatedAt)
	if duration < 23*time.Hour+59*time.Minute || duration > 24*time.Hour+time.Minute {
		t.Fatalf("expected urgent due_at around 24h, got %s", duration)
	}
	normalBody := `{"category":"product","summary":"普通反馈","content":"按常规时限处理","urgency":"normal"}`
	normalResponse := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/platform-feedback", normalBody, token, "feedback-normal-001")
	if normalResponse.Code != http.StatusOK {
		t.Fatalf("expected normal feedback 200, got %d: %s", normalResponse.Code, normalResponse.Body.String())
	}
	var normal model.PlatformFeedback
	if err := db.First(&normal, "summary = ?", "普通反馈").Error; err != nil {
		t.Fatal(err)
	}
	normalDuration := normal.DueAt.Sub(normal.CreatedAt)
	if normalDuration < 71*time.Hour+59*time.Minute || normalDuration > 72*time.Hour+time.Minute {
		t.Fatalf("expected normal due_at around 72h, got %s", normalDuration)
	}
	var feedbackCount, caseCount int64
	db.Model(&model.PlatformFeedback{}).Count(&feedbackCount)
	db.Model(&model.OperationCase{}).Count(&caseCount)
	if feedbackCount != 2 || caseCount != 2 {
		t.Fatalf("idempotent replay created duplicates: feedback=%d cases=%d", feedbackCount, caseCount)
	}
}

func TestNoticeUpsertKeepsImmutableVersions(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	admin := createTestUser(t, db, "notice-admin@stu.henu.edu.cn", model.RoleAdmin)
	token := loginTestUser(t, router, admin.Email)
	sourceID := uuid.NewString()
	firstBody := `{"schema_version":"campus-notice-import/1.0","external_id":"notice-001","source_id":"` + sourceID + `","title":"第一版","body":"正文一","published_at":"2026-07-18T00:00:00Z","original_url":"https://example.edu/notice-001"}`
	secondBody := `{"schema_version":"campus-notice-import/1.0","external_id":"notice-001","source_id":"` + sourceID + `","title":"第二版","body":"正文二","published_at":"2026-07-18T00:00:00Z","original_url":"https://example.edu/notice-001"}`

	for index, body := range []string{firstBody, firstBody, secondBody} {
		key := []string{"notice-create-001", "notice-create-duplicate", "notice-update-002"}[index]
		response := performJSONWithIdempotency(router, http.MethodPost, "/api/v1/admin/school-notices", body, token, key)
		if response.Code != http.StatusOK {
			t.Fatalf("notice request %d failed: %d %s", index, response.Code, response.Body.String())
		}
	}
	var notices int64
	var versions []model.CampusNoticeVersion
	db.Model(&model.CampusNotice{}).Count(&notices)
	db.Order("version asc").Find(&versions)
	if notices != 1 || len(versions) != 2 {
		t.Fatalf("expected one head and two immutable versions, notices=%d versions=%d", notices, len(versions))
	}
	if versions[0].Body != "正文一" || versions[1].Body != "正文二" {
		t.Fatalf("version bodies were overwritten: %#v", versions)
	}
}

func TestUploadIntentUsesS3CompatiblePresignAndScopePermissions(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	cfg.ObjectStorage.Endpoint = "http://localhost:9000"
	cfg.ObjectStorage.Region = "us-east-1"
	cfg.ObjectStorage.Bucket = "henu-kit"
	cfg.ObjectStorage.AccessKey = "test-access"
	cfg.ObjectStorage.SecretKey = "test-secret"
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)
	user := createTestUser(t, db, "food-upload@stu.henu.edu.cn", model.RoleUser)
	token := loginTestUser(t, router, user.Email)

	food := performJSON(router, http.MethodPost, "/api/v1/object-upload-intents", `{"scope":"food_image","file_name":"food.jpg","content_type":"image/jpeg","size_bytes":1024}`, token)
	if food.Code != http.StatusOK || !bytes.Contains(food.Body.Bytes(), []byte("X-Amz-Signature")) || bytes.Contains(food.Body.Bytes(), []byte("test-secret")) {
		t.Fatalf("unexpected food upload intent: %d %s", food.Code, food.Body.String())
	}
	notice := performJSON(router, http.MethodPost, "/api/v1/object-upload-intents", `{"scope":"notice_attachment","file_name":"notice.pdf","content_type":"application/pdf","size_bytes":1024}`, token)
	if notice.Code != http.StatusForbidden {
		t.Fatalf("expected notice attachment scope to require admin, got %d: %s", notice.Code, notice.Body.String())
	}
}

func performJSONWithIdempotency(router http.Handler, method, path, body, token, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
