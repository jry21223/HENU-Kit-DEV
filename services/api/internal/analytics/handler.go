package analytics

import (
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const trendDays = 14

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type totals struct {
	Users              int64 `json:"users"`
	Courses            int64 `json:"courses"`
	Materials          int64 `json:"materials"`
	PublishedMaterials int64 `json:"publishedMaterials"`
	PendingMaterials   int64 `json:"pendingMaterials"`
	Packages           int64 `json:"packages"`
	Downloads          int64 `json:"downloads"`
	Reports            int64 `json:"reports"`
	PendingReports     int64 `json:"pendingReports"`
}

type trendPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type topMaterial struct {
	MaterialID  string `json:"materialId"`
	Title       string `json:"title"`
	CourseID    string `json:"courseId"`
	Type        string `json:"type"`
	AccessLevel string `json:"accessLevel"`
	Status      string `json:"status"`
	Downloads   int64  `json:"downloads"`
}

type courseDemand struct {
	CourseID               string `json:"courseId"`
	CourseName             string `json:"courseName"`
	Grade                  string `json:"grade"`
	Status                 string `json:"status"`
	MaterialCount          int64  `json:"materialCount"`
	PublishedMaterialCount int64  `json:"publishedMaterialCount"`
	DownloadCount          int64  `json:"downloadCount"`
}

type accessBreakdown struct {
	AccessLevel string `json:"accessLevel"`
	Downloads   int64  `json:"downloads"`
}

type reportBreakdown struct {
	TargetType string `json:"targetType"`
	Status     string `json:"status"`
	Count      int64  `json:"count"`
}

func (h Handler) Overview(ctx *gin.Context) {
	var total totals
	if err := h.db.Model(&model.User{}).Count(&total.Users).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.Course{}).Where("status <> ?", model.StatusArchived).Count(&total.Courses).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.Material{}).Where("status <> ?", model.StatusArchived).Count(&total.Materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.Material{}).Where("status = ?", model.StatusPublished).Count(&total.PublishedMaterials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.Material{}).Where("status = ?", model.StatusPending).Count(&total.PendingMaterials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.CoursePackage{}).Where("status <> ?", model.StatusArchived).Count(&total.Packages).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.MaterialDownloadLog{}).Count(&total.Downloads).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.Report{}).Count(&total.Reports).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if err := h.db.Model(&model.Report{}).Where("status = ?", model.StatusPending).Count(&total.PendingReports).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	var courses []model.Course
	if err := h.db.Where("status <> ?", model.StatusArchived).Find(&courses).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	var materials []model.Material
	if err := h.db.Where("status <> ?", model.StatusArchived).Find(&materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	since := time.Now().AddDate(0, 0, -trendDays+1).Truncate(24 * time.Hour)
	var logs []model.MaterialDownloadLog
	if err := h.db.Where("downloaded_at >= ?", since).
		Order("downloaded_at desc").
		Limit(5000).
		Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	var reports []model.Report
	if err := h.db.Order("created_at desc").Limit(5000).Find(&reports).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	materialsByID := make(map[string]model.Material, len(materials))
	for _, material := range materials {
		materialsByID[material.ID] = material
	}

	response.OK(ctx, gin.H{
		"totals":          total,
		"downloadTrend":   buildTrend(logs, since),
		"topMaterials":    buildTopMaterials(logs, materialsByID),
		"courseDemand":    buildCourseDemand(logs, materials, courses),
		"accessBreakdown": buildAccessBreakdown(logs),
		"reportBreakdown": buildReportBreakdown(reports),
	})
}

func buildTrend(logs []model.MaterialDownloadLog, since time.Time) []trendPoint {
	counts := map[string]int64{}
	for _, log := range logs {
		counts[log.DownloadedAt.Format("2006-01-02")]++
	}
	points := make([]trendPoint, 0, trendDays)
	for day := 0; day < trendDays; day++ {
		key := since.AddDate(0, 0, day).Format("2006-01-02")
		points = append(points, trendPoint{Date: key, Count: counts[key]})
	}
	return points
}

