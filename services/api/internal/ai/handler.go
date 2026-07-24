package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

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

var baseTaskPointCosts = map[string]int64{
	"chat":                    2,
	"wrong_question_analysis": 5,
	"targeted_question":       10,
	"paper_generation":        30,
	"draft_review":            15,
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
	quota, err := h.createTaskWithQuota(ctx, user, &task)
	if err != nil {
		status := http.StatusBadRequest
		code := response.CodeBadRequest
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
			code = response.CodeNotFound
		}
		// Map known error strings to safe user-facing codes; default to generic message.
		errCode := "task_creation_failed"
		if _, ok := err.(errString); ok {
			errCode = err.Error()
		}
		response.Error(ctx, status, code, errCode, nil)
		return
	}

	enqueued := h.enqueue(ctx.Request.Context(), task)
	response.OK(ctx, gin.H{"task": task, "enqueued": enqueued, "quota": quota})
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

func (h Handler) PublishDraft(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var draft model.AIDraft
	if err := h.db.First(&draft, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "draft_not_found", nil)
		return
	}
	if draft.PublishedID != nil {
		var question model.QuizQuestion
		if err := h.db.First(&question, "id = ?", *draft.PublishedID).Error; err != nil {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "published_resource_missing", nil)
			return
		}
		response.OK(ctx, gin.H{"published": true, "alreadyPublished": true, "resourceType": "quiz_question", "question": question})
		return
	}
	if draft.Status != model.StatusApproved {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "draft_not_publishable", gin.H{"status": draft.Status})
		return
	}
	if draft.OutputType != "targeted_question" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsupported_publish_target", gin.H{"outputType": draft.OutputType})
		return
	}
	if draft.CourseID == nil || strings.TrimSpace(*draft.CourseID) == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "course_required", nil)
		return
	}

	questionDraft, err := parseQuestionDraft(draft.DraftContent)
	if err != nil {
		// parseQuestionDraft returns safe errString values; map to a safe code.
		errCode := "invalid_draft_content"
		if _, ok := err.(errString); ok {
			errCode = err.Error()
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, errCode, nil)
		return
	}

	var question model.QuizQuestion
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var course model.Course
		if err := tx.First(&course, "id = ? AND status = ?", *draft.CourseID, model.StatusPublished).Error; err != nil {
			return errString("course_not_found")
		}
		question = model.QuizQuestion{
			CourseID:    course.ID,
			Type:        questionDraft.Type,
			Stem:        questionDraft.Stem,
			Answer:      questionDraft.Answer,
			Explanation: questionDraft.Explanation,
			Difficulty:  questionDraft.Difficulty,
			Status:      model.StatusPublished,
			AuthorID:    &user.ID,
		}
		if err := tx.Create(&question).Error; err != nil {
			return err
		}
		for index, option := range questionDraft.Options {
			item := model.QuizOption{
				QuestionID: question.ID,
				Label:      option.Label,
				Content:    option.Content,
				SortOrder:  index + 1,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		update := tx.Model(&model.AIDraft{}).
			Where("id = ? AND published_id IS NULL AND status = ?", draft.ID, model.StatusApproved).
			Updates(map[string]interface{}{"status": model.StatusPublished, "published_id": question.ID})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return errString("draft_already_published")
		}
		return audit.Record(ctx, tx, "ai_draft.published", "ai_draft", draft.ID, map[string]interface{}{
			"outputType":   draft.OutputType,
			"resourceType": "quiz_question",
			"resourceId":   question.ID,
			"operatorId":   user.ID,
		})
	})
	if err != nil {
		if err.Error() == "course_not_found" {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "course_not_found", nil)
			return
		}
		if err.Error() == "draft_already_published" {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "draft_already_published", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "publish_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"published": true, "alreadyPublished": false, "resourceType": "quiz_question", "question": question})
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

type questionDraftPayload struct {
	Type         string                `json:"type"`
	QuestionType string                `json:"questionType"`
	Stem         string                `json:"stem"`
	Title        string                `json:"title"`
	Summary      string                `json:"summary"`
	Answer       string                `json:"answer"`
	Explanation  string                `json:"explanation"`
	Difficulty   int                   `json:"difficulty"`
	Options      []questionOptionDraft `json:"options"`
}

type questionOptionDraft struct {
	Label   string `json:"label"`
	Content string `json:"content"`
}

func parseQuestionDraft(content datatypes.JSON) (questionDraftPayload, error) {
	var payload questionDraftPayload
	if len(content) == 0 {
		return payload, errString("draft_content_required")
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return payload, errString("invalid_draft_content")
	}
	payload.Type = strings.TrimSpace(payload.Type)
	if payload.Type == "" {
		payload.Type = strings.TrimSpace(payload.QuestionType)
	}
	if payload.Type == "" {
		payload.Type = "single_choice"
	}
	if !isSupportedPublishQuestionType(payload.Type) {
		return payload, errString("unsupported_question_type")
	}
	payload.Stem = strings.TrimSpace(payload.Stem)
	if payload.Stem == "" {
		payload.Stem = strings.TrimSpace(payload.Title)
	}
	if payload.Stem == "" {
		payload.Stem = strings.TrimSpace(payload.Summary)
	}
	payload.Answer = strings.TrimSpace(payload.Answer)
	payload.Explanation = strings.TrimSpace(payload.Explanation)
	if payload.Stem == "" || payload.Answer == "" {
		return payload, errString("question_stem_and_answer_required")
	}
	if payload.Difficulty <= 0 {
		payload.Difficulty = 1
	}
	if payload.Difficulty > 5 {
		payload.Difficulty = 5
	}
	cleanOptions := make([]questionOptionDraft, 0, len(payload.Options))
	for _, option := range payload.Options {
		option.Label = strings.TrimSpace(option.Label)
		option.Content = strings.TrimSpace(option.Content)
		if option.Label == "" || option.Content == "" {
			return payload, errString("invalid_question_options")
		}
		cleanOptions = append(cleanOptions, option)
	}
	payload.Options = cleanOptions
	if (payload.Type == "single_choice" || payload.Type == "multiple_choice") && len(payload.Options) < 2 {
		return payload, errString("choice_options_required")
	}
	return payload, nil
}

func isSupportedPublishQuestionType(value string) bool {
	switch value {
	case "single_choice", "multiple_choice", "true_false", "fill_blank", "short_answer":
		return true
	default:
		return false
	}
}

func isReviewableDraftStatus(status string) bool {
	switch status {
	case model.StatusDraft, model.StatusPending, model.StatusNeedsChanges:
		return true
	default:
		return false
	}
}

type quotaResult struct {
	Source         string `json:"source"`
	PlanCode       string `json:"planCode,omitempty"`
	BaseCost       int64  `json:"baseCost"`
	PointsCost     int64  `json:"pointsCost"`
	BalanceAfter   int64  `json:"balanceAfter"`
	MembershipFree bool   `json:"membershipFree"`
}

func (h Handler) createTaskWithQuota(ctx *gin.Context, user *model.User, task *model.AITask) (quotaResult, error) {
	var quota quotaResult
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(task).Error; err != nil {
			return errString("create_failed")
		}
		var err error
		quota, err = consumeQuota(ctx, tx, user, *task)
		return err
	})
	return quota, err
}

