package wiki

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
	"final-review-platform/services/api/internal/notification"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db *gorm.DB
}

var (
	errEntryNotReviewable              = errors.New("entry_not_reviewable")
	errProposalNotReviewable           = errors.New("proposal_not_reviewable")
	errEntryNotEditable                = errors.New("entry_not_editable")
	errProposalNotEditable             = errors.New("proposal_not_editable")
	errProposalStale                   = errors.New("proposal_stale")
	errCreatorApplicationNotReviewable = errors.New("creator_application_not_reviewable")
	reviewableStatuses                 = []string{model.StatusPending, model.StatusDraft, model.StatusNeedsChanges}
	userEditableStatuses               = []string{model.StatusDraft, model.StatusPending, model.StatusNeedsChanges, model.StatusRejected}
)

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type entryRequest struct {
	CourseID string `json:"courseId"`
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Content  string `json:"content"`
	Summary  string `json:"summary"`
}

type reviewRequest struct {
	ReviewReason string `json:"reviewReason"`
}

type proposalRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Summary string `json:"summary"`
}

type creatorApplicationRequest struct {
	Reason      string `json:"reason"`
	SampleTitle string `json:"sampleTitle"`
	SampleBody  string `json:"sampleBody"`
}

type publicEntry struct {
	ID           string `json:"id"`
	AuthorID     string `json:"authorId"`
	CourseID     string `json:"courseId,omitempty"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	Content      string `json:"content"`
	Version      int    `json:"version"`
	Visibility   string `json:"visibility"`
	LikeCount    int64  `json:"likeCount"`
	CommentCount int64  `json:"commentCount"`
	CollectCount int64  `json:"collectCount"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type adminProposal struct {
	model.WikiEditProposal
	CurrentTitle   string `json:"currentTitle"`
	CurrentContent string `json:"currentContent"`
	CurrentVersion int    `json:"currentVersion"`
	CurrentStatus  string `json:"currentStatus"`
	BaseContent    string `json:"baseContent"`
	BaseSummary    string `json:"baseSummary"`
	IsStale        bool   `json:"isStale"`
}

type creatorApplicationSelf struct {
	ID           string `json:"id"`
	Reason       string `json:"reason"`
	SampleTitle  string `json:"sampleTitle"`
	SampleBody   string `json:"sampleBody"`
	Status       string `json:"status"`
	ReviewedAt   string `json:"reviewedAt,omitempty"`
	ReviewReason string `json:"reviewReason,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type myEntry struct {
	ID           string `json:"id"`
	CourseID     string `json:"courseId,omitempty"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	Content      string `json:"content"`
	Version      int    `json:"version"`
	Status       string `json:"status"`
	Visibility   string `json:"visibility"`
	ReviewedAt   string `json:"reviewedAt,omitempty"`
	ReviewReason string `json:"reviewReason,omitempty"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

type myProposal struct {
	ID              string `json:"id"`
	EntryID         string `json:"entryId"`
	EntryTitle      string `json:"entryTitle"`
	EntryStatus     string `json:"entryStatus"`
	BaseVersion     int    `json:"baseVersion"`
	ProposedTitle   string `json:"proposedTitle"`
	ProposedContent string `json:"proposedContent"`
	Summary         string `json:"summary"`
	Status          string `json:"status"`
	ReviewedAt      string `json:"reviewedAt,omitempty"`
	ReviewReason    string `json:"reviewReason,omitempty"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

func (h Handler) ListPublished(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 50, 100)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Model(&model.WikiEntry{}).
		Joins("LEFT JOIN courses ON courses.id = wiki_entries.course_id").
		Where("wiki_entries.status = ? AND wiki_entries.visibility = ? AND (wiki_entries.course_id IS NULL OR courses.status = ?)", model.StatusPublished, "public", model.StatusPublished)
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("wiki_entries.course_id = ?", courseID)
	}
	var entries []model.WikiEntry
	if err := query.Order("wiki_entries.updated_at desc").Limit(limit).Find(&entries).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entries": publicEntries(entries)})
}

