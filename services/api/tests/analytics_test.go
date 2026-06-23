package tests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/internal/server"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestAdminAnalyticsOverviewRequiresAdminAndAggregatesDownloads(t *testing.T) {
	db := newTestDB(t)
	router := server.NewRouter(testConfig(), applogger.New("test"), db, nil)
	course := createTestCourse(t, db)
	user := createTestUser(t, db, "analytics-user@stu.henu.edu.cn", model.RoleUser)

	publishedMaterial := createTestMaterial(t, db, course.ID, "Discrete notes", model.MaterialAccessLoginRequired, "materials/discrete-notes.txt")
	paidMaterial := createTestMaterial(t, db, course.ID, "Discrete mock paper", model.MaterialAccessPaid, "materials/discrete-mock.txt")
	pendingMaterial := model.Material{
		CourseID:       course.ID,
		Title:          "Pending answer",
		Type:           "answer",
		StorageKey:     "materials/pending-answer.txt",
		FileName:       "pending-answer.txt",
		AccessLevel:    model.MaterialAccessPaid,
		PreviewContent: "pending",
		Status:         model.StatusPending,
	}
	if err := db.Create(&pendingMaterial).Error; err != nil {
		t.Fatal(err)
	}

	coursePackage := model.CoursePackage{
		SchoolID:    course.SchoolID,
		CollegeID:   course.CollegeID,
		MajorID:     course.MajorID,
		CourseID:    &course.ID,
		Grade:       course.Grade,
		Title:       "Analytics package",
		Slug:        "analytics-package",
		PriceFen:    1990,
		Status:      model.StatusPublished,
		Description: "package for analytics test",
	}
	if err := db.Create(&coursePackage).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	logs := []model.MaterialDownloadLog{
		{UserID: &user.ID, MaterialID: paidMaterial.ID, AccessLevel: paidMaterial.AccessLevel, DownloadedAt: now.Add(-2 * time.Hour), IP: "127.0.0.1"},
		{UserID: &user.ID, MaterialID: paidMaterial.ID, AccessLevel: paidMaterial.AccessLevel, DownloadedAt: now.Add(-1 * time.Hour), IP: "127.0.0.1"},
		{UserID: &user.ID, MaterialID: publishedMaterial.ID, AccessLevel: publishedMaterial.AccessLevel, DownloadedAt: now.Add(-25 * time.Hour), IP: "127.0.0.1"},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	reports := []model.Report{
		{
			ReviewFields: model.ReviewFields{Status: model.StatusPending},
			ReporterID:   user.ID,
			TargetType:   "material",
			TargetID:     paidMaterial.ID,
			Reason:       "疑似侵权",
			Description:  "analytics pending report",
		},
		{
			ReviewFields: model.ReviewFields{Status: model.StatusApproved},
			ReporterID:   user.ID,
			TargetType:   "user",
			TargetID:     user.ID,
			Reason:       "骚扰",
			Description:  "analytics handled report",
		},
	}
	if err := db.Create(&reports).Error; err != nil {
		t.Fatal(err)
	}

	noToken := performJSON(router, http.MethodGet, "/api/v1/admin/analytics/overview", "", "")
	if noToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthenticated analytics 401, got %d: %s", noToken.Code, noToken.Body.String())
	}
	studentToken := loginTestUser(t, router, "analytics-user@stu.henu.edu.cn")
	forbidden := performJSON(router, http.MethodGet, "/api/v1/admin/analytics/overview", "", studentToken)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected student analytics 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}

	createTestUser(t, db, "analytics-admin@stu.henu.edu.cn", model.RoleAdmin)
	adminToken := loginTestUser(t, router, "analytics-admin@stu.henu.edu.cn")
	response := performJSON(router, http.MethodGet, "/api/v1/admin/analytics/overview", "", adminToken)
	if response.Code != http.StatusOK {
		t.Fatalf("expected admin analytics 200, got %d: %s", response.Code, response.Body.String())
	}

	var payload struct {
		Data struct {
			Totals struct {
				Courses            int64 `json:"courses"`
				Materials          int64 `json:"materials"`
				PublishedMaterials int64 `json:"publishedMaterials"`
				PendingMaterials   int64 `json:"pendingMaterials"`
				Packages           int64 `json:"packages"`
				Downloads          int64 `json:"downloads"`
				Reports            int64 `json:"reports"`
				PendingReports     int64 `json:"pendingReports"`
			} `json:"totals"`
			DownloadTrend []struct {
				Date  string `json:"date"`
				Count int64  `json:"count"`
			} `json:"downloadTrend"`
			TopMaterials []struct {
				MaterialID string `json:"materialId"`
				Title      string `json:"title"`
				Downloads  int64  `json:"downloads"`
			} `json:"topMaterials"`
			CourseDemand []struct {
				CourseID               string `json:"courseId"`
				MaterialCount          int64  `json:"materialCount"`
				PublishedMaterialCount int64  `json:"publishedMaterialCount"`
				DownloadCount          int64  `json:"downloadCount"`
			} `json:"courseDemand"`
			AccessBreakdown []struct {
				AccessLevel string `json:"accessLevel"`
				Downloads   int64  `json:"downloads"`
			} `json:"accessBreakdown"`
			ReportBreakdown []struct {
				TargetType string `json:"targetType"`
				Status     string `json:"status"`
				Count      int64  `json:"count"`
			} `json:"reportBreakdown"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	if payload.Data.Totals.Courses != 1 || payload.Data.Totals.Materials != 3 || payload.Data.Totals.PublishedMaterials != 2 {
		t.Fatalf("unexpected totals: %#v", payload.Data.Totals)
	}
	if payload.Data.Totals.PendingMaterials != 1 || payload.Data.Totals.Packages != 1 || payload.Data.Totals.Downloads != 3 {
		t.Fatalf("unexpected pending/package/download totals: %#v", payload.Data.Totals)
	}
	if payload.Data.Totals.Reports != 2 || payload.Data.Totals.PendingReports != 1 {
		t.Fatalf("unexpected report totals: %#v", payload.Data.Totals)
	}
	if len(payload.Data.DownloadTrend) != 14 {
		t.Fatalf("expected 14 trend points, got %d", len(payload.Data.DownloadTrend))
	}
	if len(payload.Data.TopMaterials) == 0 || payload.Data.TopMaterials[0].MaterialID != paidMaterial.ID || payload.Data.TopMaterials[0].Downloads != 2 {
		t.Fatalf("expected paid material as top material, got %#v", payload.Data.TopMaterials)
	}
	if len(payload.Data.CourseDemand) == 0 || payload.Data.CourseDemand[0].CourseID != course.ID || payload.Data.CourseDemand[0].DownloadCount != 3 {
		t.Fatalf("expected course demand to include all downloads, got %#v", payload.Data.CourseDemand)
	}
	if payload.Data.CourseDemand[0].MaterialCount != 3 || payload.Data.CourseDemand[0].PublishedMaterialCount != 2 {
		t.Fatalf("unexpected course material counts: %#v", payload.Data.CourseDemand[0])
	}
	if len(payload.Data.AccessBreakdown) == 0 {
		t.Fatal("expected access breakdown")
	}
	if !hasReportBreakdown(payload.Data.ReportBreakdown, "material", model.StatusPending, 1) {
		t.Fatalf("expected pending material report breakdown, got %#v", payload.Data.ReportBreakdown)
	}
	if !hasReportBreakdown(payload.Data.ReportBreakdown, "user", model.StatusApproved, 1) {
		t.Fatalf("expected approved user report breakdown, got %#v", payload.Data.ReportBreakdown)
	}
}

func hasReportBreakdown(rows []struct {
	TargetType string `json:"targetType"`
	Status     string `json:"status"`
	Count      int64  `json:"count"`
}, targetType string, status string, count int64) bool {
	for _, row := range rows {
		if row.TargetType == targetType && row.Status == status && row.Count == count {
			return true
		}
	}
	return false
}
