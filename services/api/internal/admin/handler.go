package admin

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const maxUploadBytes = 20 * 1024 * 1024
const defaultOperationLogRetentionDays = 180
const defaultOperationLogExportLimit = 5000

var allowedUploadExtensions = map[string]bool{
	".pdf":  true,
	".txt":  true,
	".md":   true,
	".docx": true,
}

type Handler struct {
	db                        *gorm.DB
	uploadDir                 string
	operationLogRetentionDays int
	operationLogExportLimit   int
}

func NewHandler(db *gorm.DB, uploadDir string, operationLogRetentionDays int, operationLogExportLimit int) Handler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if operationLogRetentionDays <= 0 {
		operationLogRetentionDays = defaultOperationLogRetentionDays
	}
	if operationLogExportLimit <= 0 {
		operationLogExportLimit = defaultOperationLogExportLimit
	}
	return Handler{
		db:                        db,
		uploadDir:                 uploadDir,
		operationLogRetentionDays: operationLogRetentionDays,
		operationLogExportLimit:   operationLogExportLimit,
	}
}

type schoolRequest struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	EmailDomains string `json:"emailDomains"`
	Status       string `json:"status"`
}

type collegeRequest struct {
	SchoolID string `json:"schoolId"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

type majorRequest struct {
	SchoolID  string `json:"schoolId"`
	CollegeID string `json:"collegeId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Status    string `json:"status"`
}

type courseRequest struct {
	SchoolID    string `json:"schoolId"`
	CollegeID   string `json:"collegeId"`
	MajorID     string `json:"majorId"`
	Grade       string `json:"grade"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	ExamScope   string `json:"examScope"`
	Status      string `json:"status"`
}

type materialRequest struct {
	CourseID       string `json:"courseId"`
	Title          string `json:"title"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	StorageKey     string `json:"storageKey"`
	FileName       string `json:"fileName"`
	FileSize       int64  `json:"fileSize"`
	PreviewContent string `json:"previewContent"`
	AccessLevel    string `json:"accessLevel"`
	Status         string `json:"status"`
}

type materialStatusRequest struct {
	Status string `json:"status"`
}

type materialReviewRequest struct {
	ReviewReason string `json:"reviewReason"`
}

func (h Handler) OperationLogs(ctx *gin.Context) {
	query, ok := h.operationLogQuery(ctx)
	if !ok {
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var logs []model.OperationLog
	if err := query.Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"operationLogs": logs})
}

func (h Handler) ExportOperationLogs(ctx *gin.Context) {
	query, ok := h.operationLogQuery(ctx)
	if !ok {
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), h.operationLogExportLimit, h.operationLogExportLimit)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var logs []model.OperationLog
	if err := query.Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	fileName := "operation-logs-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	ctx.Header("Content-Type", "text/csv; charset=utf-8")
	ctx.Header("Content-Disposition", "attachment; filename="+fileName)
	writer := csv.NewWriter(ctx.Writer)
	_ = writer.Write([]string{"id", "created_at", "operator_id", "action", "target_type", "target_id", "ip", "user_agent", "metadata"})
	for _, log := range logs {
		_ = writer.Write([]string{
			safeCSVCell(log.ID),
			safeCSVCell(log.CreatedAt.UTC().Format(time.RFC3339)),
			safeCSVCell(log.OperatorID),
			safeCSVCell(log.Action),
			safeCSVCell(log.TargetType),
			safeCSVCell(log.TargetID),
			safeCSVCell(log.IP),
			safeCSVCell(log.UserAgent),
			safeCSVCell(jsonString(log.Metadata)),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "export_failed", nil)
		return
	}
}

func (h Handler) OperationLogRetention(ctx *gin.Context) {
	response.OK(ctx, gin.H{
		"retentionDays":     h.operationLogRetentionDays,
		"exportLimit":       h.operationLogExportLimit,
		"automaticDeletion": false,
		"policy":            "Operation logs are retained for audit review; automatic deletion is not enabled in the MVP.",
		"recommendedReview": "Review and export high-risk admin mutations before manual retention cleanup.",
	})
}