func (h Handler) Detail(ctx *gin.Context) {
	var entry model.WikiEntry
	if err := h.db.Model(&model.WikiEntry{}).
		Joins("LEFT JOIN courses ON courses.id = wiki_entries.course_id").
		Where("wiki_entries.id = ? AND wiki_entries.status = ? AND wiki_entries.visibility = ? AND (wiki_entries.course_id IS NULL OR courses.status = ?)", ctx.Param("id"), model.StatusPublished, "public", model.StatusPublished).
		First(&entry).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
		return
	}
	response.OK(ctx, gin.H{"entry": toPublicEntry(entry)})
}

func (h Handler) MyEntries(ctx *gin.Context) {
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
	var entries []model.WikiEntry
	if err := h.db.Where("author_id = ?", user.ID).Order("updated_at desc").Limit(limit).Find(&entries).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entries": myEntries(entries)})
}

func (h Handler) MyProposals(ctx *gin.Context) {
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
	var proposals []model.WikiEditProposal
	if err := h.db.Where("editor_id = ?", user.ID).Order("updated_at desc").Limit(limit).Find(&proposals).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	entriesByID, err := h.entriesByProposalEntryID(proposals)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"proposals": myProposals(proposals, entriesByID)})
}

func (h Handler) ResubmitEntry(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req entryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	title := strings.TrimSpace(req.Title)
	slug := strings.TrimSpace(req.Slug)
	content := strings.TrimSpace(req.Content)
	summary := strings.TrimSpace(req.Summary)
	courseID := strings.TrimSpace(req.CourseID)
	if err := validateEntryInput(title, slug, content, summary); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}

	var entry model.WikiEntry
	if err := h.db.First(&entry, "id = ? AND author_id = ?", ctx.Param("id"), user.ID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
		return
	}
	if !isUserEditableStatus(entry.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "entry_not_editable", gin.H{"status": entry.Status})
		return
	}
	var courseIDPtr *string
	if courseID != "" {
		var course model.Course
		if err := h.db.First(&course, "id = ? AND status = ?", courseID, model.StatusPublished).Error; err != nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "course_not_found", nil)
			return
		}
		courseIDPtr = &course.ID
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WikiEntry{}).
			Where("id = ? AND author_id = ? AND status IN ?", entry.ID, user.ID, userEditableStatuses).
			Updates(map[string]interface{}{
				"course_id":     courseIDPtr,
				"title":         title,
				"slug":          slug,
				"content":       content,
				"status":        model.StatusPending,
				"reviewer_id":   nil,
				"reviewed_at":   nil,
				"review_reason": "",
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errEntryNotEditable
		}
		if summary == "" {
			summary = "resubmitted draft"
		}
		return tx.Model(&model.WikiEditHistory{}).
			Where("entry_id = ? AND version = ?", entry.ID, entry.Version).
			Updates(map[string]interface{}{"content": content, "summary": summary}).Error
	})
	if err != nil {
		if errors.Is(err, errEntryNotEditable) {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "entry_not_editable", gin.H{"status": entry.Status})
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "resubmit_failed", nil)
		return
	}
	if err := h.db.First(&entry, "id = ?", entry.ID).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entry": myEntries([]model.WikiEntry{entry})[0]})
}

func (h Handler) ResubmitProposal(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req proposalRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	summary := strings.TrimSpace(req.Summary)
	if err := validateProposalInput(title, content, summary); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}

	var proposal model.WikiEditProposal
	if err := h.db.First(&proposal, "id = ? AND editor_id = ?", ctx.Param("id"), user.ID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "proposal_not_found", nil)
		return
	}
	if !isUserEditableStatus(proposal.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "proposal_not_editable", gin.H{"status": proposal.Status})
		return
	}
	var entry model.WikiEntry
	if err := h.db.Model(&model.WikiEntry{}).
		Joins("LEFT JOIN courses ON courses.id = wiki_entries.course_id").
		Where("wiki_entries.id = ? AND wiki_entries.status = ? AND wiki_entries.visibility = ? AND (wiki_entries.course_id IS NULL OR courses.status = ?)", proposal.EntryID, model.StatusPublished, "public", model.StatusPublished).
		First(&entry).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
		return
	}
	if title == entry.Title && content == entry.Content {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "proposal_unchanged", nil)
		return
	}

	result := h.db.Model(&model.WikiEditProposal{}).
		Where("id = ? AND editor_id = ? AND status IN ?", proposal.ID, user.ID, userEditableStatuses).
		Updates(map[string]interface{}{
			"base_version":     entry.Version,
			"proposed_title":   title,
			"proposed_content": content,
			"summary":          summary,
			"status":           model.StatusPending,
			"reviewer_id":      nil,
			"reviewed_at":      nil,
			"review_reason":    "",
		})
	if result.Error != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "resubmit_failed", nil)
		return
	}
	if result.RowsAffected == 0 {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "proposal_not_editable", gin.H{"status": proposal.Status})
		return
	}
	if err := h.db.First(&proposal, "id = ?", proposal.ID).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"proposal": myProposals([]model.WikiEditProposal{proposal}, map[string]model.WikiEntry{entry.ID: entry})[0]})
}

