package moment

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const (
	visibilityPublic        = "public"
	visibilityMutualFriends = "mutual_friends"
	relationFollow          = "follow"
	relationBlock           = "block"
	relationMomentLike      = "moment_like"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type createMomentRequest struct {
	Content    string   `json:"content"`
	Images     []string `json:"images"`
	Visibility string   `json:"visibility"`
}

type createCommentRequest struct {
	Content string `json:"content"`
}

type momentDTO struct {
	model.Moment
	Author      userSummary        `json:"author"`
	Images      []string           `json:"images"`
	LikedByMe   bool               `json:"likedByMe"`
	RecentReply []momentCommentDTO `json:"recentComments,omitempty"`
}

type momentCommentDTO struct {
	model.MomentComment
	Author userSummary `json:"author"`
}

type userSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func (h Handler) List(ctx *gin.Context) {
	currentUser, _ := auth.CurrentUser(ctx)
	limit, err := parseLimit(ctx.Query("limit"), 50, 100)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	var moments []model.Moment
	if err := h.db.Where("status = ?", model.StatusPublished).Order("created_at desc").Limit(limit).Find(&moments).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	visible := make([]model.Moment, 0, len(moments))
	for _, item := range moments {
		if h.canViewMoment(currentUser, item) {
			visible = append(visible, item)
		}
	}
	response.OK(ctx, gin.H{"moments": h.momentDTOs(currentUser, visible)})
}

func (h Handler) Create(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req createMomentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	content := strings.TrimSpace(req.Content)
	if len([]rune(content)) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_required", nil)
		return
	}
	if len([]rune(content)) > 500 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_too_long", nil)
		return
	}
	visibility := strings.TrimSpace(req.Visibility)
	if visibility == "" {
		visibility = visibilityPublic
	}
	if visibility != visibilityPublic && visibility != visibilityMutualFriends {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_visibility", nil)
		return
	}
	if len(req.Images) > 9 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "too_many_images", nil)
		return
	}
	images, err := validateImages(req.Images)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	moment := model.Moment{
		AuthorID: user.ID,
		Content:  content,
		Images:   images,
		Status:   model.StatusPublished,
	}
	moment.Visibility = visibility
	if err := h.db.Create(&moment).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"moment": h.momentDTO(user, moment)})
}

func (h Handler) Delete(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	if item.AuthorID != user.ID && user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return
	}
	if err := h.db.Delete(&item).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "delete_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"deleted": true})
}

func (h Handler) Like(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", ctx.Param("id")).Error; err != nil || !h.canViewMoment(user, item) {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	created := false
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var existing model.UserRelation
		err := tx.Where("user_id = ? AND target_id = ? AND type = ?", user.ID, item.ID, relationMomentLike).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&model.UserRelation{UserID: user.ID, TargetID: item.ID, Type: relationMomentLike}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Moment{}).Where("id = ?", item.ID).UpdateColumn("like_count", gorm.Expr("like_count + ?", 1)).Error; err != nil {
				return err
			}
			created = true
		}
		return tx.First(&item, "id = ?", item.ID).Error
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "like_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"liked": true, "created": created, "moment": h.momentDTO(user, item)})
}

func (h Handler) CreateComment(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", ctx.Param("id")).Error; err != nil || !h.canViewMoment(user, item) {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	var req createCommentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	content := strings.TrimSpace(req.Content)
	if len([]rune(content)) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_required", nil)
		return
	}
	if len([]rune(content)) > 500 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "content_too_long", nil)
		return
	}
	comment := model.MomentComment{AuthorID: user.ID, MomentID: item.ID, Content: content, Status: model.StatusPublished}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		return tx.Model(&model.Moment{}).Where("id = ?", item.ID).UpdateColumn("comment_count", gorm.Expr("comment_count + ?", 1)).Error
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "comment_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"comment": h.commentDTO(comment)})
}

