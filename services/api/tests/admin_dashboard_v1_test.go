package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

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