func (h Handler) Create(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req entryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	title := strings.TrimSpace(req.Title)
	slug := strings.TrimSpace(req.Slug)
	content := strings.TrimSpace(req.Content)
	summary := strings.TrimSpace(req.Summary)
	courseID := strings.TrimSpace(req.CourseID)
	if err := validateEntryInput(title, slug, content, summary); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	var courseIDPtr *string
	if courseID != "" {
		var course model.Course
		if err := h.db.First(&course, "id = ? AND status = ?", courseID, model.StatusPublished).Error; err != nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "course_not_found", nil)
			return
		}
		courseIDPtr = &course.ID
	}
	entry := model.WikiEntry{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		ContentStats: model.ContentStats{Visibility: "public"},
		AuthorID:     user.ID,
		CourseID:     courseIDPtr,
		Title:        title,
		Slug:         slug,
		Content:      content,
		Version:      1,
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if summary == "" {
			summary = "initial submission"
		}
		history := model.WikiEditHistory{
			EntryID:  entry.ID,
			EditorID: user.ID,
			Version:  entry.Version,
			Content:  entry.Content,
			Summary:  summary,
		}
		return tx.Create(&history).Error
	})
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entry": entry})
}

func (h Handler) CreateProposal(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req proposalRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	title := strings.TrimSpace(req.Title)
	content := strings.TrimSpace(req.Content)
	summary := strings.TrimSpace(req.Summary)
	if err := validateProposalInput(title, content, summary); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	var entry model.WikiEntry
	if err := h.db.Model(&model.WikiEntry{}).
		Joins("LEFT JOIN courses ON courses.id = wiki_entries.course_id").
		Where("wiki_entries.id = ? AND wiki_entries.status = ? AND wiki_entries.visibility = ? AND (wiki_entries.course_id IS NULL OR courses.status = ?)", ctx.Param("id"), model.StatusPublished, "public", model.StatusPublished).
		First(&entry).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
		return
	}
	if title == entry.Title && content == entry.Content {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "proposal_unchanged", nil)
		return
	}
	proposal := model.WikiEditProposal{
		ReviewFields:    model.ReviewFields{Status: model.StatusPending},
		EntryID:         entry.ID,
		EditorID:        user.ID,
		BaseVersion:     entry.Version,
		ProposedTitle:   title,
		ProposedContent: content,
		Summary:         summary,
	}
	if err := h.db.Create(&proposal).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"proposal": proposal})
}

func (h Handler) MyCreatorApplications(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 20, 100)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var applications []model.WikiCreatorApplication
	if err := h.db.
		Where("user_id = ?", user.ID).
		Order("updated_at desc").
		Limit(limit).
		Find(&applications).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"applications": creatorApplicationsForSelf(applications)})
}

