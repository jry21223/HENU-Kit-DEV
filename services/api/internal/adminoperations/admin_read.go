package adminoperations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/middleware"
)

func (h Handler) ListNotices(ctx *gin.Context) {
	query := h.db.Model(&model.CampusNotice{})
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	query.Count(&total)
	var items []model.CampusNotice
	if err := query.Order("created_at desc").Limit(listLimit(ctx)).Find(&items).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "通知列表读取失败", nil)
		return
	}
	writeSuccess(ctx, gin.H{"items": items, "total": total})
}

func (h Handler) ListNoticeVersions(ctx *gin.Context) {
	var notice model.CampusNotice
	if err := h.db.First(&notice, "id = ?", ctx.Param("id")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "NOTICE_NOT_FOUND", "通知不存在", nil)
		return
	}
	var versions []model.CampusNoticeVersion
	if err := h.db.Where("notice_id = ?", notice.ID).Order("version desc").Find(&versions).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "通知版本读取失败", nil)
		return
	}
	writeSuccess(ctx, gin.H{"notice_id": notice.ID, "title": notice.Title, "versions": versions})
}

func (h Handler) RevokeUserSessions(ctx *gin.Context) {
	current, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "not authenticated", nil)
		return
	}
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version is required", nil)
		return
	}
	var target model.User
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&target, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if target.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		if target.ID == current.ID {
			return errors.New("cannot revoke the current administrator session from this operation")
		}
		if err := tx.Model(&target).Updates(map[string]any{"token_version": target.TokenVersion + 1, "version": target.Version + 1}).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "user.sessions_revoked", "user", target.ID, map[string]any{"previous_token_version": target.TokenVersion, "token_version": target.TokenVersion + 1, "request_id": ensureRequestID(ctx)})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "user was updated by another operator", nil)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "USER_NOT_FOUND", "user was not found", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusForbidden, "SESSION_REVOCATION_FAILED", err.Error(), nil)
		return
	}
	h.db.First(&target, "id = ?", target.ID)
	writeSuccess(ctx, gin.H{"user_id": target.ID, "version": target.Version, "sessions_revoked": true})
}

func (h Handler) ListNoticeImportJobs(ctx *gin.Context) {
	var items []model.NoticeImportJob
	query := h.db.Model(&model.NoticeImportJob{})
	var total int64
	query.Count(&total)
	if err := query.Order("created_at desc").Limit(listLimit(ctx)).Find(&items).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "导入任务读取失败", nil)
		return
	}
	writeSuccess(ctx, gin.H{"items": items, "total": total})
}

