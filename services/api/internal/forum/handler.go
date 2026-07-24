package forum

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/notification"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

var (
	errPostNotReviewable   = errors.New("forum_post_not_reviewable")
	errReplyNotReviewable  = errors.New("forum_reply_not_reviewable")
	errPostNotEditable     = errors.New("forum_post_not_editable")
	errBestAnswerExists    = errors.New("best_answer_already_selected")
	errRewardNotSettleable = errors.New("reward_not_settleable")
	errInsufficientPoints  = errors.New("insufficient_points")
	reviewableStatuses     = []string{model.StatusPending, model.StatusDraft, model.StatusNeedsChanges}
	userEditableStatuses   = []string{model.StatusDraft, model.StatusPending, model.StatusNeedsChanges, model.StatusRejected}
)

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type postRequest struct {
	BoardID      string `json:"boardId"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Type         string `json:"type"`
	RewardPoints int64  `json:"rewardPoints"`
}

type replyRequest struct {
	Content string `json:"content"`
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
	RewardPoints int64  `json:"rewardPoints"`
	RewardStatus string `json:"rewardStatus,omitempty"`
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

type myPost struct {
	ID           string `json:"id"`
	BoardID      string `json:"boardId"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Type         string `json:"type"`
	RewardPoints int64  `json:"rewardPoints"`
	RewardStatus string `json:"rewardStatus,omitempty"`
	Status       string `json:"status"`
	ReviewReason string `json:"reviewReason,omitempty"`
	Visibility   string `json:"visibility"`
	CommentCount int64  `json:"commentCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type myReply struct {
	ID           string `json:"id"`
	PostID       string `json:"postId"`
	PostTitle    string `json:"postTitle"`
	PostStatus   string `json:"postStatus"`
	Content      string `json:"content"`
	IsBest       bool   `json:"isBest"`
	Status       string `json:"status"`
	ReviewReason string `json:"reviewReason,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
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

func (h Handler) MyPosts(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 50, 100)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var posts []model.ForumPost
	if err := h.db.Where("author_id = ?", user.ID).Order("updated_at desc").Limit(limit).Find(&posts).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"posts": myPosts(posts)})
}

func (h Handler) MyReplies(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 50, 100)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var replies []model.ForumReply
	if err := h.db.Where("author_id = ?", user.ID).Order("updated_at desc").Limit(limit).Find(&replies).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	postIDs := make([]string, 0, len(replies))
	seenPostIDs := make(map[string]struct{}, len(replies))
	for _, reply := range replies {
		if _, seen := seenPostIDs[reply.PostID]; seen {
			continue
		}
		seenPostIDs[reply.PostID] = struct{}{}
		postIDs = append(postIDs, reply.PostID)
	}
	postsByID := make(map[string]model.ForumPost, len(postIDs))
	if len(postIDs) > 0 {
		var posts []model.ForumPost
		if err := h.db.Where("id IN ?", postIDs).Find(&posts).Error; err != nil {
			response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
			return
		}
		for _, post := range posts {
			postsByID[post.ID] = post
		}
	}
	response.OK(ctx, gin.H{"replies": myReplies(replies, postsByID)})
}

func (h Handler) ResubmitPost(ctx *gin.Context) {
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
	content := strings.TrimSpace(req.Content)
	if err := validatePostTextInput(title, content); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "validation_failed", nil)
		return
	}

	var post model.ForumPost
	if err := h.db.First(&post, "id = ? AND author_id = ?", ctx.Param("id"), user.ID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}
	if !isUserEditableStatus(post.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_post_not_editable", gin.H{"status": post.Status})
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"title":         title,
			"content":       content,
			"status":        model.StatusPending,
			"reviewer_id":   nil,
			"reviewed_at":   nil,
			"review_reason": "",
		}
		if post.Type == "reward" && post.RewardStatus != "escrowed" && post.RewardPoints > 0 {
			if err := reescrowForumReward(tx, user.ID, post.ID, post.RewardPoints); err != nil {
				return err
			}
			updates["reward_status"] = "escrowed"
		}
		result := tx.Model(&model.ForumPost{}).
			Where("id = ? AND author_id = ? AND status IN ?", post.ID, user.ID, userEditableStatuses).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errPostNotEditable
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errInsufficientPoints) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "insufficient_points", nil)
			return
		}
		if errors.Is(err, errPostNotEditable) {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_post_not_editable", gin.H{"status": post.Status})
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "resubmit_failed", nil)
		return
	}
	if err := h.db.First(&post, "id = ?", post.ID).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"post": myPosts([]model.ForumPost{post})[0]})
}

