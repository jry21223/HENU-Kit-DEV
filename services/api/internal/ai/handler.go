package ai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	redislib "github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/notification"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

var allowedTaskTypes = map[string]struct{}{
	"chat":                    {},
	"wrong_question_analysis": {},
	"targeted_question":       {},
	"paper_generation":        {},
	"draft_review":            {},
}

type Handler struct {
	db         *gorm.DB
	cache      *redislib.Client
	taskStream string
}

func NewHandler(db *gorm.DB, cache *redislib.Client, taskStream string) Handler {
	if strings.TrimSpace(taskStream) == "" {
		taskStream = "ai_tasks"
	}
	return Handler{db: db, cache: cache, taskStream: taskStream}
}

type createTaskRequest struct {
	CourseID string         `json:"courseId"`
	Type     string         `json:"type"`
	Input    datatypes.JSON `json:"input"`
}

type reviewDraftRequest struct {
	ReviewReason string `json:"reviewReason"`
}

func (h Handler) CreateTask(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}

	var req createTaskRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	taskType := strings.TrimSpace(req.Type)
	if taskType == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "type_required", nil)
		return
	}
	if _, ok := allowedTaskTypes[taskType]; !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsupported_task_type", nil)
		return
	}

	var courseID *string
	if strings.TrimSpace(req.CourseID) != "" {
		var course model.Course
		if err := h.db.First(&course, "id = ? AND status = ?", req.CourseID, model.StatusPublished).Error; err != nil {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "course_not_found", nil)
			return
		}
		courseID = &course.ID
	}
	input := req.Input
	if len(input) == 0 {
		input = datatypes.JSON([]byte(`{}`))
	}
	task := model.AITask{
		UserID:   &user.ID,
		CourseID: courseID,
		Type:     taskType,
		Status:   model.AITaskPending,
		Input:    input,
	}
	if err := h.db.Create(&task).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "create_failed", nil)
		return
	}

	enqueued := h.enqueue(ctx.Request.Context(), task)
	response.OK(ctx, gin.H{"task": task, "enqueued": enqueued})
}

func (h Handler) Task(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var task model.AITask
	if err := h.db.First(&task, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "task_not_found", nil)
		return
	}
	if task.UserID != nil && *task.UserID != user.ID && user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin && user.Role != model.RoleReviewer {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return
	}
	response.OK(ctx, gin.H{"task": task})
}

func (h Handler) AdminTasks(ctx *gin.Context) {
	var tasks []model.AITask
	if err := h.db.Order("created_at desc").Limit(100).Find(&tasks).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"tasks": tasks})
}

func (h Handler) AdminDrafts(ctx *gin.Context) {
	var drafts []model.AIDraft
	if err := h.db.Order("created_at desc").Limit(100).Find(&drafts).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"drafts": drafts})
}

func (h Handler) ApproveDraft(ctx *gin.Context) {
	h.reviewDraft(ctx, model.StatusApproved)
}

func (h Handler) RejectDraft(ctx *gin.Context) {
	h.reviewDraft(ctx, model.StatusRejected)
}

func (h Handler) reviewDraft(ctx *gin.Context, status string) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req reviewDraftRequest
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
	var draft model.AIDraft
	if err := h.db.First(&draft, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "draft_not_found", nil)
		return
	}
	if !isReviewableDraftStatus(draft.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "draft_not_reviewable", gin.H{"status": draft.Status})
		return
	}
	previousStatus := draft.Status
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.AIDraft{}).Where("id = ?", ctx.Param("id")).Updates(map[string]interface{}{
			"status":        status,
			"reviewer_id":   user.ID,
			"reviewed_at":   gorm.Expr("CURRENT_TIMESTAMP"),
			"review_reason": reason,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		var task model.AITask
		if err := tx.Select("id", "user_id").First(&task, "id = ?", draft.TaskID).Error; err != nil {
			return err
		}
		if task.UserID != nil {
			if err := notification.CreateReviewNotification(tx, notification.ReviewNotificationInput{
				UserID:        *task.UserID,
				ResourceType:  "ai_draft",
				ResourceID:    draft.ID,
				ResourceTitle: draft.OutputType,
				Status:        status,
				Reason:        reason,
			}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, "ai_draft."+status, "ai_draft", draft.ID, map[string]interface{}{
			"taskId":         draft.TaskID,
			"outputType":     draft.OutputType,
			"previousStatus": previousStatus,
			"status":         status,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "draft_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func isReviewableDraftStatus(status string) bool {
	switch status {
	case model.StatusDraft, model.StatusPending, model.StatusNeedsChanges:
		return true
	default:
		return false
	}
}

func (h Handler) enqueue(ctx context.Context, task model.AITask) bool {
	if h.cache == nil {
		return false
	}
	err := h.cache.XAdd(ctx, &redislib.XAddArgs{
		Stream: h.taskStream,
		Values: map[string]interface{}{
			"task_id": task.ID,
			"type":    task.Type,
		},
	}).Err()
	return err == nil
}
