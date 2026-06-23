package tests

import (
	"net/http"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestReportSubmissionReviewAndNotifications(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	material := createTestMaterial(t, db, course.ID, "Reportable material", model.MaterialAccessFree, "materials/reportable.txt")
	reporter := createTestUser(t, db, "reporter@stu.henu.edu.cn", model.RoleUser)
	reviewer := createTestUser(t, db, "report-reviewer@stu.henu.edu.cn", model.RoleReviewer)
	reporterToken := loginTestUser(t, router, reporter.Email)
	reviewerToken := loginTestUser(t, router, reviewer.Email)

	unauthorized := performJSON(router, http.MethodPost, "/api/v1/reports", `{"targetType":"material","targetId":"`+material.ID+`","reason":"侵权"}`, "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated report create 401, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}
	invalidTargetType := performJSON(router, http.MethodPost, "/api/v1/reports", `{"targetType":"payment","targetId":"`+material.ID+`","reason":"侵权"}`, reporterToken)
	if invalidTargetType.Code != http.StatusBadRequest || !strings.Contains(invalidTargetType.Body.String(), "invalid_target_type") {
		t.Fatalf("expected invalid target type rejection, got %d: %s", invalidTargetType.Code, invalidTargetType.Body.String())
	}
	hiddenMaterial := model.Material{
		CourseID:    course.ID,
		Title:       "Hidden material",
		Type:        "knowledge_note",
		StorageKey:  "materials/hidden-report.txt",
		AccessLevel: model.MaterialAccessFree,
		Status:      model.StatusPending,
	}
	if err := db.Create(&hiddenMaterial).Error; err != nil {
		t.Fatal(err)
	}
	hiddenTarget := performJSON(router, http.MethodPost, "/api/v1/reports", `{"targetType":"material","targetId":"`+hiddenMaterial.ID+`","reason":"侵权"}`, reporterToken)
	if hiddenTarget.Code != http.StatusNotFound || !strings.Contains(hiddenTarget.Body.String(), "target_not_found") {
		t.Fatalf("expected hidden target rejection, got %d: %s", hiddenTarget.Code, hiddenTarget.Body.String())
	}

	createReport := performJSON(router, http.MethodPost, "/api/v1/reports", `{"targetType":"material","targetId":"`+material.ID+`","reason":"侵权","description":"疑似未经授权传播资料"}`, reporterToken)
	if createReport.Code != http.StatusOK || !strings.Contains(createReport.Body.String(), `"created":true`) {
		t.Fatalf("expected report create 200, got %d: %s", createReport.Code, createReport.Body.String())
	}
	var report model.Report
	if err := db.First(&report, "reporter_id = ? AND target_type = ? AND target_id = ?", reporter.ID, "material", material.ID).Error; err != nil {
		t.Fatal(err)
	}
	if report.Status != model.StatusPending {
		t.Fatalf("expected pending report, got %#v", report)
	}
	duplicateReport := performJSON(router, http.MethodPost, "/api/v1/reports", `{"targetType":"material","targetId":"`+material.ID+`","reason":"侵权","description":"重复提交"}`, reporterToken)
	if duplicateReport.Code != http.StatusOK || !strings.Contains(duplicateReport.Body.String(), `"created":false`) || !strings.Contains(duplicateReport.Body.String(), report.ID) {
		t.Fatalf("expected duplicate pending report to return existing report, got %d: %s", duplicateReport.Code, duplicateReport.Body.String())
	}
	var pendingDuplicates int64
	if err := db.Model(&model.Report{}).Where("reporter_id = ? AND target_type = ? AND target_id = ? AND status = ?", reporter.ID, "material", material.ID, model.StatusPending).Count(&pendingDuplicates).Error; err != nil {
		t.Fatal(err)
	}
	if pendingDuplicates != 1 {
		t.Fatalf("expected duplicate pending report guard, got %d", pendingDuplicates)
	}

	forbiddenList := performJSON(router, http.MethodGet, "/api/v1/admin/reports", "", reporterToken)
	if forbiddenList.Code != http.StatusForbidden {
		t.Fatalf("expected user report admin list 403, got %d: %s", forbiddenList.Code, forbiddenList.Body.String())
	}
	invalidLimit := performJSON(router, http.MethodGet, "/api/v1/admin/reports?limit=bad", "", reviewerToken)
	if invalidLimit.Code != http.StatusBadRequest || !strings.Contains(invalidLimit.Body.String(), "invalid_limit") {
		t.Fatalf("expected invalid report limit rejection, got %d: %s", invalidLimit.Code, invalidLimit.Body.String())
	}
	reviewList := performJSON(router, http.MethodGet, "/api/v1/admin/reports", "", reviewerToken)
	if reviewList.Code != http.StatusOK || !strings.Contains(reviewList.Body.String(), report.ID) || !strings.Contains(reviewList.Body.String(), "疑似未经授权传播资料") {
		t.Fatalf("expected reviewer report list to include pending report, got %d: %s", reviewList.Code, reviewList.Body.String())
	}
	rejectWithoutReason := performJSON(router, http.MethodPost, "/api/v1/admin/reports/"+report.ID+"/reject", "", reviewerToken)
	if rejectWithoutReason.Code != http.StatusBadRequest || !strings.Contains(rejectWithoutReason.Body.String(), "review_reason_required") {
		t.Fatalf("expected report rejection reason required, got %d: %s", rejectWithoutReason.Code, rejectWithoutReason.Body.String())
	}

	resolveReport := performJSON(router, http.MethodPost, "/api/v1/admin/reports/"+report.ID+"/resolve", `{"reviewReason":"已处理并记录"}`, reviewerToken)
	if resolveReport.Code != http.StatusOK || !strings.Contains(resolveReport.Body.String(), model.StatusApproved) {
		t.Fatalf("expected report resolve 200, got %d: %s", resolveReport.Code, resolveReport.Body.String())
	}
	if err := db.First(&report, "id = ?", report.ID).Error; err != nil {
		t.Fatal(err)
	}
	if report.Status != model.StatusApproved || report.ReviewerID == nil || *report.ReviewerID != reviewer.ID || report.ReviewedAt == nil || report.ReviewReason != "已处理并记录" {
		t.Fatalf("expected report review metadata, got %#v", report)
	}
	if countOperationLogs(t, db, "report.resolved", "report", report.ID, reviewer.ID) != 1 {
		t.Fatal("expected report resolve operation log")
	}
	reviewAgain := performJSON(router, http.MethodPost, "/api/v1/admin/reports/"+report.ID+"/reject", `{"reviewReason":"overwrite"}`, reviewerToken)
	if reviewAgain.Code != http.StatusConflict || !strings.Contains(reviewAgain.Body.String(), "report_not_reviewable") {
		t.Fatalf("expected repeated report review conflict, got %d: %s", reviewAgain.Code, reviewAgain.Body.String())
	}
	reporterNotifications := performJSON(router, http.MethodGet, "/api/v1/me/notifications", "", reporterToken)
	if reporterNotifications.Code != http.StatusOK {
		t.Fatalf("expected reporter notifications 200, got %d: %s", reporterNotifications.Code, reporterNotifications.Body.String())
	}
	body := reporterNotifications.Body.String()
	for _, expected := range []string{`"type":"report_result"`, `"targetType":"material"`, `"status":"approved"`, "已处理并记录"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected report notification to contain %q, got %s", expected, body)
		}
	}
	if strings.Contains(body, reviewer.ID) {
		t.Fatalf("expected report notification to avoid reviewer id, got %s", body)
	}

	secondReportResponse := performJSON(router, http.MethodPost, "/api/v1/reports", `{"targetType":"user","targetId":"`+reviewer.ID+`","reason":"骚扰","description":"误报场景"}`, reporterToken)
	if secondReportResponse.Code != http.StatusOK {
		t.Fatalf("expected second report create 200, got %d: %s", secondReportResponse.Code, secondReportResponse.Body.String())
	}
	var rejectedReport model.Report
	if err := db.First(&rejectedReport, "target_type = ? AND target_id = ?", "user", reviewer.ID).Error; err != nil {
		t.Fatal(err)
	}
	rejectReport := performJSON(router, http.MethodPost, "/api/v1/admin/reports/"+rejectedReport.ID+"/reject", `{"reviewReason":"证据不足"}`, reviewerToken)
	if rejectReport.Code != http.StatusOK || !strings.Contains(rejectReport.Body.String(), model.StatusRejected) {
		t.Fatalf("expected report reject 200, got %d: %s", rejectReport.Code, rejectReport.Body.String())
	}
	if countOperationLogs(t, db, "report.rejected", "report", rejectedReport.ID, reviewer.ID) != 1 {
		t.Fatal("expected report reject operation log")
	}
	reporterNotifications = performJSON(router, http.MethodGet, "/api/v1/me/notifications?unread=true", "", reporterToken)
	if reporterNotifications.Code != http.StatusOK || !strings.Contains(reporterNotifications.Body.String(), `"status":"rejected"`) || !strings.Contains(reporterNotifications.Body.String(), "证据不足") {
		t.Fatalf("expected rejected report notification, got %d: %s", reporterNotifications.Code, reporterNotifications.Body.String())
	}
}