func (h Handler) ResubmitReply(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req replyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	content := strings.TrimSpace(req.Content)
	if err := validateReplyInput(content); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "validation_failed", nil)
		return
	}

	var reply model.ForumReply
	if err := h.db.First(&reply, "id = ? AND author_id = ?", ctx.Param("id"), user.ID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "reply_not_found", nil)
		return
	}
	if !isUserEditableStatus(reply.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_reply_not_editable", gin.H{"status": reply.Status})
		return
	}
	var post model.ForumPost
	if err := h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.id = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", reply.PostID, model.StatusPublished, "public", model.StatusPublished).
		First(&post).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}

	result := h.db.Model(&model.ForumReply{}).
		Where("id = ? AND author_id = ? AND status IN ?", reply.ID, user.ID, userEditableStatuses).
		Updates(map[string]interface{}{
			"content":       content,
			"status":        model.StatusPending,
			"reviewer_id":   nil,
			"reviewed_at":   nil,
			"review_reason": "",
		})
	if result.Error != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "resubmit_failed", nil)
		return
	}
	if result.RowsAffected == 0 {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_reply_not_editable", gin.H{"status": reply.Status})
		return
	}
	if err := h.db.First(&reply, "id = ?", reply.ID).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reply": myReplies([]model.ForumReply{reply}, map[string]model.ForumPost{post.ID: post})[0]})
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
	rewardPoints := req.RewardPoints
	if postType == "" {
		postType = "normal"
	}
	if err := validatePostInput(boardID, title, content, postType, rewardPoints); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "validation_failed", nil)
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
		RewardPoints: rewardPoints,
		Status:       model.StatusPending,
	}
	if post.Type == "reward" {
		post.RewardStatus = "escrowed"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&post).Error; err != nil {
			return err
		}
		if post.Type == "reward" {
			return escrowForumReward(tx, user.ID, post.ID, rewardPoints)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errInsufficientPoints) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "insufficient_points", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"post": post})
}

func (h Handler) CreateReply(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req replyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	content := strings.TrimSpace(req.Content)
	if err := validateReplyInput(content); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "validation_failed", nil)
		return
	}
	var post model.ForumPost
	if err := h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.id = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", ctx.Param("id"), model.StatusPublished, "public", model.StatusPublished).
		First(&post).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}
	reply := model.ForumReply{
		AuthorID: user.ID,
		PostID:   post.ID,
		Content:  content,
		Status:   model.StatusPending,
	}
	if err := h.db.Create(&reply).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reply": reply})
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

func (h Handler) AdminReplies(ctx *gin.Context) {
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
	if postID := strings.TrimSpace(ctx.Query("postId")); postID != "" {
		query = query.Where("post_id = ?", postID)
	}
	var replies []model.ForumReply
	if err := query.Order("updated_at desc").Limit(limit).Find(&replies).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"replies": replies})
}

func (h Handler) ApprovePost(ctx *gin.Context) {
	h.reviewPost(ctx, model.StatusPublished)
}

func (h Handler) RejectPost(ctx *gin.Context) {
	h.reviewPost(ctx, model.StatusRejected)
}

func (h Handler) ApproveReply(ctx *gin.Context) {
	h.reviewReply(ctx, model.StatusPublished)
}

func (h Handler) RejectReply(ctx *gin.Context) {
	h.reviewReply(ctx, model.StatusRejected)
}