func (h Handler) ReviewNotice(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "未登录", nil)
		return
	}
	var input struct {
		Decision        string `json:"decision"`
		Reason          string `json:"reason"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 || (input.Decision != "approve" && input.Decision != "reject") {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "decision 和 expected_version 必填", nil)
		return
	}
	if input.Decision == "reject" && strings.TrimSpace(input.Reason) == "" {
		writeError(ctx, http.StatusBadRequest, "REVIEW_REASON_REQUIRED", "驳回时必须填写原因", nil)
		return
	}
	var notice model.CampusNotice
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&notice, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if notice.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		before := map[string]any{"status": notice.Status, "version": notice.Version}
		now := time.Now().UTC()
		status := "approved"
		if input.Decision == "reject" {
			status = "rejected"
		}
		if err := tx.Model(&notice).Updates(map[string]any{
			"status": status, "review_reason": strings.TrimSpace(input.Reason), "reviewed_by": user.ID,
			"reviewed_at": now, "version": notice.Version + 1,
		}).Error; err != nil {
			return err
		}
		if status == "approved" {
			if err := h.distributeApprovedNotice(tx, notice); err != nil {
				return err
			}
			if err := tx.Model(&notice).Update("distribution_status", "distributed").Error; err != nil {
				return err
			}
			if err := createOutboxEvent(tx, "campus_notice", notice.ID, "notice.distributed.v1", map[string]any{"notice_id": notice.ID, "version": notice.CurrentVersion}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, "campus_notice.reviewed", "campus_notice", notice.ID, map[string]any{
			"before":     before,
			"after":      map[string]any{"status": status, "version": notice.Version + 1, "reason": strings.TrimSpace(input.Reason)},
			"request_id": ensureRequestID(ctx),
		})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "通知已被其他管理员更新", nil)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "NOTICE_NOT_FOUND", "通知不存在", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "NOTICE_REVIEW_FAILED", "通知审核失败", nil)
		return
	}
	h.db.First(&notice, "id = ?", notice.ID)
	writeSuccess(ctx, notice)
}

func (h Handler) distributeApprovedNotice(tx *gorm.DB, notice model.CampusNotice) error {
	var version model.CampusNoticeVersion
	if err := tx.Where("notice_id = ? AND version = ?", notice.ID, notice.CurrentVersion).First(&version).Error; err != nil {
		return err
	}
	var users []model.User
	if err := tx.Where("status = ? AND email_verified = ?", "active", true).Find(&users).Error; err != nil {
		return err
	}
	var subscriptions []model.NoticeEmailSubscription
	if err := tx.Where("enabled = ?", true).Find(&subscriptions).Error; err != nil {
		return err
	}
	subscribed := make(map[string]bool, len(subscriptions))
	for _, subscription := range subscriptions {
		subscribed[subscription.UserID] = true
	}
	category := "digest"
	if notice.Importance == "urgent" {
		category = "critical"
	} else if notice.Importance == "important" {
		category = "transactional"
	}
	for _, target := range users {
		if !noticeAudienceMatches(notice.Audience, target) {
			continue
		}
		stationReceipt := model.NoticeDistributionReceipt{NoticeID: notice.ID, UserID: target.ID, Channel: "in_app"}
		result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "notice_id"}, {Name: "user_id"}, {Name: "channel"}}, DoNothing: true}).Create(&stationReceipt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			body := version.Body
			if len([]rune(body)) > 240 {
				body = string([]rune(body)[:240]) + "…"
			}
			if err := tx.Create(&model.Notification{UserID: target.ID, Type: "campus_notice", Title: notice.Title, Body: body}).Error; err != nil {
				return err
			}
		}
		if !subscribed[target.ID] {
			continue
		}
		emailReceipt := model.NoticeDistributionReceipt{NoticeID: notice.ID, UserID: target.ID, Channel: "email"}
		result = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "notice_id"}, {Name: "user_id"}, {Name: "channel"}}, DoNothing: true}).Create(&emailReceipt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		recipient := strings.ToLower(target.Email)
		recipientSum := sha256.Sum256([]byte(recipient))
		enqueueSum := sha256.Sum256([]byte("notice\n" + notice.ID + "\n" + target.ID))
		requestSum := sha256.Sum256([]byte(notice.ContentHash + "\n" + recipient))
		enqueueKey := hex.EncodeToString(enqueueSum[:])
		recipientUserID := target.ID
		delivery := model.MailDelivery{
			EnqueueKey: enqueueKey, RequestHash: hex.EncodeToString(requestSum[:]), Category: category, Status: "queued",
			RecipientHash: hex.EncodeToString(recipientSum[:]), RecipientUserID: &recipientUserID, Recipient: recipient,
			TemplateCode: "campus_notice", Subject: notice.Title, Body: version.Body,
			RequestID: "req_notice_" + enqueueKey[:20], QueuedAt: time.Now().UTC(), Version: 1,
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "enqueue_key"}}, DoNothing: true}).Create(&delivery).Error; err != nil {
			return err
		}
	}
	return nil
}

func noticeAudienceMatches(raw []byte, user model.User) bool {
	var selectors []string
	if len(raw) == 0 || json.Unmarshal(raw, &selectors) != nil || len(selectors) == 0 {
		return true
	}
	for _, selector := range selectors {
		if selector == "all_verified_users" {
			return true
		}
		parts := strings.SplitN(selector, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[0] == "school" && user.SchoolID != nil && *user.SchoolID == parts[1] {
			return true
		}
		if parts[0] == "major" && user.MajorID != nil && *user.MajorID == parts[1] {
			return true
		}
	}
	return false
}

func (h Handler) ListMailOperations(ctx *gin.Context) {
	var deliveries []model.MailDelivery
	var deadLetters []model.MailDeadLetter
	query := h.db.Model(&model.MailDelivery{})
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	query.Count(&total)
	if err := query.Order("queued_at desc").Limit(listLimit(ctx)).Find(&deliveries).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "邮件投递读取失败", nil)
		return
	}
	h.db.Where("status = ?", "open").Order("created_at desc").Limit(listLimit(ctx)).Find(&deadLetters)
	var suppressions []model.MailSuppression
	h.db.Order("created_at desc").Limit(listLimit(ctx)).Find(&suppressions)
	var attempts []model.MailAttempt
	if len(deliveries) > 0 {
		ids := make([]string, 0, len(deliveries))
		for _, delivery := range deliveries {
			ids = append(ids, delivery.ID)
		}
		h.db.Where("delivery_id IN ?", ids).Order("created_at desc").Limit(listLimit(ctx) * 5).Find(&attempts)
	}
	writeSuccess(ctx, gin.H{"deliveries": deliveries, "attempts": attempts, "dead_letters": deadLetters, "suppressions": suppressions, "total": total})
}

func (h Handler) RetryMailDelivery(ctx *gin.Context) {
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version is required", nil)
		return
	}
	var delivery model.MailDelivery
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&delivery, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if delivery.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		if delivery.Status != "failed" {
			return errors.New("mail delivery is not retryable")
		}
		before := map[string]any{"status": delivery.Status, "version": delivery.Version}
		if err := tx.Model(&delivery).Updates(map[string]any{"status": "queued", "next_retry_at": nil, "locked_at": nil, "locked_by": "", "last_error_code": "", "version": delivery.Version + 1}).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "mail_delivery.retried", "mail_delivery", delivery.ID, map[string]any{"before": before, "after": map[string]any{"status": "queued", "version": delivery.Version + 1}, "request_id": ensureRequestID(ctx)})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "mail delivery was updated by another operator", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "MAIL_RETRY_FAILED", err.Error(), nil)
		return
	}
	h.db.First(&delivery, "id = ?", delivery.ID)
	writeSuccess(ctx, delivery)
}

func (h Handler) ReplayMailDeadLetter(ctx *gin.Context) {
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version is required", nil)
		return
	}
	var deadLetter model.MailDeadLetter
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deadLetter, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if deadLetter.Status != "open" {
			return errors.New("dead letter is not open")
		}
		var delivery model.MailDelivery
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&delivery, "id = ?", deadLetter.DeliveryID).Error; err != nil {
			return err
		}
		if delivery.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		now := time.Now().UTC()
		if err := tx.Model(&deadLetter).Updates(map[string]any{"status": "replayed", "resolved_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&delivery).Updates(map[string]any{"status": "queued", "attempt_count": 0, "next_retry_at": nil, "locked_at": nil, "locked_by": "", "last_error_code": "", "version": delivery.Version + 1}).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "mail_dead_letter.replayed", "mail_dead_letter", deadLetter.ID, map[string]any{"delivery_id": delivery.ID, "request_id": ensureRequestID(ctx)})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "mail delivery was updated by another operator", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "MAIL_REPLAY_FAILED", err.Error(), nil)
		return
	}
	writeSuccess(ctx, deadLetter)
}

func (h Handler) CreateMailSuppression(ctx *gin.Context) {
	var input struct {
		Recipient  string  `json:"recipient"`
		ReasonCode string  `json:"reason_code"`
		ExpiresAt  *string `json:"expires_at"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "invalid suppression request", nil)
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(input.Recipient))
	if err != nil || strings.TrimSpace(input.ReasonCode) == "" {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "recipient and reason_code are required", nil)
		return
	}
	var expiresAt *time.Time
	if input.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, *input.ExpiresAt)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, "INVALID_EXPIRES_AT", "expires_at must be RFC3339", nil)
			return
		}
		parsed = parsed.UTC()
		expiresAt = &parsed
	}
	sum := sha256.Sum256([]byte(strings.ToLower(address.Address)))
	suppression := model.MailSuppression{RecipientHash: hex.EncodeToString(sum[:]), ReasonCode: strings.TrimSpace(input.ReasonCode), ExpiresAt: expiresAt, Version: 1}
	if err := h.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "recipient_hash"}}, DoUpdates: clause.Assignments(map[string]any{"reason_code": suppression.ReasonCode, "expires_at": expiresAt, "updated_at": time.Now().UTC(), "version": gorm.Expr("mail_suppressions.version + 1")})}).Create(&suppression).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "SUPPRESSION_SAVE_FAILED", "suppression could not be saved", nil)
		return
	}
	h.db.Where("recipient_hash = ?", suppression.RecipientHash).First(&suppression)
	_ = audit.Record(ctx, h.db, "mail_suppression.saved", "mail_suppression", suppression.ID, map[string]any{"recipient_hash_prefix": suppression.RecipientHash[:12], "reason_code": suppression.ReasonCode, "request_id": ensureRequestID(ctx)})
	writeSuccess(ctx, suppression)
}