func (h Handler) operationLogQuery(ctx *gin.Context) (*gorm.DB, bool) {
	query := h.db.Model(&model.OperationLog{})
	if operatorID := strings.TrimSpace(ctx.Query("operatorId")); operatorID != "" {
		query = query.Where("operator_id = ?", operatorID)
	}
	if action := strings.TrimSpace(ctx.Query("action")); action != "" {
		query = query.Where("action = ?", action)
	}
	if targetType := strings.TrimSpace(ctx.Query("targetType")); targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if targetID := strings.TrimSpace(ctx.Query("targetId")); targetID != "" {
		query = query.Where("target_id = ?", targetID)
	}
	if createdFrom := strings.TrimSpace(ctx.Query("createdFrom")); createdFrom != "" {
		parsed, ok := parseLogTime(createdFrom, false)
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_created_from", nil)
			return nil, false
		}
		query = query.Where("created_at >= ?", parsed)
	}
	if createdTo := strings.TrimSpace(ctx.Query("createdTo")); createdTo != "" {
		parsed, ok := parseLogTime(createdTo, true)
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_created_to", nil)
			return nil, false
		}
		query = query.Where("created_at <= ?", parsed)
	}
	return query, true
}

func (h Handler) CreateSchool(ctx *gin.Context) {
	var req schoolRequest
	if !bindJSON(ctx, &req) {
		return
	}
	school := model.School{
		Name:         required(req.Name),
		Slug:         required(req.Slug),
		EmailDomains: strings.TrimSpace(req.EmailDomains),
		Status:       defaultStatus(req.Status),
	}
	if school.Name == "" || school.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&school).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "school.create", "school", school.ID, map[string]interface{}{
			"slug":   school.Slug,
			"status": school.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"school": school})
}

func (h Handler) UpdateSchool(ctx *gin.Context) {
	var req schoolRequest
	if !bindJSON(ctx, &req) {
		return
	}
	updates := compactMap(map[string]interface{}{
		"name":          strings.TrimSpace(req.Name),
		"slug":          strings.TrimSpace(req.Slug),
		"email_domains": strings.TrimSpace(req.EmailDomains),
		"status":        strings.TrimSpace(req.Status),
	})
	h.updateByID(ctx, &model.School{}, updates, "school")
}

func (h Handler) ArchiveSchool(ctx *gin.Context) {
	h.archiveByID(ctx, &model.School{}, "school")
}

func (h Handler) CreateCollege(ctx *gin.Context) {
	var req collegeRequest
	if !bindJSON(ctx, &req) {
		return
	}
	college := model.College{
		SchoolID: strings.TrimSpace(req.SchoolID),
		Name:     required(req.Name),
		Status:   defaultStatus(req.Status),
	}
	if college.SchoolID == "" || college.Name == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&college).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "college.create", "college", college.ID, map[string]interface{}{
			"schoolId": college.SchoolID,
			"status":   college.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"college": college})
}

func (h Handler) UpdateCollege(ctx *gin.Context) {
	var req collegeRequest
	if !bindJSON(ctx, &req) {
		return
	}
	updates := compactMap(map[string]interface{}{
		"school_id": strings.TrimSpace(req.SchoolID),
		"name":      strings.TrimSpace(req.Name),
		"status":    strings.TrimSpace(req.Status),
	})
	h.updateByID(ctx, &model.College{}, updates, "college")
}

func (h Handler) ArchiveCollege(ctx *gin.Context) {
	h.archiveByID(ctx, &model.College{}, "college")
}

