package notification

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

func (h Handler) MyNotifications(ctx *gin.Context) {
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
	query := h.db.Where("user_id = ?", user.ID)
	if strings.TrimSpace(ctx.Query("unread")) == "true" {
		query = query.Where("read_at IS NULL")
	}
	var notifications []model.Notification
	if err := query.Order("created_at desc").Limit(limit).Find(&notifications).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	var unreadCount int64
	if err := h.db.Model(&model.Notification{}).Where("user_id = ? AND read_at IS NULL", user.ID).Count(&unreadCount).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"notifications": notifications, "unreadCount": unreadCount})
}

func (h Handler) MarkRead(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	result := h.db.Model(&model.Notification{}).
		Where("id = ? AND user_id = ?", ctx.Param("id"), user.ID).
		Where("read_at IS NULL").
		Update("read_at", gorm.Expr("CURRENT_TIMESTAMP"))
	if result.Error != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "mark_read_failed", nil)
		return
	}
	if result.RowsAffected == 0 {
		var existing model.Notification
		if err := h.db.First(&existing, "id = ? AND user_id = ?", ctx.Param("id"), user.ID).Error; err != nil {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "notification_not_found", nil)
			return
		}
		response.OK(ctx, gin.H{"read": true, "notificationId": existing.ID})
		return
	}
	response.OK(ctx, gin.H{"read": true, "notificationId": ctx.Param("id")})
}

func (h Handler) MarkAllRead(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	result := h.db.Model(&model.Notification{}).
		Where("user_id = ? AND read_at IS NULL", user.ID).
		Update("read_at", gorm.Expr("CURRENT_TIMESTAMP"))
	if result.Error != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "mark_read_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"read": true, "count": result.RowsAffected})
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