func (h Handler) ReleaseMailSuppression(ctx *gin.Context) {
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version is required", nil)
		return
	}
	var suppression model.MailSuppression
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&suppression, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if suppression.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		now := time.Now().UTC()
		if err := tx.Model(&suppression).Updates(map[string]any{"expires_at": now, "version": suppression.Version + 1}).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "mail_suppression.released", "mail_suppression", suppression.ID, map[string]any{"recipient_hash_prefix": suppression.RecipientHash[:12], "request_id": ensureRequestID(ctx)})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "suppression was updated by another operator", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "SUPPRESSION_RELEASE_FAILED", err.Error(), nil)
		return
	}
	h.db.First(&suppression, "id = ?", suppression.ID)
	writeSuccess(ctx, suppression)
}

func (h Handler) ListPlatformFeedback(ctx *gin.Context) {
	query := h.db.Model(&model.PlatformFeedback{})
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if urgency := strings.TrimSpace(ctx.Query("urgency")); urgency != "" {
		query = query.Where("urgency = ?", urgency)
	}
	var total int64
	query.Count(&total)
	var items []model.PlatformFeedback
	if err := query.Order("due_at asc").Limit(listLimit(ctx)).Find(&items).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "反馈列表读取失败", nil)
		return
	}
	writeSuccess(ctx, gin.H{"items": items, "total": total, "as_of": time.Now().UTC().Format(time.RFC3339)})
}