func buildTopMaterials(logs []model.MaterialDownloadLog, materialsByID map[string]model.Material) []topMaterial {
	counts := map[string]int64{}
	for _, log := range logs {
		counts[log.MaterialID]++
	}
	rows := make([]topMaterial, 0, len(counts))
	for materialID, count := range counts {
		material, ok := materialsByID[materialID]
		if !ok {
			rows = append(rows, topMaterial{MaterialID: materialID, Title: "deleted material", Downloads: count})
			continue
		}
		rows = append(rows, topMaterial{
			MaterialID:  material.ID,
			Title:       material.Title,
			CourseID:    material.CourseID,
			Type:        material.Type,
			AccessLevel: material.AccessLevel,
			Status:      material.Status,
			Downloads:   count,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Downloads == rows[j].Downloads {
			return rows[i].Title < rows[j].Title
		}
		return rows[i].Downloads > rows[j].Downloads
	})
	return limitTopMaterials(rows, 10)
}

func limitTopMaterials(rows []topMaterial, limit int) []topMaterial {
	if len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func buildCourseDemand(logs []model.MaterialDownloadLog, materials []model.Material, courses []model.Course) []courseDemand {
	rowsByCourse := make(map[string]*courseDemand, len(courses))
	for _, course := range courses {
		rowsByCourse[course.ID] = &courseDemand{
			CourseID:   course.ID,
			CourseName: course.Name,
			Grade:      course.Grade,
			Status:     course.Status,
		}
	}

	materialCourse := map[string]string{}
	for _, material := range materials {
		materialCourse[material.ID] = material.CourseID
		row, ok := rowsByCourse[material.CourseID]
		if !ok {
			continue
		}
		row.MaterialCount++
		if material.Status == model.StatusPublished {
			row.PublishedMaterialCount++
		}
	}

	for _, log := range logs {
		courseID := materialCourse[log.MaterialID]
		if row, ok := rowsByCourse[courseID]; ok {
			row.DownloadCount++
		}
	}

	rows := make([]courseDemand, 0, len(rowsByCourse))
	for _, row := range rowsByCourse {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].DownloadCount == rows[j].DownloadCount {
			if rows[i].PublishedMaterialCount == rows[j].PublishedMaterialCount {
				return rows[i].CourseName < rows[j].CourseName
			}
			return rows[i].PublishedMaterialCount > rows[j].PublishedMaterialCount
		}
		return rows[i].DownloadCount > rows[j].DownloadCount
	})
	if len(rows) > 10 {
		return rows[:10]
	}
	return rows
}

func buildAccessBreakdown(logs []model.MaterialDownloadLog) []accessBreakdown {
	counts := map[string]int64{}
	for _, log := range logs {
		counts[log.AccessLevel]++
	}
	rows := make([]accessBreakdown, 0, len(counts))
	for accessLevel, count := range counts {
		rows = append(rows, accessBreakdown{AccessLevel: accessLevel, Downloads: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Downloads == rows[j].Downloads {
			return rows[i].AccessLevel < rows[j].AccessLevel
		}
		return rows[i].Downloads > rows[j].Downloads
	})
	return rows
}

func buildReportBreakdown(reports []model.Report) []reportBreakdown {
	counts := map[string]int64{}
	for _, report := range reports {
		key := report.TargetType + "\x00" + report.Status
		counts[key]++
	}
	rows := make([]reportBreakdown, 0, len(counts))
	for key, count := range counts {
		parts := splitReportBreakdownKey(key)
		rows = append(rows, reportBreakdown{TargetType: parts[0], Status: parts[1], Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count == rows[j].Count {
			if rows[i].TargetType == rows[j].TargetType {
				return rows[i].Status < rows[j].Status
			}
			return rows[i].TargetType < rows[j].TargetType
		}
		return rows[i].Count > rows[j].Count
	})
	return rows
}

func splitReportBreakdownKey(key string) [2]string {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return [2]string{key[:i], key[i+1:]}
		}
	}
	return [2]string{key, ""}
}