func (h Handler) CreateMajor(ctx *gin.Context) {
	var req majorRequest
	if !bindJSON(ctx, &req) {
		return
	}
	major := model.Major{
		SchoolID:  strings.TrimSpace(req.SchoolID),
		CollegeID: strings.TrimSpace(req.CollegeID),
		Name:      required(req.Name),
		Slug:      required(req.Slug),
		Status:    defaultStatus(req.Status),
	}
	if major.SchoolID == "" || major.CollegeID == "" || major.Name == "" || major.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&major).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "major.create", "major", major.ID, map[string]interface{}{
			"schoolId":  major.SchoolID,
			"collegeId": major.CollegeID,
			"slug":      major.Slug,
			"status":    major.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"major": major})
}

func (h Handler) UpdateMajor(ctx *gin.Context) {
	var req majorRequest
	if !bindJSON(ctx, &req) {
		return
	}
	updates := compactMap(map[string]interface{}{
		"school_id":  strings.TrimSpace(req.SchoolID),
		"college_id": strings.TrimSpace(req.CollegeID),
		"name":       strings.TrimSpace(req.Name),
		"slug":       strings.TrimSpace(req.Slug),
		"status":     strings.TrimSpace(req.Status),
	})
	h.updateByID(ctx, &model.Major{}, updates, "major")
}

func (h Handler) ArchiveMajor(ctx *gin.Context) {
	h.archiveByID(ctx, &model.Major{}, "major")
}

func (h Handler) CreateCourse(ctx *gin.Context) {
	var req courseRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status, ok := normalizeCourseStatus(req.Status, model.StatusPublished)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	course := model.Course{
		SchoolID:    strings.TrimSpace(req.SchoolID),
		CollegeID:   strings.TrimSpace(req.CollegeID),
		MajorID:     strings.TrimSpace(req.MajorID),
		Grade:       required(req.Grade),
		Name:        required(req.Name),
		Slug:        required(req.Slug),
		Description: strings.TrimSpace(req.Description),
		ExamScope:   strings.TrimSpace(req.ExamScope),
		Status:      status,
	}
	if course.SchoolID == "" || course.CollegeID == "" || course.MajorID == "" || course.Grade == "" || course.Name == "" || course.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&course).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "course.create", "course", course.ID, map[string]interface{}{
			"schoolId":  course.SchoolID,
			"collegeId": course.CollegeID,
			"majorId":   course.MajorID,
			"grade":     course.Grade,
			"slug":      course.Slug,
			"status":    course.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"course": course})
}

func (h Handler) ListCourses(ctx *gin.Context) {
	query := h.db.Model(&model.Course{})
	if schoolID := strings.TrimSpace(ctx.Query("schoolId")); schoolID != "" {
		query = query.Where("school_id = ?", schoolID)
	}
	if majorID := strings.TrimSpace(ctx.Query("majorId")); majorID != "" {
		query = query.Where("major_id = ?", majorID)
	}
	if grade := strings.TrimSpace(ctx.Query("grade")); grade != "" {
		query = query.Where("grade = ?", grade)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		normalized, ok := normalizeCourseStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", normalized)
	}
	var courses []model.Course
	if err := query.Order("updated_at desc").Limit(500).Find(&courses).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"courses": courses})
}

func (h Handler) UpdateCourse(ctx *gin.Context) {
	var req courseRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != "" {
		normalized, ok := normalizeCourseStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		status = normalized
	}
	updates := compactMap(map[string]interface{}{
		"school_id":   strings.TrimSpace(req.SchoolID),
		"college_id":  strings.TrimSpace(req.CollegeID),
		"major_id":    strings.TrimSpace(req.MajorID),
		"grade":       strings.TrimSpace(req.Grade),
		"name":        strings.TrimSpace(req.Name),
		"slug":        strings.TrimSpace(req.Slug),
		"description": strings.TrimSpace(req.Description),
		"exam_scope":  strings.TrimSpace(req.ExamScope),
		"status":      status,
	})
	h.updateByID(ctx, &model.Course{}, updates, "course")
}