func (h Handler) ListFeedbackOperations(ctx *gin.Context) {
	var platform []model.PlatformFeedback
	if err := h.db.Order("due_at asc").Limit(listLimit(ctx)).Find(&platform).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "platform feedback could not be read", nil)
		return
	}
	var quiz []model.Report
	if err := h.db.Where("target_type = ?", "quiz_question").Order("created_at desc").Limit(listLimit(ctx)).Find(&quiz).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "quiz feedback could not be read", nil)
		return
	}
	writeSuccess(ctx, gin.H{"platform": platform, "quiz": quiz, "as_of": time.Now().UTC().Format(time.RFC3339)})
}

func (h Handler) VerifyQuizFeedback(ctx *gin.Context) {
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version is required", nil)
		return
	}
	var report model.Report
	if err := h.db.First(&report, "id = ? AND target_type = ?", ctx.Param("id"), "quiz_question").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "QUIZ_FEEDBACK_NOT_FOUND", "quiz feedback was not found", nil)
		return
	}
	jsonVerified := json.Valid(report.TargetSnapshot) && len(report.TargetSnapshot) > 2
	var question model.QuizQuestion
	postgresVerified := h.db.First(&question, "id = ?", report.TargetID).Error == nil
	apiVerified := false
	requestContext, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, h.apiBaseURL+"/questions/"+report.TargetID, nil)
	if err == nil {
		if response, requestErr := http.DefaultClient.Do(request); requestErr == nil {
			apiVerified = response.StatusCode == http.StatusOK
			_ = response.Body.Close()
		}
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&report, "id = ?", report.ID).Error; err != nil {
			return err
		}
		if report.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		if err := tx.Model(&report).Updates(map[string]any{"json_verified": jsonVerified, "postgres_verified": postgresVerified, "api_verified": apiVerified, "version": report.Version + 1}).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "quiz_feedback.verified", "report", report.ID, map[string]any{"json_verified": jsonVerified, "postgres_verified": postgresVerified, "api_verified": apiVerified, "request_id": ensureRequestID(ctx)})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "quiz feedback was updated by another operator", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUIZ_VERIFICATION_FAILED", "quiz feedback verification failed", nil)
		return
	}
	h.db.First(&report, "id = ?", report.ID)
	writeSuccess(ctx, report)
}