func (h Handler) DeleteComment(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var comment model.MomentComment
	if err := h.db.First(&comment, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "comment_not_found", nil)
		return
	}
	var item model.Moment
	if err := h.db.First(&item, "id = ?", comment.MomentID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "moment_not_found", nil)
		return
	}
	if comment.AuthorID != user.ID && item.AuthorID != user.ID && user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&comment).Error; err != nil {
			return err
		}
		return tx.Model(&model.Moment{}).Where("id = ? AND comment_count > 0", item.ID).UpdateColumn("comment_count", gorm.Expr("comment_count - ?", 1)).Error
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "delete_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"deleted": true})
}

func (h Handler) momentDTOs(currentUser *model.User, moments []model.Moment) []momentDTO {
	dtos := make([]momentDTO, 0, len(moments))
	for _, item := range moments {
		dtos = append(dtos, h.momentDTO(currentUser, item))
	}
	return dtos
}

func (h Handler) momentDTO(currentUser *model.User, item model.Moment) momentDTO {
	return momentDTO{
		Moment:      item,
		Author:      h.userSummary(item.AuthorID),
		Images:      decodeImages(item.Images),
		LikedByMe:   currentUser != nil && h.hasRelation(currentUser.ID, item.ID, relationMomentLike),
		RecentReply: h.recentComments(item.ID),
	}
}

func (h Handler) recentComments(momentID string) []momentCommentDTO {
	var comments []model.MomentComment
	if err := h.db.Where("moment_id = ? AND status = ?", momentID, model.StatusPublished).Order("created_at asc").Limit(5).Find(&comments).Error; err != nil {
		return []momentCommentDTO{}
	}
	dtos := make([]momentCommentDTO, 0, len(comments))
	for _, comment := range comments {
		dtos = append(dtos, h.commentDTO(comment))
	}
	return dtos
}

func (h Handler) commentDTO(comment model.MomentComment) momentCommentDTO {
	return momentCommentDTO{MomentComment: comment, Author: h.userSummary(comment.AuthorID)}
}

func (h Handler) userSummary(userID string) userSummary {
	var user model.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		return userSummary{ID: userID}
	}
	return userSummary{ID: user.ID, Name: user.Name, Role: user.Role}
}

func (h Handler) canViewMoment(currentUser *model.User, item model.Moment) bool {
	if item.Status != model.StatusPublished {
		return false
	}
	if item.Visibility == visibilityPublic || item.Visibility == "" {
		if currentUser == nil {
			return true
		}
		return !h.hasBlockEitherDirection(currentUser.ID, item.AuthorID)
	}
	if item.Visibility != visibilityMutualFriends || currentUser == nil {
		return false
	}
	if currentUser.ID == item.AuthorID {
		return true
	}
	return !h.hasBlockEitherDirection(currentUser.ID, item.AuthorID) &&
		h.hasRelation(currentUser.ID, item.AuthorID, relationFollow) &&
		h.hasRelation(item.AuthorID, currentUser.ID, relationFollow)
}

func (h Handler) hasRelation(userID string, targetID string, relationType string) bool {
	var count int64
	if err := h.db.Model(&model.UserRelation{}).Where("user_id = ? AND target_id = ? AND type = ?", userID, targetID, relationType).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (h Handler) hasBlockEitherDirection(leftID string, rightID string) bool {
	var count int64
	if err := h.db.Model(&model.UserRelation{}).
		Where("type = ? AND ((user_id = ? AND target_id = ?) OR (user_id = ? AND target_id = ?))", relationBlock, leftID, rightID, rightID, leftID).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func validateImages(values []string) (datatypes.JSON, error) {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if len([]rune(trimmed)) > 500 {
			return nil, errors.New("image_url_too_long")
		}
		if !(strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "/uploads/")) {
			return nil, errors.New("invalid_image_url")
		}
		cleaned = append(cleaned, trimmed)
	}
	raw, err := json.Marshal(cleaned)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(raw), nil
}

func decodeImages(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var images []string
	if err := json.Unmarshal(raw, &images); err != nil {
		return []string{}
	}
	return images
}

func parseLimit(value string, fallback int, max int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 || limit > max {
		return 0, errors.New("invalid_limit")
	}
	return limit, nil
}
