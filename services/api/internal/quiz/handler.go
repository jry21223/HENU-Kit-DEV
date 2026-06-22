package quiz

import (
	"errors"
	"net/http"
	"time"

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

type optionDTO struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Content   string `json:"content"`
	SortOrder int    `json:"sortOrder"`
}

type questionDTO struct {
	ID               string      `json:"id"`
	CourseID         string      `json:"courseId"`
	KnowledgePointID *string     `json:"knowledgePointId,omitempty"`
	Type             string      `json:"type"`
	Stem             string      `json:"stem"`
	Difficulty       int         `json:"difficulty"`
	Options          []optionDTO `json:"options,omitempty"`
}

func (h Handler) CourseQuestions(ctx *gin.Context) {
	var questions []model.QuizQuestion
	if err := h.db.Where("course_id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).
		Order("created_at asc").
		Find(&questions).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"questions": h.toQuestionDTOs(questions)})
}

func (h Handler) Question(ctx *gin.Context) {
	question, ok := h.findPublishedQuestion(ctx)
	if !ok {
		return
	}
	response.OK(ctx, gin.H{"question": h.toQuestionDTO(question)})
}

func (h Handler) Submit(ctx *gin.Context) {
	question, ok := h.findPublishedQuestion(ctx)
	if !ok {
		return
	}

	var request struct {
		Answer string `json:"answer"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}

	result := Judge(question.Type, question.Answer, request.Answer)
	if user, hasUser := auth.CurrentUser(ctx); hasUser {
		if err := h.recordQuizAnswer(user.ID, question, request.Answer, result); err != nil {
			response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "record_failed", nil)
			return
		}
		if !result.IsCorrect {
			if err := h.upsertWrongQuestion(user.ID, question, request.Answer); err != nil {
				response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "record_failed", nil)
				return
			}
		}
	}

	response.OK(ctx, gin.H{
		"isCorrect":   result.IsCorrect,
		"score":       result.Score,
		"explanation": question.Explanation,
	})
}

func (h Handler) WrongQuestions(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}

	query := h.db.Where("user_id = ?", user.ID)
	if courseID := ctx.Query("courseId"); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}

	var wrongQuestions []model.WrongQuestion
	if err := query.Order("updated_at desc").Find(&wrongQuestions).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"wrongQuestions": wrongQuestions})
}

func (h Handler) DeleteWrongQuestion(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}

	result := h.db.Where("id = ? AND user_id = ?", ctx.Param("id"), user.ID).Delete(&model.WrongQuestion{})
	if result.Error != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "delete_failed", nil)
		return
	}
	if result.RowsAffected == 0 {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "wrong_question_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"deleted": true})
}

func (h Handler) WeaknessReport(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}

	type row struct {
		CourseID   string `json:"courseId"`
		WrongCount int64  `json:"wrongCount"`
	}
	var rows []row
	query := h.db.Model(&model.WrongQuestion{}).
		Select("course_id, SUM(wrong_count) as wrong_count").
		Where("user_id = ?", user.ID).
		Group("course_id").
		Order("wrong_count desc")
	if err := query.Scan(&rows).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"weakness": rows})
}

func (h Handler) findPublishedQuestion(ctx *gin.Context) (model.QuizQuestion, bool) {
	var question model.QuizQuestion
	if err := h.db.First(&question, "id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "question_not_found", nil)
		return question, false
	}
	return question, true
}

func (h Handler) toQuestionDTOs(questions []model.QuizQuestion) []questionDTO {
	result := make([]questionDTO, 0, len(questions))
	for _, question := range questions {
		result = append(result, h.toQuestionDTO(question))
	}
	return result
}

func (h Handler) toQuestionDTO(question model.QuizQuestion) questionDTO {
	var options []model.QuizOption
	h.db.Where("question_id = ?", question.ID).Order("sort_order asc").Find(&options)

	return questionDTO{
		ID:               question.ID,
		CourseID:         question.CourseID,
		KnowledgePointID: question.KnowledgePointID,
		Type:             question.Type,
		Stem:             question.Stem,
		Difficulty:       question.Difficulty,
		Options:          toOptionDTOs(options),
	}
}

func toOptionDTOs(options []model.QuizOption) []optionDTO {
	result := make([]optionDTO, 0, len(options))
	for _, option := range options {
		result = append(result, optionDTO{
			ID:        option.ID,
			Label:     option.Label,
			Content:   option.Content,
			SortOrder: option.SortOrder,
		})
	}
	return result
}

func (h Handler) recordQuizAnswer(userID string, question model.QuizQuestion, submitted string, result JudgeResult) error {
	attempt := model.QuizAttempt{
		UserID:     userID,
		CourseID:   question.CourseID,
		Mode:       "practice",
		Score:      result.Score,
		StartedAt:  time.Now(),
		FinishedAt: ptrTime(time.Now()),
	}
	if err := h.db.Create(&attempt).Error; err != nil {
		return err
	}
	answer := model.QuizAnswer{
		AttemptID:  attempt.ID,
		QuestionID: question.ID,
		UserID:     userID,
		Answer:     submitted,
		IsCorrect:  result.IsCorrect,
		Score:      result.Score,
	}
	return h.db.Create(&answer).Error
}

func (h Handler) upsertWrongQuestion(userID string, question model.QuizQuestion, submitted string) error {
	var wrong model.WrongQuestion
	err := h.db.Where("user_id = ? AND question_id = ?", userID, question.ID).First(&wrong).Error
	if err == nil {
		return h.db.Model(&wrong).Updates(map[string]interface{}{
			"wrong_count": wrong.WrongCount + 1,
			"last_answer": submitted,
		}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	wrong = model.WrongQuestion{
		UserID:     userID,
		QuestionID: question.ID,
		CourseID:   question.CourseID,
		WrongCount: 1,
		LastAnswer: submitted,
	}
	return h.db.Create(&wrong).Error
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