func (h Handler) ResolveQuizFeedback(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "not authenticated", nil)
		return
	}
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version is required", nil)
		return
	}
	var report model.Report
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&report, "id = ? AND target_type = ?", ctx.Param("id"), "quiz_question").Error; err != nil {
			return err
		}
		if report.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		if !report.JSONVerified || !report.PostgresVerified || !report.APIVerified {
			return errors.New("three-party verification is incomplete")
		}
		if report.Status != model.StatusPending {
			return errors.New("quiz feedback is not pending")
		}
		now := time.Now().UTC()
		if err := tx.Model(&report).Updates(map[string]any{"status": model.StatusApproved, "reviewer_id": user.ID, "reviewed_at": now, "version": report.Version + 1}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.OperationCase{}).Where("source_service = ? AND source_type = ? AND source_id = ?", "quizcraft", "quiz_feedback", report.ID).Updates(map[string]any{"status": "resolved", "resolved_at": now, "version": gorm.Expr("version + 1")}).Error; err != nil {
			return err
		}
		if err := createOutboxEvent(tx, "quiz_feedback", report.ID, "quiz.feedback_resolved.v1", map[string]any{"report_id": report.ID, "question_id": report.TargetID}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, "quiz_feedback.resolved", "report", report.ID, map[string]any{"request_id": ensureRequestID(ctx)})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "quiz feedback was updated by another operator", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusConflict, "QUIZ_FEEDBACK_RESOLUTION_BLOCKED", err.Error(), nil)
		return
	}
	h.db.First(&report, "id = ?", report.ID)
	writeSuccess(ctx, report)
}