func (h Handler) CreateCreatorApplication(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	if isCreatorCapableRole(user.Role) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "already_creator", nil)
		return
	}
	if user.Role != model.RoleUser {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "creator_application_role_not_supported", nil)
		return
	}

	var req creatorApplicationRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	sampleTitle := strings.TrimSpace(req.SampleTitle)
	sampleBody := strings.TrimSpace(req.SampleBody)
	if err := validateCreatorApplicationInput(reason, sampleTitle, sampleBody); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}

	var pendingCount int64
	if err := h.db.Model(&model.WikiCreatorApplication{}).
		Where("user_id = ? AND status IN ?", user.ID, []string{model.StatusPending, model.StatusDraft, model.StatusNeedsChanges}).
		Count(&pendingCount).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if pendingCount > 0 {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "creator_application_pending", nil)
		return
	}

	application := model.WikiCreatorApplication{
		ReviewFields: model.ReviewFields{Status: model.StatusPending},
		UserID:       user.ID,
		Reason:       reason,
		SampleTitle:  sampleTitle,
		SampleBody:   sampleBody,
	}
	if err := h.db.Create(&application).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"application": application})
}

func (h Handler) AdminEntries(ctx *gin.Context) {
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
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	var entries []model.WikiEntry
	if err := query.Order("updated_at desc").Limit(limit).Find(&entries).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"entries": entries})
}

func (h Handler) AdminProposals(ctx *gin.Context) {
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
	if entryID := strings.TrimSpace(ctx.Query("entryId")); entryID != "" {
		query = query.Where("entry_id = ?", entryID)
	}
	if editorID := strings.TrimSpace(ctx.Query("editorId")); editorID != "" {
		query = query.Where("editor_id = ?", editorID)
	}
	var proposals []model.WikiEditProposal
	if err := query.Order("updated_at desc").Limit(limit).Find(&proposals).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	result, err := h.adminProposalsWithContext(proposals)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"proposals": result})
}

func (h Handler) AdminCreatorApplications(ctx *gin.Context) {
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
	if userID := strings.TrimSpace(ctx.Query("userId")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	var applications []model.WikiCreatorApplication
	if err := query.Order("updated_at desc").Limit(limit).Find(&applications).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"applications": applications})
}

func (h Handler) ApproveEntry(ctx *gin.Context) {
	h.reviewEntry(ctx, model.StatusPublished)
}

func (h Handler) RejectEntry(ctx *gin.Context) {
	h.reviewEntry(ctx, model.StatusRejected)
}

func (h Handler) ApproveProposal(ctx *gin.Context) {
	h.reviewProposal(ctx, model.StatusPublished)
}

func (h Handler) RejectProposal(ctx *gin.Context) {
	h.reviewProposal(ctx, model.StatusRejected)
}

func (h Handler) ApproveCreatorApplication(ctx *gin.Context) {
	h.reviewCreatorApplication(ctx, model.StatusApproved)
}

func (h Handler) RejectCreatorApplication(ctx *gin.Context) {
	h.reviewCreatorApplication(ctx, model.StatusRejected)
}

