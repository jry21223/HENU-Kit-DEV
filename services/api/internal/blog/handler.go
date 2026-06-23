package blog

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
	errPostNotReviewable = errors.New("post_not_reviewable")
	reviewableStatuses   = []string{model.StatusPending, model.StatusDraft, model.StatusNeedsChanges}
)

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type postRequest struct {
	Title   string `json:"title"`
	Slug    string `json:"slug"`
	Content string `json:"content"`
}

type reviewRequest struct {
	ReviewReason string `json:"reviewReason"`
}

func (h Handler) ListPublished(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 50, 100)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var posts []model.BlogPost
	if err := h.db.Where("status = ?", model.StatusPublished).Order("created_at desc").Limit(limit).Find(&posts).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"posts": posts})
}

func (h Handler) Detail(ctx *gin.Context) {
	var post model.BlogPost
	if err := h.db.First(&post, "id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"post": post})
}

func (h Handler) Create(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req postRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	title := strings.TrimSpace(req.Title)
	slug := strings.TrimSpace(req.Slug)
	content := strings.TrimSpace(req.Content)
	if err := validatePostInput(title, slug, content); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	post := model.BlogPost{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     user.ID,
		Title:        title,
		Slug:         slug,
		Content:      content,
	}
	if err := h.db.Create(&post).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"post": post})
}

func (h Handler) AdminPosts(ctx *gin.Context) {
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
	var posts []model.BlogPost
	if err := query.Order("updated_at desc").Limit(limit).Find(&posts).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"posts": posts})
}

func (h Handler) ApprovePost(ctx *gin.Context) {
	h.reviewPost(ctx, model.StatusPublished)
}

func (h Handler) RejectPost(ctx *gin.Context) {
	h.reviewPost(ctx, model.StatusRejected)
}

func (h Handler) reviewPost(ctx *gin.Context, status string) {
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

	var post model.BlogPost
	if err := h.db.First(&post, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}
	if !isReviewableStatus(post.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "post_not_reviewable", gin.H{"status": post.Status})
		return
	}
	previousStatus := post.Status
	action := "blog_post.published"
	if status == model.StatusRejected {
		action = "blog_post.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.BlogPost{}).
			Where("id = ? AND status IN ?", post.ID, reviewableStatuses).
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
			return errPostNotReviewable
		}
		return audit.Record(ctx, tx, action, "blog_post", post.ID, map[string]interface{}{
			"authorId":       post.AuthorID,
			"previousStatus": previousStatus,
			"status":         status,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, errPostNotReviewable) {
			latestStatus := post.Status
			var latest model.BlogPost
			if queryErr := h.db.Select("status").First(&latest, "id = ?", post.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "post_not_reviewable", gin.H{"status": latestStatus})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func validatePostInput(title string, slug string, content string) error {
	if title == "" || slug == "" || content == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(title)) > 200 {
		return errors.New("title_too_long")
	}
	if len(slug) > 220 || !safeSlug(slug) {
		return errors.New("invalid_slug")
	}
	if len([]rune(content)) > 50000 {
		return errors.New("content_too_long")
	}
	return nil
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
