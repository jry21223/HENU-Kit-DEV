package wiki

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

var (
	errEntryNotReviewable = errors.New("entry_not_reviewable")
	reviewableStatuses    = []string{model.StatusPending, model.StatusDraft, model.StatusNeedsChanges}
)

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type entryRequest struct {
	CourseID string `json:"courseId"`
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Content  string `json:"content"`
	Summary  string `json:"summary"`
}

type reviewRequest struct {
	ReviewReason string `json:"reviewReason"`
}

type publicEntry struct {
	ID           string `json:"id"`
	AuthorID     string `json:"authorId"`
	CourseID     string `json:"courseId,omitempty"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	Content      string `json:"content"`
	Version      int    `json:"version"`
	Visibility   string `json:"visibility"`
	LikeCount    int64  `json:"likeCount"`
	CommentCount int64  `json:"commentCount"`
	CollectCount int64  `json:"collectCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func (h Handler) ListPublished(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 50, 100)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Model(&model.WikiEntry{}).
		Joins("LEFT JOIN courses ON courses.id = wiki_entries.course_id").
		Where("wiki_entries.status = ? AND wiki_entries.visibility = ? AND (wiki_entries.course_id IS NULL OR courses.status = ?)", model.StatusPublished, "public", model.StatusPublished)
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("wiki_entries.course_id = ?", courseID)
	}
	var entries []model.WikiEntry
	if err := query.Order("wiki_entries.updated_at desc").Limit(limit).Find(&entries).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entries": publicEntries(entries)})
}

func (h Handler) Detail(ctx *gin.Context) {
	var entry model.WikiEntry
	if err := h.db.Model(&model.WikiEntry{}).
		Joins("LEFT JOIN courses ON courses.id = wiki_entries.course_id").
		Where("wiki_entries.id = ? AND wiki_entries.status = ? AND wiki_entries.visibility = ? AND (wiki_entries.course_id IS NULL OR courses.status = ?)", ctx.Param("id"), model.StatusPublished, "public", model.StatusPublished).
		First(&entry).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"entry": toPublicEntry(entry)})
}

func (h Handler) Create(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req entryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	title := strings.TrimSpace(req.Title)
	slug := strings.TrimSpace(req.Slug)
	content := strings.TrimSpace(req.Content)
	summary := strings.TrimSpace(req.Summary)
	courseID := strings.TrimSpace(req.CourseID)
	if err := validateEntryInput(title, slug, content, summary); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	var courseIDPtr *string
	if courseID != "" {
		var course model.Course
		if err := h.db.First(&course, "id = ? AND status = ?", courseID, model.StatusPublished).Error; err != nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "course_not_found", nil)
			return
		}
		courseIDPtr = &course.ID
	}
	entry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     user.ID,
		CourseID:     courseIDPtr,
		Title:        title,
		Slug:         slug,
		Content:      content,
		Version:      1,
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if summary == "" {
			summary = "initial submission"
		}
		history := model.WikiEditHistory{
			EntryID:  entry.ID,
			EditorID: user.ID,
			Version:  entry.Version,
			Content:  entry.Content,
			Summary:  summary,
		}
		return tx.Create(&history).Error
	})
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entry": entry})
}

func (h Handler) AdminEntries(ctx *gin.Context) {
	status := strings.TrimSpace(ctx.Query("status"))
	if status == "" {
		status = model.StatusPending
	}
	if !isReviewListStatus(status) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 100, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Where("status = ?", status)
	if authorID := strings.TrimSpace(ctx.Query("authorId")); authorID != "" {
		query = query.Where("author_id = ?", authorID)
	}
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	var entries []model.WikiEntry
	if err := query.Order("updated_at desc").Limit(limit).Find(&entries).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entries": entries})
}

func (h Handler) ApproveEntry(ctx *gin.Context) {
	h.reviewEntry(ctx, model.StatusPublished)
}

func (h Handler) RejectEntry(ctx *gin.Context) {
	h.reviewEntry(ctx, model.StatusRejected)
}

func (h Handler) reviewEntry(ctx *gin.Context, status string) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req reviewRequest
	if ctx.Request.Body != nil && ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
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

	var entry model.WikiEntry
	if err := h.db.First(&entry, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
		return
	}
	if !isReviewableStatus(entry.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "entry_not_reviewable", gin.H{"status": entry.Status})
		return
	}
	previousStatus := entry.Status
	action := "wiki_entry.published"
	if status == model.StatusRejected {
		action = "wiki_entry.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WikiEntry{}).
			Where("id = ? AND status IN ?", entry.ID, reviewableStatuses).
			Updates(map[string]interface{}{
				"status":        status,
				"reviewer_id":   user.ID,
				"reviewed_at":   gorm.Expr("CURRENT_TIMESTAMP"),
				"review_reason": reason,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errEntryNotReviewable
		}
		return audit.Record(ctx, tx, action, "wiki_entry", entry.ID, map[string]interface{}{
			"authorId":       entry.AuthorID,
			"courseId":       entry.CourseID,
			"previousStatus": previousStatus,
			"status":         status,
			"version":        entry.Version,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, errEntryNotReviewable) {
			latestStatus := entry.Status
			var latest model.WikiEntry
			if queryErr := h.db.Select("status").First(&latest, "id = ?", entry.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "entry_not_reviewable", gin.H{"status": latestStatus})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func validateEntryInput(title string, slug string, content string, summary string) error {
	if title == "" || slug == "" || content == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(title)) > 200 {
		return errors.New("title_too_long")
	}
	if len(slug) > 220 || !safeSlug(slug) {
		return errors.New("invalid_slug")
	}
	if len([]rune(content)) > 80000 {
		return errors.New("content_too_long")
	}
	if len([]rune(summary)) > 500 {
		return errors.New("summary_too_long")
	}
	return nil
}

func publicEntries(entries []model.WikiEntry) []publicEntry {
	result := make([]publicEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, toPublicEntry(entry))
	}
	return result
}

func toPublicEntry(entry model.WikiEntry) publicEntry {
	var courseID string
	if entry.CourseID != nil {
		courseID = *entry.CourseID
	}
	return publicEntry{
		ID:           entry.ID,
		AuthorID:     entry.AuthorID,
		CourseID:     courseID,
		Title:        entry.Title,
		Slug:         entry.Slug,
		Content:      entry.Content,
		Version:      entry.Version,
		Visibility:   entry.Visibility,
		LikeCount:    entry.LikeCount,
		CommentCount: entry.CommentCount,
		CollectCount: entry.CollectCount,
		CreatedAt:    entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    entry.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func safeSlug(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return true
}

func isReviewableStatus(status string) bool {
	for _, item := range reviewableStatuses {
		if status == item {
			return true
		}
	}
	return false
}

func isReviewListStatus(status string) bool {
	switch status {
	case model.StatusDraft, model.StatusPending, model.StatusNeedsChanges, model.StatusPublished, model.StatusRejected:
		return true
	default:
		return false
	}
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
