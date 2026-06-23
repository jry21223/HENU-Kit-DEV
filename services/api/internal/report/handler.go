package report

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

type createReportRequest struct {
	TargetType  string `json:"targetType"`
	TargetID    string `json:"targetId"`
	Reason      string `json:"reason"`
	Description string `json:"description"`
}

type reviewReportRequest struct {
	ReviewReason string `json:"reviewReason"`
}

var errReportNotReviewable = errors.New("report_not_reviewable")

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

func (h Handler) Create(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req createReportRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	targetType := strings.TrimSpace(req.TargetType)
	targetID := strings.TrimSpace(req.TargetID)
	reason := strings.TrimSpace(req.Reason)
	description := strings.TrimSpace(req.Description)
	if err := validateCreateReportInput(targetType, targetID, reason, description); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	if err := h.ensureReportTargetVisible(targetType, targetID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "target_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "target_check_failed", nil)
		return
	}

	var existing model.Report
	err := h.db.Where("reporter_id = ? AND target_type = ? AND target_id = ? AND status = ?", user.ID, targetType, targetID, model.StatusPending).
		First(&existing).Error
	if err == nil {
		response.OK(ctx, gin.H{"report": existing, "created": false})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	report := model.Report{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		ReporterID:   user.ID,
		TargetType:   targetType,
		TargetID:     targetID,
		Reason:       reason,
		Description:  description,
	}
	if err := h.db.Create(&report).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"report": report, "created": true})
}

func (h Handler) AdminReports(ctx *gin.Context) {
	status := strings.TrimSpace(ctx.Query("status"))
	if status == "" {
		status = model.StatusPending
	}
	if status != "all" && !isReportStatus(status) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 100, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Model(&model.Report{})
	if status != "all" {
		query = query.Where("status = ?", status)
	}
	if targetType := strings.TrimSpace(ctx.Query("targetType")); targetType != "" {
		if !isAllowedReportTargetType(targetType) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_target_type", nil)
			return
		}
		query = query.Where("target_type = ?", targetType)
	}
	var reports []model.Report
	if err := query.Order("created_at desc").Limit(limit).Find(&reports).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reports": reports})
}

func (h Handler) Resolve(ctx *gin.Context) {
	h.review(ctx, model.StatusApproved)
}

func (h Handler) Reject(ctx *gin.Context) {
	h.review(ctx, model.StatusRejected)
}

func (h Handler) review(ctx *gin.Context, status string) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req reviewReportRequest
	if ctx.Request.Body != nil && ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
			return
		}
	}
	reason := strings.TrimSpace(req.ReviewReason)
	if len([]rune(reason)) > 1000 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "review_reason_too_long", nil)
		return
	}
	if status == model.StatusRejected && reason == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "review_reason_required", nil)
		return
	}

	var report model.Report
	if err := h.db.First(&report, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "report_not_found", nil)
		return
	}
	if report.Status != model.StatusPending {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "report_not_reviewable", gin.H{"status": report.Status})
		return
	}
	action := "report.resolved"
	if status == model.StatusRejected {
		action = "report.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Report{}).
			Where("id = ? AND status = ?", report.ID, model.StatusPending).
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
			return errReportNotReviewable
		}
		if err := notification.CreateReportResultNotification(tx, notification.ReportResultNotificationInput{
			UserID:     report.ReporterID,
			ReportID:   report.ID,
			TargetType: report.TargetType,
			TargetID:   report.TargetID,
			Status:     status,
			Reason:     reason,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, action, "report", report.ID, map[string]interface{}{
			"reporterId": report.ReporterID,
			"targetType": report.TargetType,
			"targetId":   report.TargetID,
			"status":     status,
		})
	})
	if err != nil {
		if errors.Is(err, errReportNotReviewable) {
			latestStatus := report.Status
			var latest model.Report
			if queryErr := h.db.Select("status").First(&latest, "id = ?", report.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "report_not_reviewable", gin.H{"status": latestStatus})
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func validateCreateReportInput(targetType string, targetID string, reason string, description string) error {
	if !isAllowedReportTargetType(targetType) {
		return errors.New("invalid_target_type")
	}
	if _, err := uuid.Parse(targetID); err != nil {
		return errors.New("invalid_target_id")
	}
	if len([]rune(reason)) < 2 || len([]rune(reason)) > 120 {
		return errors.New("invalid_reason")
	}
	if len([]rune(description)) > 2000 {
		return errors.New("description_too_long")
	}
	return nil
}

func (h Handler) ensureReportTargetVisible(targetType string, targetID string) error {
	switch targetType {
	case "material":
		return h.db.Select("id").First(&model.Material{}, "id = ? AND status = ?", targetID, model.StatusPublished).Error
	case "wiki_entry":
		return h.db.Select("id").First(&model.WikiEntry{}, "id = ? AND status = ? AND visibility = ?", targetID, model.StatusPublished, "public").Error
	case "blog_post":
		return h.db.Select("id").First(&model.BlogPost{}, "id = ? AND status = ? AND visibility = ?", targetID, model.StatusPublished, "public").Error
	case "forum_post":
		return h.db.Select("id").First(&model.ForumPost{}, "id = ? AND status = ? AND visibility = ?", targetID, model.StatusPublished, "public").Error
	case "forum_reply":
		return h.db.Select("id").First(&model.ForumReply{}, "id = ? AND status = ?", targetID, model.StatusPublished).Error
	case "user":
		return h.db.Select("id").First(&model.User{}, "id = ? AND status <> ?", targetID, "deleted").Error
	default:
		return gorm.ErrRecordNotFound
	}
}

func isAllowedReportTargetType(targetType string) bool {
	switch targetType {
	case "material", "wiki_entry", "blog_post", "forum_post", "forum_reply", "user":
		return true
	default:
		return false
	}
}

func isReportStatus(status string) bool {
	switch status {
	case model.StatusPending, model.StatusApproved, model.StatusRejected:
		return true
	default:
		return false
	}
}

func parseLimit(raw string, defaultLimit int, maxLimit int) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return defaultLimit, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 || limit > maxLimit {
		return 0, false
	}
	return limit, true
}