func (h Handler) ArchiveCourse(ctx *gin.Context) {
	h.archiveByID(ctx, &model.Course{}, "course")
}

func (h Handler) CreateMaterial(ctx *gin.Context) {
	var req materialRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status, ok := normalizeMaterialStatus(req.Status, model.StatusDraft)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	materialType, ok := normalizeMaterialType(req.Type, "other")
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_material_type", nil)
		return
	}
	accessLevel, ok := normalizeAccessLevel(req.AccessLevel, model.MaterialAccessLoginRequired)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_access_level", nil)
		return
	}
	material := model.Material{
		CourseID:       strings.TrimSpace(req.CourseID),
		Title:          required(req.Title),
		Type:           materialType,
		Description:    strings.TrimSpace(req.Description),
		StorageKey:     strings.TrimSpace(req.StorageKey),
		FileName:       strings.TrimSpace(req.FileName),
		FileSize:       req.FileSize,
		PreviewContent: strings.TrimSpace(req.PreviewContent),
		AccessLevel:    accessLevel,
		Status:         status,
	}
	if material.CourseID == "" || material.Title == "" || material.StorageKey == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if hasUnsafePath(material.StorageKey) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsafe_storage_key", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&material).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "material.create", "material", material.ID, map[string]interface{}{
			"courseId":    material.CourseID,
			"type":        material.Type,
			"accessLevel": material.AccessLevel,
			"status":      material.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"material": material})
}

func (h Handler) ListMaterials(ctx *gin.Context) {
	query := h.db.Model(&model.Material{})
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		if _, ok := normalizeMaterialStatus(status, ""); !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", status)
	}
	var materials []model.Material
	if err := query.Order("updated_at desc").Limit(500).Find(&materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"materials": materials})
}

func (h Handler) ListMaterialReviews(ctx *gin.Context) {
	status := strings.TrimSpace(ctx.Query("status"))
	if status == "" {
		status = model.StatusPending
	}
	if !isMaterialReviewListStatus(status) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	query := h.db.Model(&model.Material{}).Where("status = ?", status)
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	var materials []model.Material
	if err := query.Order("updated_at desc").Limit(500).Find(&materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"materials": materials})
}

func (h Handler) UpdateMaterial(ctx *gin.Context) {
	if !rejectMaterialFileFieldUpdates(ctx) {
		return
	}
	var req materialRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != "" {
		normalized, ok := normalizeMaterialStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		status = normalized
	}
	materialType := strings.TrimSpace(req.Type)
	if materialType != "" {
		normalized, ok := normalizeMaterialType(materialType, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_material_type", nil)
			return
		}
		materialType = normalized
	}
	accessLevel := strings.TrimSpace(req.AccessLevel)
	if accessLevel != "" {
		normalized, ok := normalizeAccessLevel(accessLevel, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_access_level", nil)
			return
		}
		accessLevel = normalized
	}
	updates := compactMap(map[string]interface{}{
		"course_id":       strings.TrimSpace(req.CourseID),
		"title":           strings.TrimSpace(req.Title),
		"type":            materialType,
		"description":     strings.TrimSpace(req.Description),
		"preview_content": strings.TrimSpace(req.PreviewContent),
		"access_level":    accessLevel,
		"status":          status,
	})
	h.updateByID(ctx, &model.Material{}, updates, "material")
}