func (h Handler) reviewEntry(ctx *gin.Context, status string) {
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

	var entry model.WikiEntry
	if err := h.db.First(&entry, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
		return
	}
	if !isReviewableStatus(entry.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "entry_not_reviewable", gin.H{"status": entry.Status})
		return
	}
	previousStatus := entry.Status
	action := "wiki_entry.published"
	if status == model.StatusRejected {
		action = "wiki_entry.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WikiEntry{}).
			Where("id = ? AND status IN ?", entry.ID, reviewableStatuses).
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
			return errEntryNotReviewable
		}
		if err := notification.CreateReviewNotification(tx, notification.ReviewNotificationInput{
			UserID:        entry.AuthorID,
			ResourceType:  "wiki_entry",
			ResourceID:    entry.ID,
			ResourceTitle: entry.Title,
			Status:        status,
			Reason:        reason,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, action, "wiki_entry", entry.ID, map[string]interface{}{
			"authorId":       entry.AuthorID,
			"courseId":       entry.CourseID,
			"previousStatus": previousStatus,
			"status":         status,
			"version":        entry.Version,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, errEntryNotReviewable) {
			latestStatus := entry.Status
			var latest model.WikiEntry
			if queryErr := h.db.Select("status").First(&latest, "id = ?", entry.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "entry_not_reviewable", gin.H{"status": latestStatus})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "entry_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func (h Handler) reviewProposal(ctx *gin.Context, status string) {
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

	var proposal model.WikiEditProposal
	if err := h.db.First(&proposal, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "proposal_not_found", nil)
		return
	}
	if !isReviewableStatus(proposal.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "proposal_not_reviewable", gin.H{"status": proposal.Status})
		return
	}
	previousStatus := proposal.Status
	action := "wiki_proposal.published"
	if status == model.StatusRejected {
		action = "wiki_proposal.rejected"
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WikiEditProposal{}).
			Where("id = ? AND status IN ?", proposal.ID, reviewableStatuses).
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
			return errProposalNotReviewable
		}
		if status == model.StatusPublished {
			entryUpdate := tx.Model(&model.WikiEntry{}).
				Where("id = ? AND status = ? AND version = ?", proposal.EntryID, model.StatusPublished, proposal.BaseVersion).
				Updates(map[string]interface{}{
					"title":         proposal.ProposedTitle,
					"content":       proposal.ProposedContent,
					"version":       proposal.BaseVersion + 1,
					"reviewer_id":   user.ID,
					"reviewed_at":   gorm.Expr("CURRENT_TIMESTAMP"),
					"review_reason": reason,
				})
			if entryUpdate.Error != nil {
				return entryUpdate.Error
			}
			if entryUpdate.RowsAffected == 0 {
				return errProposalStale
			}
			summary := strings.TrimSpace(proposal.Summary)
			if summary == "" {
				summary = "approved edit proposal"
			}
			history := model.WikiEditHistory{
				EntryID:  proposal.EntryID,
				EditorID: proposal.EditorID,
				Version:  proposal.BaseVersion + 1,
				Content:  proposal.ProposedContent,
				Summary:  summary,
			}
			if err := tx.Create(&history).Error; err != nil {
				return err
			}
		}
		if err := notification.CreateReviewNotification(tx, notification.ReviewNotificationInput{
			UserID:        proposal.EditorID,
			ResourceType:  "wiki_proposal",
			ResourceID:    proposal.ID,
			ResourceTitle: proposal.ProposedTitle,
			Status:        status,
			Reason:        reason,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, action, "wiki_edit_proposal", proposal.ID, map[string]interface{}{
			"entryId":        proposal.EntryID,
			"editorId":       proposal.EditorID,
			"baseVersion":    proposal.BaseVersion,
			"nextVersion":    proposal.BaseVersion + 1,
			"previousStatus": previousStatus,
			"status":         status,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, errProposalNotReviewable) {
			latestStatus := proposal.Status
			var latest model.WikiEditProposal
			if queryErr := h.db.Select("status").First(&latest, "id = ?", proposal.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "proposal_not_reviewable", gin.H{"status": latestStatus})
			return
		}
		if errors.Is(err, errProposalStale) {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "proposal_stale", gin.H{"baseVersion": proposal.BaseVersion})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "proposal_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func (h Handler) reviewCreatorApplication(ctx *gin.Context, status string) {
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

	var application model.WikiCreatorApplication
	if err := h.db.First(&application, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "creator_application_not_found", nil)
		return
	}
	if !isReviewableStatus(application.Status) {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "creator_application_not_reviewable", gin.H{"status": application.Status})
		return
	}
	previousStatus := application.Status
	action := "wiki_creator_application.approved"
	if status == model.StatusRejected {
		action = "wiki_creator_application.rejected"
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.WikiCreatorApplication{}).
			Where("id = ? AND status IN ?", application.ID, reviewableStatuses).
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
			return errCreatorApplicationNotReviewable
		}
		if status == model.StatusApproved {
			var applicant model.User
			if err := tx.First(&applicant, "id = ?", application.UserID).Error; err != nil {
				return err
			}
			if applicant.Role == model.RoleUser {
				if err := tx.Model(&model.User{}).Where("id = ?", applicant.ID).Update("role", model.RoleCreator).Error; err != nil {
					return err
				}
			}
		}
		if err := notification.CreateReviewNotification(tx, notification.ReviewNotificationInput{
			UserID:        application.UserID,
			ResourceType:  "wiki_creator_application",
			ResourceID:    application.ID,
			ResourceTitle: application.SampleTitle,
			Status:        status,
			Reason:        reason,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, action, "wiki_creator_application", application.ID, map[string]interface{}{
			"userId":         application.UserID,
			"previousStatus": previousStatus,
			"status":         status,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, errCreatorApplicationNotReviewable) {
			latestStatus := application.Status
			var latest model.WikiCreatorApplication
			if queryErr := h.db.Select("status").First(&latest, "id = ?", application.ID).Error; queryErr == nil {
				latestStatus = latest.Status
			}
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "creator_application_not_reviewable", gin.H{"status": latestStatus})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "creator_application_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func validateEntryInput(title string, slug string, content string, summary string) error {
	if title == "" || slug == "" || content == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(title)) > 200 {
		return errors.New("title_too_long")
	}
	if len(slug) > 220 || !safeSlug(slug) {
		return errors.New("invalid_slug")
	}
	if len([]rune(content)) > 80000 {
		return errors.New("content_too_long")
	}
	if len([]rune(summary)) > 500 {
		return errors.New("summary_too_long")
	}
	return nil
}