func (h Handler) UpdatePlatformFeedback(ctx *gin.Context) {
	var input struct {
		Status          string  `json:"status"`
		AssigneeID      *string `json:"assignee_id"`
		DueAt           *string `json:"due_at"`
		ExpectedVersion int     `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version 必填", nil)
		return
	}
	if input.Status != "new" && input.Status != "in_progress" && input.Status != "resolved" {
		writeError(ctx, http.StatusBadRequest, "INVALID_STATUS", "反馈状态不合法", nil)
		return
	}
	var parsedDueAt *time.Time
	if input.DueAt != nil {
		value, err := time.Parse(time.RFC3339, *input.DueAt)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, "INVALID_DUE_AT", "due_at 必须使用 RFC3339 UTC 时间", nil)
			return
		}
		value = value.UTC()
		parsedDueAt = &value
	}
	var feedback model.PlatformFeedback
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&feedback, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if feedback.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		before := map[string]any{"status": feedback.Status, "assignee_id": feedback.AssigneeID, "due_at": feedback.DueAt, "version": feedback.Version}
		updates := map[string]any{"status": input.Status, "assignee_id": input.AssigneeID, "version": feedback.Version + 1}
		if parsedDueAt != nil {
			updates["due_at"] = *parsedDueAt
		}
		if input.Status == "resolved" {
			now := time.Now().UTC()
			updates["resolved_at"] = now
		} else {
			updates["resolved_at"] = nil
		}
		if err := tx.Model(&feedback).Updates(updates).Error; err != nil {
			return err
		}
		caseUpdates := map[string]any{"status": "open", "assignee_id": input.AssigneeID, "version": gorm.Expr("version + 1")}
		if parsedDueAt != nil {
			caseUpdates["due_at"] = *parsedDueAt
		}
		if input.Status == "resolved" {
			now := time.Now().UTC()
			caseUpdates["status"] = "resolved"
			caseUpdates["resolved_at"] = now
		} else {
			caseUpdates["resolved_at"] = nil
		}
		if err := tx.Model(&model.OperationCase{}).Where("source_service = ? AND source_type = ? AND source_id = ?", "feedback", "platform_feedback", feedback.ID).Updates(caseUpdates).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "platform_feedback.updated", "platform_feedback", feedback.ID, map[string]any{
			"before":     before,
			"after":      map[string]any{"status": input.Status, "assignee_id": input.AssigneeID, "due_at": parsedDueAt, "version": feedback.Version + 1},
			"request_id": ensureRequestID(ctx),
		})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "反馈已被其他管理员更新", nil)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "FEEDBACK_NOT_FOUND", "反馈不存在", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "FEEDBACK_UPDATE_FAILED", "反馈更新失败", nil)
		return
	}
	h.db.First(&feedback, "id = ?", feedback.ID)
	writeSuccess(ctx, feedback)
}

type foodCandidate struct {
	EntryID       string  `json:"entry_id"`
	EntryVersion  int     `json:"entry_version"`
	RoundID       string  `json:"round_id"`
	Name          string  `json:"name"`
	CurrentTierID string  `json:"current_tier_id"`
	Participants  int64   `json:"participants"`
	Underrated    float64 `json:"underrated_rate"`
	AboutRight    float64 `json:"about_right_rate"`
	Overrated     float64 `json:"overrated_rate"`
	Decision      string  `json:"decision"`
}

func (h Handler) ListFoodOperations(ctx *gin.Context) {
	var tiers []model.FoodTierDefinition
	var submissions []model.FoodSubmission
	var entries []model.FoodEntry
	var anomalies []model.FoodVoteAnomaly
	var history []model.FoodTierAdjustment
	h.db.Where("enabled = ?", true).Order("sort_order asc").Find(&tiers)
	h.db.Order("created_at desc").Limit(listLimit(ctx)).Find(&submissions)
	h.db.Order("created_at desc").Limit(listLimit(ctx)).Find(&entries)
	h.db.Order("created_at desc").Limit(listLimit(ctx)).Find(&anomalies)
	h.db.Order("adjusted_at desc").Limit(listLimit(ctx)).Find(&history)
	candidates := make([]foodCandidate, 0, len(entries))
	for _, entry := range entries {
		var round model.FoodCalibrationRound
		if err := h.db.Where("entry_id = ? AND status = ?", entry.ID, "open").Order("round_number desc").First(&round).Error; err != nil {
			continue
		}
		candidate := h.foodCandidate(entry, round)
		candidates = append(candidates, candidate)
	}
	writeSuccess(ctx, gin.H{"tiers": tiers, "submissions": submissions, "entries": entries, "candidates": candidates, "anomalies": anomalies, "history": history})
}

func (h Handler) foodCandidate(entry model.FoodEntry, round model.FoodCalibrationRound) foodCandidate {
	return foodCandidateWithDB(h.db, entry, round)
}

func foodCandidateWithDB(db *gorm.DB, entry model.FoodEntry, round model.FoodCalibrationRound) foodCandidate {
	result := foodCandidate{EntryID: entry.ID, EntryVersion: entry.Version, RoundID: round.ID, Name: entry.Name, CurrentTierID: entry.CurrentTierID, Decision: "insufficient_votes"}
	var rows []struct {
		Position string
		Count    int64
	}
	db.Model(&model.FoodCalibrationVote{}).Select("position, count(*) as count").Where("round_id = ? AND status IN ?", round.ID, []string{"valid", "restored"}).Group("position").Scan(&rows)
	for _, row := range rows {
		result.Participants += row.Count
	}
	if result.Participants > 0 {
		for _, row := range rows {
			rate := float64(row.Count) / float64(result.Participants)
			switch row.Position {
			case "underrated":
				result.Underrated = rate
			case "about_right":
				result.AboutRight = rate
			case "overrated":
				result.Overrated = rate
			}
		}
	}
	var blocking int64
	db.Model(&model.FoodVoteAnomaly{}).Where("round_id = ? AND blocking = ? AND status = ?", round.ID, true, "open").Count(&blocking)
	if blocking > 0 {
		result.Decision = "blocked_by_risk"
		return result
	}
	if entry.LastAdjustedAt != nil && time.Since(*entry.LastAdjustedAt) < 7*24*time.Hour {
		result.Decision = "cooldown"
		return result
	}
	if result.Participants < 10 {
		return result
	}
	switch {
	case result.Underrated >= .7:
		result.Decision = "promote_candidate"
	case result.Overrated >= .7:
		result.Decision = "demote_candidate"
	case result.AboutRight >= .7:
		result.Decision = "stable"
	default:
		result.Decision = "contested"
	}
	return result
}

func (h Handler) AdjustFoodEntry(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "not authenticated", nil)
		return
	}
	var input struct {
		Direction       string `json:"direction"`
		ExpectedVersion int    `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 || (input.Direction != "promote" && input.Direction != "demote") {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "direction 和 expected_version 必填", nil)
		return
	}
	var entry model.FoodEntry
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&entry, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if entry.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		var round model.FoodCalibrationRound
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("entry_id = ? AND status = ?", entry.ID, "open").Order("round_number desc").First(&round).Error; err != nil {
			return err
		}
		candidate := foodCandidateWithDB(tx, entry, round)
		expectedDecision := "promote_candidate"
		if input.Direction == "demote" {
			expectedDecision = "demote_candidate"
		}
		if candidate.Decision != expectedDecision {
			return errFoodPolicyNotEligible
		}
		var current model.FoodTierDefinition
		if err := tx.First(&current, "id = ? AND enabled = ?", entry.CurrentTierID, true).Error; err != nil {
			return err
		}
		var target model.FoodTierDefinition
		tierQuery := tx.Where("enabled = ?", true)
		if input.Direction == "promote" {
			tierQuery = tierQuery.Where("sort_order < ?", current.SortOrder).Order("sort_order desc")
		} else {
			tierQuery = tierQuery.Where("sort_order > ?", current.SortOrder).Order("sort_order asc")
		}
		if err := tierQuery.First(&target).Error; err != nil {
			return errFoodTierBoundary
		}
		now := time.Now().UTC()
		if err := tx.Model(&entry).Updates(map[string]any{"current_tier_id": target.ID, "last_adjusted_at": now, "version": entry.Version + 1}).Error; err != nil {
			return err
		}
		if err := tx.Model(&round).Updates(map[string]any{"status": "closed", "closed_at": now}).Error; err != nil {
			return err
		}
		newRound := model.FoodCalibrationRound{EntryID: entry.ID, RoundNumber: round.RoundNumber + 1, Status: "open", PolicyVersion: "food_calibration_v1", OpenedAt: now}
		if err := tx.Create(&newRound).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.FoodTierAdjustment{EntryID: entry.ID, RoundID: round.ID, FromTierID: current.ID, ToTierID: target.ID, Direction: input.Direction, ActorID: user.ID, AdjustedAt: now}).Error; err != nil {
			return err
		}
		if err := createOutboxEvent(tx, "food_entry", entry.ID, "food.tier_adjusted.v1", map[string]any{"entry_id": entry.ID, "from_tier_id": current.ID, "to_tier_id": target.ID, "direction": input.Direction, "new_round_id": newRound.ID}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, "food_entry.adjusted", "food_entry", entry.ID, map[string]any{
			"before":     map[string]any{"tier_id": current.ID, "version": entry.Version, "round_id": round.ID},
			"after":      map[string]any{"tier_id": target.ID, "version": entry.Version + 1, "round_id": newRound.ID, "direction": input.Direction},
			"request_id": ensureRequestID(ctx),
		})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "榜单条目已被其他管理员更新", nil)
		return
	}
	if errors.Is(err, errFoodPolicyNotEligible) {
		writeError(ctx, http.StatusConflict, "FOOD_POLICY_NOT_ELIGIBLE", "当前轮次不满足该调档方向", nil)
		return
	}
	if errors.Is(err, errFoodTierBoundary) {
		writeError(ctx, http.StatusConflict, "FOOD_TIER_BOUNDARY", "当前已经位于该方向的边界档位", nil)
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "FOOD_ENTRY_NOT_FOUND", "榜单条目或开放轮次不存在", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "FOOD_ADJUSTMENT_FAILED", "调档失败", nil)
		return
	}
	h.db.First(&entry, "id = ?", entry.ID)
	writeSuccess(ctx, entry)
}