func (h Handler) MarkBestReply(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var reply model.ForumReply
	if err := h.db.First(&reply, "id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "reply_not_found", nil)
		return
	}
	var post model.ForumPost
	if err := h.db.Model(&model.ForumPost{}).
		Joins("JOIN forum_boards ON forum_boards.id = forum_posts.board_id").
		Where("forum_posts.id = ? AND forum_posts.status = ? AND forum_posts.visibility = ? AND forum_boards.status = ?", reply.PostID, model.StatusPublished, "public", model.StatusPublished).
		First(&post).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "post_not_found", nil)
		return
	}
	if !canMarkBestAnswer(user, post) {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return
	}
	if reply.AuthorID == post.AuthorID {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "cannot_mark_own_reply", nil)
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var existingBest int64
		if err := tx.Model(&model.ForumReply{}).Where("post_id = ? AND is_best = ?", post.ID, true).Count(&existingBest).Error; err != nil {
			return err
		}
		if existingBest > 0 {
			return errBestAnswerExists
		}
		if post.Type == "reward" && (post.RewardStatus != "escrowed" || post.RewardPoints <= 0) {
			return errRewardNotSettleable
		}
		result := tx.Model(&model.ForumReply{}).
			Where("id = ? AND status = ? AND is_best = ?", reply.ID, model.StatusPublished, false).
			Update("is_best", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errBestAnswerExists
		}
		rewardStatus := post.RewardStatus
		if post.Type == "reward" {
			if err := settleForumReward(tx, reply.AuthorID, post.ID, reply.ID, post.RewardPoints); err != nil {
				return err
			}
			postUpdate := tx.Model(&model.ForumPost{}).
				Where("id = ? AND reward_status = ?", post.ID, "escrowed").
				Update("reward_status", "settled")
			if postUpdate.Error != nil {
				return postUpdate.Error
			}
			if postUpdate.RowsAffected == 0 {
				return errRewardNotSettleable
			}
			rewardStatus = "settled"
		}
		return audit.Record(ctx, tx, "forum_reply.best_selected", "forum_reply", reply.ID, map[string]interface{}{
			"postId":        post.ID,
			"postAuthorId":  post.AuthorID,
			"replyAuthorId": reply.AuthorID,
			"type":          post.Type,
			"rewardPoints":  post.RewardPoints,
			"rewardStatus":  rewardStatus,
		})
	})
	if err != nil {
		if errors.Is(err, errBestAnswerExists) {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "best_answer_already_selected", nil)
			return
		}
		if errors.Is(err, errRewardNotSettleable) {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "reward_not_settleable", gin.H{"rewardStatus": post.RewardStatus})
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "mark_best_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"marked": true, "replyId": reply.ID, "postId": post.ID})
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
	auditRewardStatus := post.RewardStatus
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
		if status == model.StatusRejected && post.Type == "reward" && post.RewardStatus == "escrowed" && post.RewardPoints > 0 {
			if err := refundForumReward(tx, post.AuthorID, post.ID, post.RewardPoints); err != nil {
				return err
			}
			if err := tx.Model(&model.ForumPost{}).Where("id = ?", post.ID).Update("reward_status", "refunded").Error; err != nil {
				return err
			}
			auditRewardStatus = "refunded"
		}
		if err := notification.CreateReviewNotification(tx, notification.ReviewNotificationInput{
			NotificationType: "forum_review",
			UserID:           post.AuthorID,
			ResourceType:     "forum_post",
			ResourceID:       post.ID,
			ResourceTitle:    post.Title,
			Status:           status,
			Reason:           reason,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, action, "forum_post", post.ID, map[string]interface{}{
			"authorId":       post.AuthorID,
			"boardId":        post.BoardID,
			"previousStatus": previousStatus,
			"status":         status,
			"type":           post.Type,
			"rewardPoints":   post.RewardPoints,
			"rewardStatus":   auditRewardStatus,
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

func (h Handler) reviewReply(ctx *gin.Context, status string) {
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

	var reply model.ForumReply
	if err := h.db.First(&reply, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "reply_not_found", nil)
		return
	}
	if !isReviewableStatus(reply.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_reply_not_reviewable", gin.H{"status": reply.Status})
		return
	}
	previousStatus := reply.Status
	action := "forum_reply.published"
	if status == model.StatusRejected {
		action = "forum_reply.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ForumReply{}).
			Where("id = ? AND status IN ?", reply.ID, reviewableStatuses).
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
			return errReplyNotReviewable
		}
		if status == model.StatusPublished {
			if err := tx.Model(&model.ForumPost{}).Where("id = ?", reply.PostID).UpdateColumn("comment_count", gorm.Expr("comment_count + ?", 1)).Error; err != nil {
				return err
			}
		}
		var post model.ForumPost
		if err := tx.Select("id", "title").First(&post, "id = ?", reply.PostID).Error; err != nil {
			return err
		}
		if err := notification.CreateReviewNotification(tx, notification.ReviewNotificationInput{
			NotificationType: "forum_review",
			UserID:           reply.AuthorID,
			ResourceType:     "forum_reply",
			ResourceID:       reply.ID,
			ResourceTitle:    post.Title,
			Status:           status,
			Reason:           reason,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, action, "forum_reply", reply.ID, map[string]interface{}{
			"authorId":       reply.AuthorID,
			"postId":         reply.PostID,
			"previousStatus": previousStatus,
			"status":         status,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, errReplyNotReviewable) {
			latestStatus := reply.Status
			var latest model.ForumReply
			if queryErr := h.db.Select("status").First(&latest, "id = ?", reply.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "forum_reply_not_reviewable", gin.H{"status": latestStatus})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "reply_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func escrowForumReward(tx *gorm.DB, userID string, postID string, rewardPoints int64) error {
	result := tx.Model(&model.User{}).
		Where("id = ? AND points_balance >= ?", userID, rewardPoints).
		UpdateColumn("points_balance", gorm.Expr("points_balance - ?", rewardPoints))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errInsufficientPoints
	}
	var user model.User
	if err := tx.Select("points_balance").First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	idempotencyKey, err := nextPointsLogKey(tx, "forum_reward_escrow", postID)
	if err != nil {
		return err
	}
	return tx.Create(&model.PointsLog{
		UserID:         userID,
		Delta:          -rewardPoints,
		BalanceAfter:   user.PointsBalance,
		Reason:         "forum_reward_escrow",
		ReferenceType:  "forum_post",
		ReferenceID:    postID,
		IdempotencyKey: idempotencyKey,
	}).Error
}

func reescrowForumReward(tx *gorm.DB, userID string, postID string, rewardPoints int64) error {
	result := tx.Model(&model.User{}).
		Where("id = ? AND points_balance >= ?", userID, rewardPoints).
		UpdateColumn("points_balance", gorm.Expr("points_balance - ?", rewardPoints))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errInsufficientPoints
	}
	var user model.User
	if err := tx.Select("points_balance").First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	idempotencyKey, err := nextPointsLogKey(tx, "forum_reward_reescrow", postID)
	if err != nil {
		return err
	}
	return tx.Create(&model.PointsLog{
		UserID:         userID,
		Delta:          -rewardPoints,
		BalanceAfter:   user.PointsBalance,
		Reason:         "forum_reward_reescrow",
		ReferenceType:  "forum_post",
		ReferenceID:    postID,
		IdempotencyKey: idempotencyKey,
	}).Error
}

func refundForumReward(tx *gorm.DB, userID string, postID string, rewardPoints int64) error {
	if err := tx.Model(&model.User{}).
		Where("id = ?", userID).
		UpdateColumn("points_balance", gorm.Expr("points_balance + ?", rewardPoints)).Error; err != nil {
		return err
	}
	var user model.User
	if err := tx.Select("points_balance").First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	idempotencyKey, err := nextPointsLogKey(tx, "forum_reward_refund", postID)
	if err != nil {
		return err
	}
	return tx.Create(&model.PointsLog{
		UserID:         userID,
		Delta:          rewardPoints,
		BalanceAfter:   user.PointsBalance,
		Reason:         "forum_reward_refund",
		ReferenceType:  "forum_post",
		ReferenceID:    postID,
		IdempotencyKey: idempotencyKey,
	}).Error
}

func settleForumReward(tx *gorm.DB, userID string, postID string, replyID string, rewardPoints int64) error {
	if err := tx.Model(&model.User{}).
		Where("id = ?", userID).
		UpdateColumn("points_balance", gorm.Expr("points_balance + ?", rewardPoints)).Error; err != nil {
		return err
	}
	var user model.User
	if err := tx.Select("points_balance").First(&user, "id = ?", userID).Error; err != nil {
		return err
	}
	return tx.Create(&model.PointsLog{
		UserID:         userID,
		Delta:          rewardPoints,
		BalanceAfter:   user.PointsBalance,
		Reason:         "forum_reward_settlement",
		ReferenceType:  "forum_reply",
		ReferenceID:    replyID,
		IdempotencyKey: "forum_reward_settlement:" + postID,
	}).Error
}

func nextPointsLogKey(tx *gorm.DB, prefix string, referenceID string) (string, error) {
	key := prefix + ":" + referenceID
	var count int64
	if err := tx.Model(&model.PointsLog{}).Where("idempotency_key LIKE ?", key+"%").Count(&count).Error; err != nil {
		return "", err
	}
	if count == 0 {
		return key, nil
	}
	return fmt.Sprintf("%s:%d", key, count+1), nil
}

func validatePostInput(boardID string, title string, content string, postType string, rewardPoints int64) error {
	if boardID == "" {
		return errors.New("missing_required_fields")
	}
	if err := validatePostTextInput(title, content); err != nil {
		return err
	}
	switch postType {
	case "normal", "question":
		if rewardPoints != 0 {
			return errors.New("invalid_reward_points")
		}
		return nil
	case "reward":
		if rewardPoints <= 0 || rewardPoints > 100000 {
			return errors.New("invalid_reward_points")
		}
		return nil
	default:
		return errors.New("invalid_post_type")
	}
}

func validatePostTextInput(title string, content string) error {
	if title == "" || content == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(title)) > 200 {
		return errors.New("title_too_long")
	}
	if len([]rune(content)) > 20000 {
		return errors.New("content_too_long")
	}
	return nil
}

func validateReplyInput(content string) error {
	if content == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(content)) > 10000 {
		return errors.New("content_too_long")
	}
	return nil
}

func canMarkBestAnswer(user *model.User, post model.ForumPost) bool {
	if user == nil {
		return false
	}
	return user.ID == post.AuthorID || user.Role == model.RoleAdmin || user.Role == model.RoleSuperAdmin
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
		RewardPoints: post.RewardPoints,
		RewardStatus: post.RewardStatus,
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

func myPosts(posts []model.ForumPost) []myPost {
	result := make([]myPost, 0, len(posts))
	for _, post := range posts {
		result = append(result, myPost{
			ID:           post.ID,
			BoardID:      post.BoardID,
			Title:        post.Title,
			Content:      post.Content,
			Type:         post.Type,
			RewardPoints: post.RewardPoints,
			RewardStatus: post.RewardStatus,
			Status:       post.Status,
			ReviewReason: post.ReviewReason,
			Visibility:   post.Visibility,
			CommentCount: post.CommentCount,
			CreatedAt:    post.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    post.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return result
}

func myReplies(replies []model.ForumReply, postsByID map[string]model.ForumPost) []myReply {
	result := make([]myReply, 0, len(replies))
	for _, reply := range replies {
		post := postsByID[reply.PostID]
		result = append(result, myReply{
			ID:           reply.ID,
			PostID:       reply.PostID,
			PostTitle:    post.Title,
			PostStatus:   post.Status,
			Content:      reply.Content,
			IsBest:       reply.IsBest,
			Status:       reply.Status,
			ReviewReason: reply.ReviewReason,
			CreatedAt:    reply.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    reply.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
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

func isUserEditableStatus(status string) bool {
	for _, item := range userEditableStatuses {
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