func consumeQuota(ctx *gin.Context, tx *gorm.DB, user *model.User, task model.AITask) (quotaResult, error) {
	baseCost := baseTaskPointCosts[task.Type]
	quota := quotaResult{Source: "points", BaseCost: baseCost, PointsCost: baseCost}
	if isRoleExempt(user.Role) {
		quota.Source = "role_exempt"
		quota.PointsCost = 0
		return quota, createAIUsageLog(tx, user.ID, task.ID, quota)
	}

	planCode, err := currentPlanCode(tx, user.ID)
	if err != nil {
		return quota, err
	}
	quota.PlanCode = planCode
	switch planCode {
	case "tier2":
		quota.Source = "membership_tier2"
		quota.PointsCost = 0
		quota.MembershipFree = true
	case "tier1":
		if task.Type == "wrong_question_analysis" {
			quota.Source = "membership_tier1"
			quota.PointsCost = 0
			quota.MembershipFree = true
		} else {
			quota.Source = "membership_tier1_discount"
			quota.PointsCost = discountedCost(baseCost)
		}
	}

	if quota.PointsCost <= 0 {
		return quota, createAIUsageLog(tx, user.ID, task.ID, quota)
	}
	deduct := tx.Model(&model.User{}).
		Where("id = ? AND points_balance >= ?", user.ID, quota.PointsCost).
		UpdateColumn("points_balance", gorm.Expr("points_balance - ?", quota.PointsCost))
	if deduct.Error != nil {
		return quota, deduct.Error
	}
	if deduct.RowsAffected == 0 {
		return quota, errString("insufficient_ai_points")
	}
	var refreshed model.User
	if err := tx.Select("points_balance").First(&refreshed, "id = ?", user.ID).Error; err != nil {
		return quota, err
	}
	quota.BalanceAfter = refreshed.PointsBalance
	if err := tx.Create(&model.PointsLog{
		UserID:         user.ID,
		Delta:          -quota.PointsCost,
		BalanceAfter:   quota.BalanceAfter,
		Reason:         "ai_task_usage",
		ReferenceType:  "ai_task",
		ReferenceID:    task.ID,
		IdempotencyKey: "ai_task_usage:" + task.ID,
	}).Error; err != nil {
		return quota, err
	}
	return quota, createAIUsageLog(tx, user.ID, task.ID, quota)
}

func currentPlanCode(tx *gorm.DB, userID string) (string, error) {
	now := time.Now()
	var memberships []model.Membership
	if err := tx.Where("user_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", userID, "active", now).
		Order("created_at desc").
		Find(&memberships).Error; err != nil {
		return "", err
	}
	best := ""
	for _, membership := range memberships {
		if membershipPlanRank(membership.PlanCode) > membershipPlanRank(best) {
			best = membership.PlanCode
		}
	}
	return best, nil
}

func createAIUsageLog(tx *gorm.DB, userID string, taskID string, quota quotaResult) error {
	return tx.Create(&model.AIUsageLog{
		UserID:     &userID,
		TaskID:     &taskID,
		Model:      "quota",
		PointsCost: quota.PointsCost,
		Source:     quota.Source,
	}).Error
}

func discountedCost(cost int64) int64 {
	if cost <= 1 {
		return cost
	}
	return (cost + 1) / 2
}

func membershipPlanRank(code string) int {
	switch code {
	case "tier2":
		return 2
	case "tier1":
		return 1
	default:
		return 0
	}
}

func isRoleExempt(role string) bool {
	switch role {
	case model.RoleReviewer, model.RoleOperator, model.RoleAdmin, model.RoleSuperAdmin:
		return true
	default:
		return false
	}
}

type errString string

func (e errString) Error() string {
	return string(e)
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