func (h Handler) UpdateMaterialStatus(ctx *gin.Context) {
	var req materialStatusRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status, ok := normalizeMaterialStatus(req.Status, "")
	if !ok || status == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	materialID := ctx.Param("id")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Material{}).Where("id = ?", materialID).Update("status", status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return audit.Record(ctx, tx, "material.status_update", "material", materialID, map[string]interface{}{
			"status": status,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "update_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"updated": true, "status": status})
}

func (h Handler) ApproveMaterial(ctx *gin.Context) {
	h.reviewMaterial(ctx, model.StatusPublished)
}

func (h Handler) RejectMaterial(ctx *gin.Context) {
	h.reviewMaterial(ctx, model.StatusRejected)
}

func (h Handler) reviewMaterial(ctx *gin.Context, status string) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req materialReviewRequest
	if ctx.Request.Body != nil && ctx.Request.ContentLength != 0 {
		if !bindJSON(ctx, &req) {
			return
		}
	}
	reason := strings.TrimSpace(req.ReviewReason)
	if len(reason) > 1000 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "review_reason_too_long", nil)
		return
	}
	if status == model.StatusRejected && reason == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "review_reason_required", nil)
		return
	}
	var material model.Material
	if err := h.db.First(&material, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
		return
	}
	if material.Status != model.StatusPending {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "material_not_reviewable", gin.H{"status": material.Status})
		return
	}
	previousStatus := material.Status
	action := "material.approved"
	if status == model.StatusRejected {
		action = "material.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Material{}).Where("id = ?", material.ID).Updates(map[string]interface{}{
			"status":        status,
			"reviewer_id":   user.ID,
			"reviewed_at":   gorm.Expr("CURRENT_TIMESTAMP"),
			"review_reason": reason,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return audit.Record(ctx, tx, action, "material", material.ID, map[string]interface{}{
			"courseId":       material.CourseID,
			"previousStatus": previousStatus,
			"status":         status,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func (h Handler) ArchiveMaterial(ctx *gin.Context) {
	h.archiveByID(ctx, &model.Material{}, "material")
}

func (h Handler) UploadMaterial(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxUploadBytes+1024)
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "file_too_large", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_file", nil)
		return
	}
	defer file.Close()

	if header.Size <= 0 || header.Size > maxUploadBytes {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "file_too_large", nil)
		return
	}
	originalName, ext, err := safeUploadFileName(header)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	if err := validateUploadContent(file, ext); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}

	courseID := strings.TrimSpace(ctx.PostForm("courseId"))
	title := required(ctx.PostForm("title"))
	if courseID == "" || title == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	status, ok := normalizeMaterialStatus(ctx.PostForm("status"), model.StatusDraft)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	materialType, ok := normalizeMaterialType(ctx.PostForm("type"), "other")
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_material_type", nil)
		return
	}
	accessLevel, ok := normalizeAccessLevel(ctx.PostForm("accessLevel"), model.MaterialAccessLoginRequired)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_access_level", nil)
		return
	}
	storageKey := filepath.ToSlash(filepath.Join("materials", courseID, uuid.NewString()+ext))
	targetPath := filepath.Join(h.uploadDir, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	defer target.Close()
	written, err := io.Copy(target, io.LimitReader(file, maxUploadBytes+1))
	if err != nil || written > maxUploadBytes {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}

	material := model.Material{
		CourseID:       courseID,
		Title:          title,
		Type:           materialType,
		Description:    strings.TrimSpace(ctx.PostForm("description")),
		StorageKey:     storageKey,
		FileName:       originalName,
		FileSize:       written,
		PreviewContent: strings.TrimSpace(ctx.PostForm("previewContent")),
		AccessLevel:    accessLevel,
		Status:         status,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&material).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "material.upload", "material", material.ID, map[string]interface{}{
			"courseId":    material.CourseID,
			"type":        material.Type,
			"accessLevel": material.AccessLevel,
			"status":      material.Status,
			"fileName":    material.FileName,
			"fileSize":    material.FileSize,
		})
	}); err != nil {
		_ = os.Remove(targetPath)
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"material": material})
}

func (h Handler) updateByID(ctx *gin.Context, target interface{}, updates map[string]interface{}, name string) {
	if len(updates) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "empty_update", nil)
		return
	}
	targetID := ctx.Param("id")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(target).Where("id = ?", targetID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return audit.Record(ctx, tx, name+".update", name, targetID, updateMetadata(updates))
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, name+"_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "update_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"updated": true})
}