func (h Handler) ResolveFoodAnomaly(ctx *gin.Context) {
	var input struct {
		ExpectedVersion int    `json:"expected_version"`
		Resolution      string `json:"resolution"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 || strings.TrimSpace(input.Resolution) == "" {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version and resolution are required", nil)
		return
	}
	var anomaly model.FoodVoteAnomaly
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&anomaly, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if anomaly.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		if anomaly.Status != "open" {
			return errors.New("food anomaly is not open")
		}
		if err := tx.Model(&anomaly).Updates(map[string]any{"status": "resolved", "version": anomaly.Version + 1}).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "food_vote_anomaly.resolved", "food_vote_anomaly", anomaly.ID, map[string]any{"resolution": strings.TrimSpace(input.Resolution), "request_id": ensureRequestID(ctx)})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "food anomaly was updated by another operator", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "FOOD_ANOMALY_RESOLUTION_FAILED", err.Error(), nil)
		return
	}
	h.db.First(&anomaly, "id = ?", anomaly.ID)
	writeSuccess(ctx, anomaly)
}

var (
	errFoodPolicyNotEligible = errors.New("food policy not eligible")
	errFoodTierBoundary      = errors.New("food tier boundary")
)

func (h Handler) ListSystemOperations(ctx *gin.Context) {
	var heartbeats []model.ServiceHeartbeat
	if err := h.db.Order("service_id asc").Find(&heartbeats).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "服务状态读取失败", nil)
		return
	}
	now := time.Now().UTC()
	probeContext, cancel := context.WithTimeout(ctx.Request.Context(), 1500*time.Millisecond)
	defer cancel()
	postgresStatus := "unavailable"
	if sqlDB, err := h.db.DB(); err == nil && sqlDB.PingContext(probeContext) == nil {
		postgresStatus = "ok"
	}
	redisStatus := "unavailable"
	if h.cache != nil && h.cache.Ping(probeContext).Err() == nil {
		redisStatus = "ok"
	}
	apiStatus := "ok"
	if postgresStatus != "ok" || redisStatus != "ok" {
		apiStatus = "partial"
	}
	api := model.ServiceHeartbeat{BaseModel: model.BaseModel{ID: "runtime-platform-core-api", CreatedAt: now, UpdatedAt: now}, ServiceID: "platform-core-api", Status: apiStatus, Version: h.version, CommitSHA: h.commit, LastReadyAt: now}
	heartbeats = append([]model.ServiceHeartbeat{api}, heartbeats...)
	var queuedMail int64
	var deadLetters int64
	var outboxPending int64
	var outboxFailed int64
	h.db.Model(&model.MailDelivery{}).Where("status IN ?", []string{"queued", "retry", "sending"}).Count(&queuedMail)
	h.db.Model(&model.MailDeadLetter{}).Where("status = ?", "open").Count(&deadLetters)
	h.db.Model(&model.OutboxEvent{}).Where("status IN ?", []string{"pending", "processing"}).Count(&outboxPending)
	h.db.Model(&model.OutboxEvent{}).Where("status = ?", "failed").Count(&outboxFailed)
	latestMigration := "unknown"
	if h.db.Migrator().HasTable("schema_migrations") {
		var row struct{ Version string }
		if h.db.Table("schema_migrations").Select("version").Order("version desc").Limit(1).Scan(&row).Error == nil && row.Version != "" {
			latestMigration = row.Version
		}
	}
	httpMetrics := middleware.HTTPSnapshot{}
	if h.telemetry != nil {
		httpMetrics = h.telemetry.Snapshot()
	}
	writeSuccess(ctx, gin.H{
		"items": heartbeats, "as_of": now.Format(time.RFC3339),
		"runtime": gin.H{"postgresql": postgresStatus, "redis": redisStatus, "mail_pending": queuedMail, "mail_dlq": deadLetters, "outbox_pending": outboxPending, "outbox_failed": outboxFailed, "latest_migration": latestMigration, "http": httpMetrics},
	})
}

func listLimit(ctx *gin.Context) int {
	value, err := strconv.Atoi(ctx.DefaultQuery("limit", "100"))
	if err != nil || value < 1 {
		return 100
	}
	if value > 200 {
		return 200
	}
	return value
}
