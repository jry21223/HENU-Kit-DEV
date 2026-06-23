package admin

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const maxUploadBytes = 20 * 1024 * 1024

var allowedUploadExtensions = map[string]bool{
	".pdf":  true,
	".txt":  true,
	".md":   true,
	".docx": true,
}

type Handler struct {
	db        *gorm.DB
	uploadDir string
}

func NewHandler(db *gorm.DB, uploadDir string) Handler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	return Handler{db: db, uploadDir: uploadDir}
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
	if err := h.db.Create(&school).Error; err != nil {
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
	if err := h.db.Create(&college).Error; err != nil {
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
	if err := h.db.Create(&major).Error; err != nil {
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
	course := model.Course{
		SchoolID:    strings.TrimSpace(req.SchoolID),
		CollegeID:   strings.TrimSpace(req.CollegeID),
		MajorID:     strings.TrimSpace(req.MajorID),
		Grade:       required(req.Grade),
		Name:        required(req.Name),
		Slug:        required(req.Slug),
		Description: strings.TrimSpace(req.Description),
		ExamScope:   strings.TrimSpace(req.ExamScope),
		Status:      defaultStatus(req.Status),
	}
	if course.SchoolID == "" || course.CollegeID == "" || course.MajorID == "" || course.Grade == "" || course.Name == "" || course.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Create(&course).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"course": course})
}

func (h Handler) UpdateCourse(ctx *gin.Context) {
	var req courseRequest
	if !bindJSON(ctx, &req) {
		return
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
		"status":      strings.TrimSpace(req.Status),
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
	material := model.Material{
		CourseID:       strings.TrimSpace(req.CourseID),
		Title:          required(req.Title),
		Type:           defaultMaterialType(req.Type),
		Description:    strings.TrimSpace(req.Description),
		StorageKey:     strings.TrimSpace(req.StorageKey),
		FileName:       strings.TrimSpace(req.FileName),
		FileSize:       req.FileSize,
		PreviewContent: strings.TrimSpace(req.PreviewContent),
		AccessLevel:    defaultAccessLevel(req.AccessLevel),
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
	if err := h.db.Create(&material).Error; err != nil {
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

func (h Handler) UpdateMaterial(ctx *gin.Context) {
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
	updates := compactMap(map[string]interface{}{
		"course_id":       strings.TrimSpace(req.CourseID),
		"title":           strings.TrimSpace(req.Title),
		"type":            strings.TrimSpace(req.Type),
		"description":     strings.TrimSpace(req.Description),
		"storage_key":     strings.TrimSpace(req.StorageKey),
		"file_name":       strings.TrimSpace(req.FileName),
		"preview_content": strings.TrimSpace(req.PreviewContent),
		"access_level":    strings.TrimSpace(req.AccessLevel),
		"status":          status,
		"file_size":       req.FileSize,
	})
	if storageKey, ok := updates["storage_key"].(string); ok && hasUnsafePath(storageKey) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsafe_storage_key", nil)
		return
	}
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
	result := h.db.Model(&model.Material{}).Where("id = ?", ctx.Param("id")).Update("status", status)
	if result.Error != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "update_failed", nil)
		return
	}
	if result.RowsAffected == 0 {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"updated": true, "status": status})
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
		Type:           defaultMaterialType(ctx.PostForm("type")),
		Description:    strings.TrimSpace(ctx.PostForm("description")),
		StorageKey:     storageKey,
		FileName:       originalName,
		FileSize:       written,
		PreviewContent: strings.TrimSpace(ctx.PostForm("previewContent")),
		AccessLevel:    defaultAccessLevel(ctx.PostForm("accessLevel")),
		Status:         status,
	}
	if err := h.db.Create(&material).Error; err != nil {
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
	result := h.db.Model(target).Where("id = ?", ctx.Param("id")).Updates(updates)
	if result.Error != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "update_failed", nil)
		return
	}
	if result.RowsAffected == 0 {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, name+"_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"updated": true})
}

func (h Handler) archiveByID(ctx *gin.Context, target interface{}, name string) {
	result := h.db.Model(target).Where("id = ?", ctx.Param("id")).Update("status", model.StatusArchived)
	if result.Error != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "archive_failed", nil)
		return
	}
	if result.RowsAffected == 0 {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, name+"_not_found", nil)
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

func defaultMaterialType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "other"
	}
	return value
}

func defaultAccessLevel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return model.MaterialAccessLoginRequired
	}
	return value
}

func normalizeMaterialStatus(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.StatusDraft, model.StatusPending, model.StatusPublished, model.StatusArchived:
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