func (h Handler) archiveByID(ctx *gin.Context, target interface{}, name string) {
	targetID := ctx.Param("id")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(target).Where("id = ?", targetID).Update("status", model.StatusArchived)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return audit.Record(ctx, tx, name+".archive", name, targetID, map[string]interface{}{
			"status": model.StatusArchived,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, name+"_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "archive_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"archived": true})
}

func bindJSON(ctx *gin.Context, target interface{}) bool {
	if err := ctx.ShouldBindJSON(target); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return false
	}
	return true
}

func rejectMaterialFileFieldUpdates(ctx *gin.Context) bool {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return false
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return false
	}
	for _, field := range []string{"storageKey", "storage_key", "fileName", "file_name", "fileSize", "file_size"} {
		if _, ok := raw[field]; ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "material_file_fields_immutable", nil)
			return false
		}
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	return true
}

func compactMap(values map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, value := range values {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				result[key] = typed
			}
		case int64:
			if typed > 0 {
				result[key] = typed
			}
		default:
			if value != nil {
				result[key] = value
			}
		}
	}
	return result
}

func updateMetadata(updates map[string]interface{}) map[string]interface{} {
	if len(updates) == 0 {
		return nil
	}
	fields := make([]string, 0, len(updates))
	for key := range updates {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	metadata := map[string]interface{}{
		"fields": fields,
	}
	for _, key := range []string{"status", "access_level", "type"} {
		if value, ok := updates[key]; ok {
			metadata[key] = value
		}
	}
	return metadata
}

func parseLimit(raw string, fallback int, max int) (int, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, false
	}
	if limit > max {
		return max, true
	}
	return limit, true
}

func parseLogTime(raw string, endOfDay bool) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		if endOfDay {
			parsed = parsed.Add(24*time.Hour - time.Nanosecond)
		}
		return parsed, true
	}
	return time.Time{}, false
}

func jsonString(value interface{}) string {
	if value == nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func safeCSVCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	trimmed := strings.TrimLeft(value, " \t")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func required(value string) string {
	return strings.TrimSpace(value)
}

func defaultStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return model.StatusPublished
	}
	return value
}

func normalizeMaterialType(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case "knowledge_note", "mock_paper", "answer", "quick_review", "past_exam", "other":
		return value, true
	default:
		return "", false
	}
}

func normalizeAccessLevel(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.MaterialAccessFree, model.MaterialAccessLoginRequired, model.MaterialAccessPaid, model.MaterialAccessMemberOnly:
		return value, true
	default:
		return "", false
	}
}

func normalizeMaterialStatus(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.StatusDraft, model.StatusPending, model.StatusPublished, model.StatusRejected, model.StatusArchived:
		return value, true
	default:
		return "", false
	}
}

func isMaterialReviewListStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case model.StatusPending, model.StatusPublished, model.StatusRejected:
		return true
	default:
		return false
	}
}

func normalizeCourseStatus(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.StatusDraft, model.StatusPublished, model.StatusArchived:
		return value, true
	default:
		return "", false
	}
}

func safeUploadFileName(header *multipart.FileHeader) (string, string, error) {
	name := strings.TrimSpace(header.Filename)
	if name == "" || strings.ContainsAny(name, `/\`) || name != filepath.Base(name) {
		return "", "", errors.New("unsafe_file_name")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedUploadExtensions[ext] {
		return "", "", errors.New("unsupported_file_type")
	}
	return name, ext, nil
}

func validateUploadContent(file multipart.File, ext string) error {
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return errors.New("invalid_file")
	}
	content := buffer[:n]
	if ext == ".pdf" && !strings.HasPrefix(string(content), "%PDF") {
		return errors.New("invalid_file_content")
	}
	if ext == ".txt" || ext == ".md" {
		if strings.Contains(string(content), "\x00") {
			return errors.New("invalid_file_content")
		}
	}
	return nil
}

func hasUnsafePath(value string) bool {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	return clean == "" || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
