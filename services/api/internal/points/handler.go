package points

import (
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

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type ruleRequest struct {
	Code        *string `json:"code"`
	Description *string `json:"description"`
	Delta       *int64  `json:"delta"`
	Enabled     *bool   `json:"enabled"`
}

func (h Handler) Me(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	response.OK(ctx, gin.H{"balance": user.PointsBalance})
}

func (h Handler) MyLogs(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 50, 200)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var logs []model.PointsLog
	if err := h.db.Where("user_id = ?", user.ID).Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"logs": logs})
}

func (h Handler) AdminLogs(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 100, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Model(&model.PointsLog{})
	if userID := strings.TrimSpace(ctx.Query("userId")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if reason := strings.TrimSpace(ctx.Query("reason")); reason != "" {
		query = query.Where("reason = ?", reason)
	}
	var logs []model.PointsLog
	if err := query.Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"logs": logs})
}

func (h Handler) AdminRules(ctx *gin.Context) {
	var rules []model.PointsRule
	if err := h.db.Order("code asc").Find(&rules).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"rules": rules})
}

func (h Handler) CreateRule(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req ruleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	if req.Code == nil || req.Delta == nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "code_and_delta_required", nil)
		return
	}
	code := strings.TrimSpace(*req.Code)
	description := optionalTrim(req.Description)
	if err := validateRuleInput(code, description); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, safePointsError(err), nil)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule := model.PointsRule{
		Code:        code,
		Description: description,
		Delta:       *req.Delta,
		Enabled:     enabled,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rule).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "points_rule.created", "points_rule", rule.ID, map[string]interface{}{
			"operatorId": user.ID,
			"code":       rule.Code,
			"delta":      rule.Delta,
			"enabled":    rule.Enabled,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"rule": rule})
}

func (h Handler) UpdateRule(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var rule model.PointsRule
	if err := h.db.First(&rule, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "rule_not_found", nil)
		return
	}
	var req ruleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	updates := map[string]interface{}{}
	if req.Code != nil {
		code := strings.TrimSpace(*req.Code)
		if err := validateRuleCode(code); err != nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, safePointsError(err), nil)
			return
		}
		updates["code"] = code
	}
	if req.Description != nil {
		description := optionalTrim(req.Description)
		if len([]rune(description)) > 500 {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "description_too_long", nil)
			return
		}
		updates["description"] = description
	}
	if req.Delta != nil {
		updates["delta"] = *req.Delta
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "empty_update", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.PointsRule{}).Where("id = ?", rule.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&rule, "id = ?", rule.ID).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "points_rule.updated", "points_rule", rule.ID, map[string]interface{}{
			"operatorId": user.ID,
			"updates":    updates,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "update_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"rule": rule})
}

func parseLimit(value string, fallback int, max int) (int, bool) {
	if strings.TrimSpace(value) == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 || limit > max {
		return 0, false
	}
	return limit, true
}

func optionalTrim(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func validateRuleInput(code string, description string) error {
	if err := validateRuleCode(code); err != nil {
		return err
	}
	if len([]rune(description)) > 500 {
		return errString("description_too_long")
	}
	return nil
}

func validateRuleCode(code string) error {
	if code == "" {
		return errString("code_required")
	}
	if len([]rune(code)) > 100 {
		return errString("code_too_long")
	}
	return nil
}

type errString string

func (e errString) Error() string {
	return string(e)
}

func safePointsError(err error) string {
	if es, ok := err.(errString); ok {
		return string(es)
	}
	return "validation_failed"
}
