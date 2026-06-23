package forum

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
	errPostNotReviewable = errors.New("forum_post_not_reviewable")
	reviewableStatuses   = []string{model.StatusPending, model.StatusDraft, model.StatusNeedsChanges}
)

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type postRequest struct {
	BoardID string `json:"boardId"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

type reviewRequest struct {
	ReviewReason string `json:"reviewReason"`
}

type publicPost struct {
	ID           string `json:"id"`
	AuthorID     string `json:"authorId"`
	BoardID      string `json:"boardId"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Type         string `json:"type"`
	Visibility   string `json:"visibility"`
	LikeCount    int64  `json:"likeCount"`
	CommentCount int64  `json:"commentCount"`
	CollectCount int64  `json:"collectCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type publicReply struct {
	ID        string `json:"id"`
	AuthorID  string `json:"authorId"`
	PostID    string `json:"postId"`
	Content   string `json:"content"`
	IsBest    bool   `json:"isBest"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (h Handler) Boards(ctx *gin.Context) {
	var boards []model.ForumBoard
	if err := h.db.Where("status = ?", model.StatusPublished).Order("created_at asc").Find(&boards).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"boards": boards})
}

func (h Handler) ListPublished(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 50, 100)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", model.StatusPublished, "public", model.StatusPublished)
	if boardID := strings.TrimSpace(ctx.Query("boardId")); boardID != "" {
		query = query.Where("forum_posts.board_id = ?", boardID)
	}
	var posts []model.ForumPost
	if err := query.Order("forum_posts.updated_at desc").Limit(limit).Find(&posts).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"posts": publicPosts(posts)})
}

func (h Handler) Detail(ctx *gin.Context) {
	var post model.ForumPost
	if err := h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.id = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", ctx.Param("id"), model.StatusPublished, "public", model.StatusPublished).
		First(&post).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}
	var replies []model.ForumReply
	if err := h.db.Where("post_id = ? AND status = ?", post.ID, model.StatusPublished).Order("created_at asc").Find(&replies).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"post": toPublicPost(post), "replies": publicReplies(replies)})
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
	boardID := strings.TrimSpace(req.BoardID)
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	postType := strings.TrimSpace(req.Type)
	if postType == "" {
		postType = "normal"
	}
	if err := validatePostInput(boardID, title, content, postType); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	var board model.ForumBoard
	if err := h.db.First(&board, "id = ? AND status = ?", boardID, model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "board_not_found", nil)
		return
	}
	post := model.ForumPost{
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     user.ID,
		BoardID:      board.ID,
		Title:        title,
		Content:      content,
		Type:         postType,
		Status:       model.StatusPending,
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
	if boardID := strings.TrimSpace(ctx.Query("boardId")); boardID != "" {
		query = query.Where("board_id = ?", boardID)
	}
	var posts []model.ForumPost
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

	var post model.ForumPost
	if err := h.db.First(&post, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}
	if !isReviewableStatus(post.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_post_not_reviewable", gin.H{"status": post.Status})
		return
	}
	previousStatus := post.Status
	action := "forum_post.published"
	if status == model.StatusRejected {
		action = "forum_post.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ForumPost{}).
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
		return audit.Record(ctx, tx, action, "forum_post", post.ID, map[string]interface{}{
			"authorId":       post.AuthorID,
			"boardId":        post.BoardID,
			"previousStatus": previousStatus,
			"status":         status,
			"type":           post.Type,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, errPostNotReviewable) {
			latestStatus := post.Status
			var latest model.ForumPost
			if queryErr := h.db.Select("status").First(&latest, "id = ?", post.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_post_not_reviewable", gin.H{"status": latestStatus})
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

func validatePostInput(boardID string, title string, content string, postType string) error {
	if boardID == "" || title == "" || content == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(title)) > 200 {
		return errors.New("title_too_long")
	}
	if len([]rune(content)) > 20000 {
		return errors.New("content_too_long")
	}
	switch postType {
	case "normal", "question":
		return nil
	default:
		return errors.New("invalid_post_type")
	}
}

func publicPosts(posts []model.ForumPost) []publicPost {
	result := make([]publicPost, 0, len(posts))
	for _, post := range posts {
		result = append(result, toPublicPost(post))
	}
	return result
}

func toPublicPost(post model.ForumPost) publicPost {
	return publicPost{
		ID:           post.ID,
		AuthorID:     post.AuthorID,
		BoardID:      post.BoardID,
		Title:        post.Title,
		Content:      post.Content,
		Type:         post.Type,
		Visibility:   post.Visibility,
		LikeCount:    post.LikeCount,
		CommentCount: post.CommentCount,
		CollectCount: post.CollectCount,
		CreatedAt:    post.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    post.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func publicReplies(replies []model.ForumReply) []publicReply {
	result := make([]publicReply, 0, len(replies))
	for _, reply := range replies {
		result = append(result, publicReply{
			ID:        reply.ID,
			AuthorID:  reply.AuthorID,
			PostID:    reply.PostID,
			Content:   reply.Content,
			IsBest:    reply.IsBest,
			CreatedAt: reply.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt: reply.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return result
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