func validateCreatorApplicationInput(reason string, sampleTitle string, sampleBody string) error {
	if reason == "" || sampleTitle == "" || sampleBody == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(reason)) > 1000 {
		return errors.New("reason_too_long")
	}
	if len([]rune(sampleTitle)) > 200 {
		return errors.New("sample_title_too_long")
	}
	if len([]rune(sampleBody)) > 20000 {
		return errors.New("sample_body_too_long")
	}
	return nil
}

func validateProposalInput(title string, content string, summary string) error {
	if title == "" || content == "" {
		return errors.New("missing_required_fields")
	}
	if len([]rune(title)) > 200 {
		return errors.New("title_too_long")
	}
	if len([]rune(content)) > 80000 {
		return errors.New("content_too_long")
	}
	if len([]rune(summary)) > 500 {
		return errors.New("summary_too_long")
	}
	return nil
}

func publicEntries(entries []model.WikiEntry) []publicEntry {
	result := make([]publicEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, toPublicEntry(entry))
	}
	return result
}

func myEntries(entries []model.WikiEntry) []myEntry {
	result := make([]myEntry, 0, len(entries))
	for _, entry := range entries {
		var courseID string
		if entry.CourseID != nil {
			courseID = *entry.CourseID
		}
		var reviewedAt string
		if entry.ReviewedAt != nil {
			reviewedAt = entry.ReviewedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		result = append(result, myEntry{
			ID:           entry.ID,
			CourseID:     courseID,
			Title:        entry.Title,
			Slug:         entry.Slug,
			Content:      entry.Content,
			Version:      entry.Version,
			Status:       entry.Status,
			Visibility:   entry.Visibility,
			ReviewedAt:   reviewedAt,
			ReviewReason: entry.ReviewReason,
			CreatedAt:    entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    entry.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return result
}

func myProposals(proposals []model.WikiEditProposal, entriesByID map[string]model.WikiEntry) []myProposal {
	result := make([]myProposal, 0, len(proposals))
	for _, proposal := range proposals {
		var reviewedAt string
		if proposal.ReviewedAt != nil {
			reviewedAt = proposal.ReviewedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		entry := entriesByID[proposal.EntryID]
		result = append(result, myProposal{
			ID:              proposal.ID,
			EntryID:         proposal.EntryID,
			EntryTitle:      entry.Title,
			EntryStatus:     entry.Status,
			BaseVersion:     proposal.BaseVersion,
			ProposedTitle:   proposal.ProposedTitle,
			ProposedContent: proposal.ProposedContent,
			Summary:         proposal.Summary,
			Status:          proposal.Status,
			ReviewedAt:      reviewedAt,
			ReviewReason:    proposal.ReviewReason,
			CreatedAt:       proposal.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:       proposal.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return result
}

func creatorApplicationsForSelf(applications []model.WikiCreatorApplication) []creatorApplicationSelf {
	result := make([]creatorApplicationSelf, 0, len(applications))
	for _, application := range applications {
		var reviewedAt string
		if application.ReviewedAt != nil {
			reviewedAt = application.ReviewedAt.Format("2006-01-02T15:04:05Z07:00")
		}
		result = append(result, creatorApplicationSelf{
			ID:           application.ID,
			Reason:       application.Reason,
			SampleTitle:  application.SampleTitle,
			SampleBody:   application.SampleBody,
			Status:       application.Status,
			ReviewedAt:   reviewedAt,
			ReviewReason: application.ReviewReason,
			CreatedAt:    application.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    application.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return result
}

func (h Handler) entriesByProposalEntryID(proposals []model.WikiEditProposal) (map[string]model.WikiEntry, error) {
	if len(proposals) == 0 {
		return map[string]model.WikiEntry{}, nil
	}
	entryIDSet := make(map[string]struct{}, len(proposals))
	for _, proposal := range proposals {
		entryIDSet[proposal.EntryID] = struct{}{}
	}
	entryIDs := make([]string, 0, len(entryIDSet))
	for entryID := range entryIDSet {
		entryIDs = append(entryIDs, entryID)
	}
	var entries []model.WikiEntry
	if err := h.db.Where("id IN ?", entryIDs).Find(&entries).Error; err != nil {
		return nil, err
	}
	entriesByID := make(map[string]model.WikiEntry, len(entries))
	for _, entry := range entries {
		entriesByID[entry.ID] = entry
	}
	return entriesByID, nil
}

func (h Handler) adminProposalsWithContext(proposals []model.WikiEditProposal) ([]adminProposal, error) {
	if len(proposals) == 0 {
		return []adminProposal{}, nil
	}
	entryIDSet := make(map[string]struct{}, len(proposals))
	for _, proposal := range proposals {
		entryIDSet[proposal.EntryID] = struct{}{}
	}
	entryIDs := make([]string, 0, len(entryIDSet))
	for entryID := range entryIDSet {
		entryIDs = append(entryIDs, entryID)
	}

	var entries []model.WikiEntry
	if err := h.db.Where("id IN ?", entryIDs).Find(&entries).Error; err != nil {
		return nil, err
	}
	entriesByID := make(map[string]model.WikiEntry, len(entries))
	for _, entry := range entries {
		entriesByID[entry.ID] = entry
	}

	var histories []model.WikiEditHistory
	if err := h.db.Where("entry_id IN ?", entryIDs).Find(&histories).Error; err != nil {
		return nil, err
	}
	historiesByVersion := make(map[string]model.WikiEditHistory, len(histories))
	for _, history := range histories {
		historiesByVersion[history.EntryID+"#"+strconv.Itoa(history.Version)] = history
	}

	result := make([]adminProposal, 0, len(proposals))
	for _, proposal := range proposals {
		item := adminProposal{WikiEditProposal: proposal, IsStale: true}
		if entry, ok := entriesByID[proposal.EntryID]; ok {
			item.CurrentTitle = entry.Title
			item.CurrentContent = entry.Content
			item.CurrentVersion = entry.Version
			item.CurrentStatus = entry.Status
			item.IsStale = entry.Version != proposal.BaseVersion
		}
		if history, ok := historiesByVersion[proposal.EntryID+"#"+strconv.Itoa(proposal.BaseVersion)]; ok {
			item.BaseContent = history.Content
			item.BaseSummary = history.Summary
		}
		result = append(result, item)
	}
	return result, nil
}

func toPublicEntry(entry model.WikiEntry) publicEntry {
	var courseID string
	if entry.CourseID != nil {
		courseID = *entry.CourseID
	}
	return publicEntry{
		ID:           entry.ID,
		AuthorID:     entry.AuthorID,
		CourseID:     courseID,
		Title:        entry.Title,
		Slug:         entry.Slug,
		Content:      entry.Content,
		Version:      entry.Version,
		Visibility:   entry.Visibility,
		LikeCount:    entry.LikeCount,
		CommentCount: entry.CommentCount,
		CollectCount: entry.CollectCount,
		CreatedAt:    entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    entry.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func safeSlug(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return true
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
	case model.StatusDraft, model.StatusPending, model.StatusNeedsChanges, model.StatusApproved, model.StatusPublished, model.StatusRejected:
		return true
	default:
		return false
	}
}

func isCreatorCapableRole(role string) bool {
	switch role {
	case model.RoleCreator, model.RoleAdmin, model.RoleSuperAdmin:
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
