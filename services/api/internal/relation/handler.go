package relation

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const (
	TypeFollow = "follow"
	TypeBlock  = "block"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type userSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

func (h Handler) Follow(ctx *gin.Context) {
	user, target, ok := h.currentUserAndTarget(ctx)
	if !ok {
		return
	}
	if h.hasBlockEitherDirection(user.ID, target.ID) {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "blocked_relation", nil)
		return
	}
	if err := h.db.Where("user_id = ? AND target_id = ? AND type = ?", user.ID, target.ID, TypeFollow).
		FirstOrCreate(&model.UserRelation{UserID: user.ID, TargetID: target.ID, Type: TypeFollow}).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "follow_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"following": true})
}

func (h Handler) Unfollow(ctx *gin.Context) {
	user, target, ok := h.currentUserAndTarget(ctx)
	if !ok {
		return
	}
	if err := h.db.Where("user_id = ? AND target_id = ? AND type = ?", user.ID, target.ID, TypeFollow).
		Delete(&model.UserRelation{}).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "unfollow_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"following": false})
}

func (h Handler) Block(ctx *gin.Context) {
	user, target, ok := h.currentUserAndTarget(ctx)
	if !ok {
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ? AND target_id = ? AND type = ?", user.ID, target.ID, TypeBlock).
			FirstOrCreate(&model.UserRelation{UserID: user.ID, TargetID: target.ID, Type: TypeBlock}).Error; err != nil {
			return err
		}
		return tx.Where("((user_id = ? AND target_id = ?) OR (user_id = ? AND target_id = ?)) AND type = ?", user.ID, target.ID, target.ID, user.ID, TypeFollow).
			Delete(&model.UserRelation{}).Error
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "block_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"blocked": true})
}

func (h Handler) Unblock(ctx *gin.Context) {
	user, target, ok := h.currentUserAndTarget(ctx)
	if !ok {
		return
	}
	if err := h.db.Where("user_id = ? AND target_id = ? AND type = ?", user.ID, target.ID, TypeBlock).
		Delete(&model.UserRelation{}).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "unblock_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"blocked": false})
}

func (h Handler) Following(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	users, err := h.relatedUsers(ctx.Query("limit"), "user_id = ? AND type = ?", "target_id", user.ID, TypeFollow)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	response.OK(ctx, gin.H{"users": users})
}

func (h Handler) Followers(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	users, err := h.relatedUsers(ctx.Query("limit"), "target_id = ? AND type = ?", "user_id", user.ID, TypeFollow)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	response.OK(ctx, gin.H{"users": users})
}

func (h Handler) Friends(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	limit, err := parseLimit(ctx.Query("limit"), 50, 200)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	var relations []model.UserRelation
	if err := h.db.Where("user_id = ? AND type = ?", user.ID, TypeFollow).Limit(limit).Find(&relations).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	friendIDs := make([]string, 0, len(relations))
	for _, relation := range relations {
		if h.hasRelation(relation.TargetID, user.ID, TypeFollow) && !h.hasBlockEitherDirection(user.ID, relation.TargetID) {
			friendIDs = append(friendIDs, relation.TargetID)
		}
	}
	response.OK(ctx, gin.H{"users": h.usersByID(friendIDs)})
}

func (h Handler) currentUserAndTarget(ctx *gin.Context) (*model.User, model.User, bool) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return nil, model.User{}, false
	}
	targetID := strings.TrimSpace(ctx.Param("id"))
	if targetID == "" || targetID == user.ID {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_target", nil)
		return nil, model.User{}, false
	}
	var target model.User
	if err := h.db.First(&target, "id = ?", targetID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "target_not_found", nil)
		return nil, model.User{}, false
	}
	return user, target, true
}

func (h Handler) relatedUsers(rawLimit string, relationWhere string, userColumn string, args ...interface{}) ([]userSummary, error) {
	limit, err := parseLimit(rawLimit, 50, 200)
	if err != nil {
		return nil, err
	}
	var relations []model.UserRelation
	if err := h.db.Where(relationWhere, args...).Order("created_at desc").Limit(limit).Find(&relations).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(relations))
	for _, relation := range relations {
		if userColumn == "target_id" {
			if !h.hasBlockEitherDirection(relation.UserID, relation.TargetID) {
				ids = append(ids, relation.TargetID)
			}
			continue
		}
		if !h.hasBlockEitherDirection(relation.UserID, relation.TargetID) {
			ids = append(ids, relation.UserID)
		}
	}
	return h.usersByID(ids), nil
}

func (h Handler) usersByID(ids []string) []userSummary {
	if len(ids) == 0 {
		return []userSummary{}
	}
	var users []model.User
	if err := h.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return []userSummary{}
	}
	byID := map[string]model.User{}
	for _, user := range users {
		byID[user.ID] = user
	}
	summaries := make([]userSummary, 0, len(ids))
	for _, id := range ids {
		if user, ok := byID[id]; ok {
			summaries = append(summaries, userSummary{ID: user.ID, Name: user.Name, Role: user.Role})
		}
	}
	return summaries
}

func (h Handler) hasRelation(userID string, targetID string, relationType string) bool {
	var count int64
	if err := h.db.Model(&model.UserRelation{}).
		Where("user_id = ? AND target_id = ? AND type = ?", userID, targetID, relationType).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func (h Handler) hasBlockEitherDirection(leftID string, rightID string) bool {
	var count int64
	if err := h.db.Model(&model.UserRelation{}).
		Where("type = ? AND ((user_id = ? AND target_id = ?) OR (user_id = ? AND target_id = ?))", TypeBlock, leftID, rightID, rightID, leftID).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
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
