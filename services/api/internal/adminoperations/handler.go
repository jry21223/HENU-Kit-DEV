package adminoperations

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/objectstorage"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/middleware"
)

const (
	maxImportBytes = 10 << 20
	maxImportRows  = 1000
)

type Handler struct {
	db         *gorm.DB
	storage    *objectstorage.Signer
	version    string
	commit     string
	apiBaseURL string
	cache      *redislib.Client
	telemetry  *middleware.HTTPRegistry
}

func NewHandler(db *gorm.DB, storage *objectstorage.Signer, version string, commit string, apiBaseURL string, cache *redislib.Client, telemetry *middleware.HTTPRegistry) Handler {
	return Handler{db: db, storage: storage, version: version, commit: commit, apiBaseURL: strings.TrimRight(apiBaseURL, "/"), cache: cache, telemetry: telemetry}
}

type captureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (writer *captureWriter) Write(data []byte) (int, error) {
	writer.body.Write(data)
	return writer.ResponseWriter.Write(data)
}

// Idempotent persists and replays successful responses for mutating endpoints.
// Authentication middleware must run before this middleware.
func (h Handler) Idempotent() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		key := strings.TrimSpace(ctx.GetHeader("Idempotency-Key"))
		if len(key) < 8 {
			writeError(ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key 至少 8 个字符", nil)
			ctx.Abort()
			return
		}
		user, ok := auth.CurrentUser(ctx)
		if !ok {
			writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "未登录", nil)
			ctx.Abort()
			return
		}
		body, err := io.ReadAll(ctx.Request.Body)
		if err != nil {
			writeError(ctx, http.StatusBadRequest, "REQUEST_READ_FAILED", "请求体读取失败", nil)
			ctx.Abort()
			return
		}
		ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(body)
		requestHash := hex.EncodeToString(sum[:])
		route := ctx.FullPath()
		if route == "" {
			route = ctx.Request.URL.Path
		}

		var existing model.IdempotencyRecord
		query := h.db.Where("actor_id = ? AND method = ? AND route = ? AND key = ?", user.ID, ctx.Request.Method, route, key)
		if err := query.First(&existing).Error; err == nil {
			if existing.RequestHash != requestHash {
				writeError(ctx, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "同一 Idempotency-Key 不能用于不同请求", nil)
			} else if existing.State == "completed" {
				ctx.Header("X-Idempotency-Replayed", "true")
				ctx.Data(existing.StatusCode, "application/json; charset=utf-8", []byte(existing.ResponseBody))
			} else {
				writeError(ctx, http.StatusConflict, "IDEMPOTENCY_REQUEST_IN_PROGRESS", "相同请求仍在处理中", nil)
			}
			ctx.Abort()
			return
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(ctx, http.StatusInternalServerError, "IDEMPOTENCY_LOOKUP_FAILED", "幂等状态读取失败", nil)
			ctx.Abort()
			return
		}

		record := model.IdempotencyRecord{ActorID: user.ID, Method: ctx.Request.Method, Route: route, Key: key, RequestHash: requestHash, State: "processing"}
		if err := h.db.Create(&record).Error; err != nil {
			writeError(ctx, http.StatusConflict, "IDEMPOTENCY_REQUEST_IN_PROGRESS", "相同请求已被接收", nil)
			ctx.Abort()
			return
		}
		writer := &captureWriter{ResponseWriter: ctx.Writer}
		ctx.Writer = writer
		ctx.Next()
		if ctx.Writer.Status() >= http.StatusInternalServerError {
			h.db.Delete(&record)
			return
		}
		h.db.Model(&record).Updates(map[string]any{"state": "completed", "status_code": ctx.Writer.Status(), "response_body": writer.body.String()})
	}
}

type noticeInput struct {
	SchemaVersion  string             `json:"schema_version"`
	ExternalID     string             `json:"external_id"`
	SourceID       string             `json:"source_id"`
	OrganizationID *string            `json:"organization_id"`
	Title          string             `json:"title"`
	Body           string             `json:"body"`
	PublishedAt    time.Time          `json:"published_at"`
	OriginalURL    string             `json:"original_url"`
	Importance     string             `json:"importance"`
	Audience       []string           `json:"audience"`
	ContentSHA256  string             `json:"content_sha256"`
	Attachments    []noticeAttachment `json:"attachments"`
}

type noticeAttachment struct {
	ObjectKey   string `json:"object_key"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type importRowResult struct {
	Row        int    `json:"row"`
	ExternalID string `json:"external_id,omitempty"`
	Result     string `json:"result"`
	Error      string `json:"error,omitempty"`
}

func createOutboxEvent(tx *gorm.DB, aggregateType, aggregateID, eventType string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&model.OutboxEvent{AggregateType: aggregateType, AggregateID: aggregateID, EventType: eventType, Payload: body, Status: "pending", AvailableAt: time.Now().UTC()}).Error
}

// EnqueueMail accepts only service-authenticated requests. Its idempotency
// scope is the calling service plus Idempotency-Key and survives restarts.
func (h Handler) EnqueueMail(ctx *gin.Context) {
	serviceID, _ := ctx.Get("service_id")
	idempotencyKey := strings.TrimSpace(ctx.GetHeader("Idempotency-Key"))
	if len(idempotencyKey) < 8 {
		writeError(ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain at least 8 characters", nil)
		return
	}
	var input struct {
		Category     string `json:"category"`
		Recipient    string `json:"recipient"`
		TemplateCode string `json:"template_code"`
		Subject      string `json:"subject"`
		Body         string `json:"body"`
		RequestID    string `json:"request_id"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "invalid mail delivery request", nil)
		return
	}
	if input.Category != "critical" && input.Category != "transactional" && input.Category != "digest" {
		writeError(ctx, http.StatusBadRequest, "INVALID_MAIL_CATEGORY", "category must be critical, transactional, or digest", nil)
		return
	}
	address, err := mail.ParseAddress(strings.TrimSpace(input.Recipient))
	if err != nil || address.Address == "" || strings.TrimSpace(input.TemplateCode) == "" || strings.TrimSpace(input.Subject) == "" || strings.TrimSpace(input.Body) == "" || strings.TrimSpace(input.RequestID) == "" {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "recipient, template_code, subject, body, and request_id are required", nil)
		return
	}
	canonicalBody, _ := json.Marshal(input)
	requestSum := sha256.Sum256(canonicalBody)
	enqueueSum := sha256.Sum256([]byte(fmt.Sprint(serviceID) + "\n" + idempotencyKey))
	enqueueKey := hex.EncodeToString(enqueueSum[:])
	requestHash := hex.EncodeToString(requestSum[:])
	var existing model.MailDelivery
	if err := h.db.Where("enqueue_key = ?", enqueueKey).First(&existing).Error; err == nil {
		if existing.RequestHash != requestHash {
			writeError(ctx, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "the idempotency key was already used for a different mail", nil)
			return
		}
		ctx.Header("X-Idempotency-Replayed", "true")
		writeSuccess(ctx, existing)
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, "MAIL_LOOKUP_FAILED", "mail idempotency lookup failed", nil)
		return
	}
	recipient := strings.ToLower(address.Address)
	recipientSum := sha256.Sum256([]byte(recipient))
	delivery := model.MailDelivery{
		EnqueueKey: enqueueKey, RequestHash: requestHash, Category: input.Category, Status: "queued",
		RecipientHash: hex.EncodeToString(recipientSum[:]), Recipient: recipient, TemplateCode: input.TemplateCode,
		Subject: input.Subject, Body: input.Body, RequestID: input.RequestID, QueuedAt: time.Now().UTC(), Version: 1,
	}
	if err := h.db.Create(&delivery).Error; err != nil {
		writeError(ctx, http.StatusConflict, "MAIL_ENQUEUE_CONFLICT", "mail delivery could not be enqueued", nil)
		return
	}
	writeSuccess(ctx, delivery)
}

func (h Handler) CreateUploadIntent(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "未登录", nil)
		return
	}
	if h.storage == nil {
		writeError(ctx, http.StatusServiceUnavailable, "OBJECT_STORAGE_UNAVAILABLE", "对象存储尚未配置", nil)
		return
	}
	var input struct {
		Scope       string `json:"scope"`
		FileName    string `json:"file_name"`
		ContentType string `json:"content_type"`
		SizeBytes   int64  `json:"size_bytes"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "上传参数不合法", nil)
		return
	}
	if input.Scope != "food_image" && input.Scope != "notice_attachment" {
		writeError(ctx, http.StatusBadRequest, "INVALID_UPLOAD_SCOPE", "上传用途不合法", nil)
		return
	}
	if input.Scope == "notice_attachment" && user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin {
		writeError(ctx, http.StatusForbidden, "FORBIDDEN", "只有管理员可以上传通知附件", nil)
		return
	}
	extension, validType := uploadExtension(input.Scope, input.ContentType)
	if !validType || input.SizeBytes < 1 || input.SizeBytes > 20<<20 || strings.TrimSpace(input.FileName) == "" {
		writeError(ctx, http.StatusBadRequest, "INVALID_UPLOAD", "文件类型或大小不合法", nil)
		return
	}
	if filepath.Ext(input.FileName) == "" {
		writeError(ctx, http.StatusBadRequest, "INVALID_FILE_NAME", "文件名必须包含扩展名", nil)
		return
	}
	now := time.Now().UTC()
	objectKey := fmt.Sprintf("%s/%s/%s%s", input.Scope, now.Format("2006/01"), uuid.NewString(), extension)
	uploadURL, err := h.storage.PresignPut(objectKey, 10*time.Minute, now)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "PRESIGN_FAILED", "上传地址创建失败", nil)
		return
	}
	writeSuccess(ctx, gin.H{
		"object_key": objectKey,
		"upload_url": uploadURL,
		"headers":    gin.H{"content-type": input.ContentType},
		"expires_at": now.Add(10 * time.Minute).Format(time.RFC3339),
	})
}

func uploadExtension(scope, contentType string) (string, bool) {
	allowed := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	if scope == "notice_attachment" {
		allowed["application/pdf"] = ".pdf"
	}
	extension, ok := allowed[contentType]
	return extension, ok
}

func (h Handler) CreateNotice(ctx *gin.Context) {
	var input noticeInput
	if err := ctx.ShouldBindJSON(&input); err != nil {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "通知格式不合法", err.Error())
		return
	}
	result, notice, err := h.upsertNotice(input)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "NOTICE_IMPORT_FAILED", "通知写入失败", err.Error())
		return
	}
	writeSuccess(ctx, gin.H{"result": result, "notice": notice})
}

func (h Handler) ImportNotices(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "未登录", nil)
		return
	}

	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxImportBytes)
	job := model.NoticeImportJob{Status: "processing", RequestedBy: user.ID}
	if err := h.db.Create(&job).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "IMPORT_JOB_CREATE_FAILED", "无法创建导入任务", nil)
		return
	}

	scanner := bufio.NewScanner(ctx.Request.Body)
	scanner.Buffer(make([]byte, 64*1024), maxImportBytes)
	results := make([]importRowResult, 0)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		job.TotalRows++
		if job.TotalRows > maxImportRows {
			job.FailedRows++
			results = append(results, importRowResult{Row: job.TotalRows, Result: "failed", Error: "导入任务最多允许 1000 条"})
			break
		}
		var input noticeInput
		if err := json.Unmarshal(scanner.Bytes(), &input); err != nil {
			job.FailedRows++
			results = append(results, importRowResult{Row: job.TotalRows, Result: "failed", Error: err.Error()})
			continue
		}
		if input.ContentSHA256 == "" {
			job.FailedRows++
			results = append(results, importRowResult{Row: job.TotalRows, ExternalID: input.ExternalID, Result: "failed", Error: "content_sha256 is required for JSONL import"})
			continue
		}
		result, _, err := h.upsertNotice(input)
		if err != nil {
			job.FailedRows++
			results = append(results, importRowResult{Row: job.TotalRows, ExternalID: input.ExternalID, Result: "failed", Error: err.Error()})
			continue
		}
		switch result {
		case "created":
			job.CreatedRows++
		case "updated":
			job.UpdatedRows++
		case "duplicate":
			job.DuplicateRows++
		}
		results = append(results, importRowResult{Row: job.TotalRows, ExternalID: input.ExternalID, Result: result})
	}
	if err := scanner.Err(); err != nil {
		job.Status = "failed"
		job.ErrorSummary = err.Error()
	} else if job.FailedRows > 0 {
		job.Status = "partial"
	} else {
		job.Status = "completed"
	}
	h.db.Save(&job)
	writeSuccess(ctx, gin.H{"job": job, "rows": results})
}

func (h Handler) CreatePlatformFeedback(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "未登录", nil)
		return
	}
	var input struct {
		Category string `json:"category"`
		Summary  string `json:"summary"`
		Content  string `json:"content"`
		Urgency  string `json:"urgency"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Summary) == "" || strings.TrimSpace(input.Content) == "" {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "反馈内容不完整", nil)
		return
	}
	if input.Urgency == "" {
		input.Urgency = model.UrgencyNormal
	}
	if input.Urgency != model.UrgencyUrgent && input.Urgency != model.UrgencyNormal {
		writeError(ctx, http.StatusBadRequest, "INVALID_URGENCY", "urgency 只能是 urgent 或 normal", nil)
		return
	}
	now := time.Now().UTC()
	dueAt := now.Add(72 * time.Hour)
	if input.Urgency == model.UrgencyUrgent {
		dueAt = now.Add(24 * time.Hour)
	}
	requestID := ensureRequestID(ctx)
	feedback := model.PlatformFeedback{UserID: &user.ID, Category: input.Category, Summary: input.Summary, Content: input.Content, Urgency: input.Urgency, Status: "new", DueAt: dueAt, RequestID: requestID, Version: 1}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&feedback).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.OperationCase{SourceService: "feedback", SourceType: "platform_feedback", SourceID: feedback.ID, Summary: feedback.Summary, Urgency: feedback.Urgency, Status: "open", DueAt: feedback.DueAt, ActionPath: "/feedback?source_type=platform_feedback&status=open", Version: 1}).Error; err != nil {
			return err
		}
		return createOutboxEvent(tx, "platform_feedback", feedback.ID, "feedback.created.v1", map[string]any{"feedback_id": feedback.ID, "urgency": feedback.Urgency, "due_at": feedback.DueAt})
	})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "FEEDBACK_CREATE_FAILED", "反馈创建失败", nil)
		return
	}
	writeSuccess(ctx, feedback)
}

func (h Handler) MyNoticeEmailSubscription(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "not authenticated", nil)
		return
	}
	var subscription model.NoticeEmailSubscription
	if err := h.db.Where("user_id = ?", user.ID).First(&subscription).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		writeSuccess(ctx, gin.H{"user_id": user.ID, "enabled": false, "version": 0})
		return
	} else if err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "subscription could not be read", nil)
		return
	}
	writeSuccess(ctx, subscription)
}

func (h Handler) UpdateMyNoticeEmailSubscription(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "not authenticated", nil)
		return
	}
	var input struct {
		Enabled         bool `json:"enabled"`
		ExpectedVersion int  `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 0 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "enabled and expected_version are required", nil)
		return
	}
	var subscription model.NoticeEmailSubscription
	err := h.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", user.ID).First(&subscription).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if input.ExpectedVersion != 0 {
				return errVersionConflict
			}
			subscription = model.NoticeEmailSubscription{UserID: user.ID, Enabled: input.Enabled, Version: 1}
			return tx.Create(&subscription).Error
		}
		if err != nil {
			return err
		}
		if subscription.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		return tx.Model(&subscription).Updates(map[string]any{"enabled": input.Enabled, "version": subscription.Version + 1}).Error
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "subscription was updated by another request", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "SUBSCRIPTION_SAVE_FAILED", "subscription could not be saved", nil)
		return
	}
	h.db.Where("user_id = ?", user.ID).First(&subscription)
	writeSuccess(ctx, subscription)
}

func (h Handler) FoodTiers(ctx *gin.Context) {
	var tiers []model.FoodTierDefinition
	if err := h.db.Where("enabled = ?", true).Order("sort_order asc").Find(&tiers).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "QUERY_FAILED", "档位读取失败", nil)
		return
	}
	writeSuccess(ctx, gin.H{"items": tiers})
}

func (h Handler) CreateFoodSubmission(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "未登录", nil)
		return
	}
	var input struct {
		Name            string `json:"name"`
		Location        string `json:"location"`
		SuggestedTierID string `json:"suggested_tier_id"`
		Reason          string `json:"reason"`
		ImageObjectKey  string `json:"image_object_key"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.Name == "" || input.Location == "" || input.Reason == "" || !strings.HasPrefix(input.ImageObjectKey, "food_image/") {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "投稿字段不完整", nil)
		return
	}
	var tier model.FoodTierDefinition
	if err := h.db.Where("id = ? AND enabled = ?", input.SuggestedTierID, true).First(&tier).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "INVALID_TIER", "建议档位不存在", nil)
		return
	}
	submission := model.FoodSubmission{SubmitterID: user.ID, Name: input.Name, Location: input.Location, SuggestedTierID: tier.ID, Reason: input.Reason, ImageObjectKey: input.ImageObjectKey, Status: model.StatusPending, Version: 1}
	if err := h.db.Create(&submission).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "SUBMISSION_CREATE_FAILED", "投稿创建失败", nil)
		return
	}
	writeSuccess(ctx, submission)
}

func (h Handler) ApproveFoodSubmission(ctx *gin.Context) {
	var input struct {
		ExpectedVersion int `json:"expected_version"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || input.ExpectedVersion < 1 {
		writeError(ctx, http.StatusBadRequest, "VALIDATION_ERROR", "expected_version 必填", nil)
		return
	}
	var entry model.FoodEntry
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var submission model.FoodSubmission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&submission, "id = ?", ctx.Param("id")).Error; err != nil {
			return err
		}
		if submission.Version != input.ExpectedVersion {
			return errVersionConflict
		}
		if submission.Status == model.StatusApproved {
			return tx.First(&entry, "submission_id = ?", submission.ID).Error
		}
		if submission.Status != model.StatusPending && submission.Status != model.StatusNeedsChanges {
			return fmt.Errorf("invalid status %s", submission.Status)
		}
		entry = model.FoodEntry{SubmissionID: submission.ID, Name: submission.Name, Location: submission.Location, InitialTierID: submission.SuggestedTierID, CurrentTierID: submission.SuggestedTierID, Version: 1}
		if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		round := model.FoodCalibrationRound{EntryID: entry.ID, RoundNumber: 1, Status: "open", PolicyVersion: "food_calibration_v1", OpenedAt: time.Now().UTC()}
		if err := tx.Create(&round).Error; err != nil {
			return err
		}
		if err := tx.Model(&submission).Updates(map[string]any{"status": model.StatusApproved, "version": submission.Version + 1}).Error; err != nil {
			return err
		}
		if err := createOutboxEvent(tx, "food_entry", entry.ID, "food.entry_approved.v1", map[string]any{"entry_id": entry.ID, "round_id": round.ID}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, "food_submission.approved", "food_submission", submission.ID, map[string]interface{}{
			"before":     map[string]interface{}{"status": submission.Status, "version": submission.Version},
			"after":      map[string]interface{}{"status": model.StatusApproved, "version": submission.Version + 1, "entry_id": entry.ID},
			"request_id": ensureRequestID(ctx),
		})
	})
	if errors.Is(err, errVersionConflict) {
		writeError(ctx, http.StatusConflict, "RESOURCE_VERSION_CONFLICT", "投稿已被其他人更新", nil)
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "APPROVAL_FAILED", "审核通过失败", err.Error())
		return
	}
	writeSuccess(ctx, entry)
}

func (h Handler) PutFoodVote(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "UNAUTHORIZED", "未登录", nil)
		return
	}
	var input struct {
		Position string `json:"position"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil || (input.Position != "underrated" && input.Position != "about_right" && input.Position != "overrated") {
		writeError(ctx, http.StatusBadRequest, "INVALID_POSITION", "只能判断被低估、差不多或被高估", nil)
		return
	}
	var round model.FoodCalibrationRound
	if err := h.db.Where("entry_id = ? AND status = ?", ctx.Param("id"), "open").Order("round_number desc").First(&round).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "ROUND_NOT_FOUND", "当前没有开放校准轮次", nil)
		return
	}
	vote := model.FoodCalibrationVote{RoundID: round.ID, UserID: user.ID, Position: input.Position, Status: "valid"}
	if err := h.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "round_id"}, {Name: "user_id"}}, DoUpdates: clause.Assignments(map[string]any{"position": input.Position, "status": "valid", "updated_at": time.Now().UTC()})}).Create(&vote).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "VOTE_SAVE_FAILED", "校准判断保存失败", nil)
		return
	}
	writeSuccess(ctx, vote)
}

func (h Handler) upsertNotice(input noticeInput) (string, model.CampusNotice, error) {
	if input.ContentSHA256 == "" {
		sum := sha256.Sum256([]byte(input.Body))
		input.ContentSHA256 = hex.EncodeToString(sum[:])
	}
	if err := validateNotice(input); err != nil {
		return "", model.CampusNotice{}, err
	}
	objects, _ := json.Marshal(input.Attachments)
	audience := input.Audience
	if len(audience) == 0 {
		audience = []string{"all_verified_users"}
	}
	audienceJSON, _ := json.Marshal(audience)
	var notice model.CampusNotice
	result := "created"
	err := h.db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("source_id = ? AND external_id = ?", input.SourceID, input.ExternalID).First(&notice).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			notice = model.CampusNotice{SourceID: input.SourceID, ExternalID: input.ExternalID, OrganizationID: input.OrganizationID, Title: input.Title, OriginalURL: input.OriginalURL, OriginalPublishedAt: &input.PublishedAt, CurrentVersion: 1, ContentHash: input.ContentSHA256, Status: "review_pending", DistributionStatus: "not_scheduled", Importance: defaultImportance(input.Importance), Audience: audienceJSON, Version: 1}
			if err := tx.Create(&notice).Error; err != nil {
				return err
			}
			return tx.Create(&model.CampusNoticeVersion{NoticeID: notice.ID, Version: 1, Title: input.Title, Body: input.Body, ContentHash: input.ContentSHA256, ObjectKeys: objects}).Error
		}
		if err != nil {
			return err
		}
		if notice.ContentHash == input.ContentSHA256 {
			result = "duplicate"
			return nil
		}
		result = "updated"
		nextVersion := notice.CurrentVersion + 1
		if err := tx.Create(&model.CampusNoticeVersion{NoticeID: notice.ID, Version: nextVersion, Title: input.Title, Body: input.Body, ContentHash: input.ContentSHA256, ObjectKeys: objects}).Error; err != nil {
			return err
		}
		return tx.Model(&notice).Updates(map[string]any{"title": input.Title, "original_url": input.OriginalURL, "original_published_at": input.PublishedAt, "organization_id": input.OrganizationID, "current_version": nextVersion, "content_hash": input.ContentSHA256, "status": "review_pending", "distribution_status": "not_scheduled", "importance": defaultImportance(input.Importance), "audience": audienceJSON, "version": notice.Version + 1}).Error
	})
	return result, notice, err
}

func validateNotice(input noticeInput) error {
	if input.SchemaVersion != "campus-notice-import/1.0" {
		return errors.New("unsupported schema_version")
	}
	if _, err := uuid.Parse(input.SourceID); err != nil {
		return errors.New("source_id must be UUID")
	}
	if input.OrganizationID != nil {
		if _, err := uuid.Parse(*input.OrganizationID); err != nil {
			return errors.New("organization_id must be UUID")
		}
	}
	if input.ExternalID == "" || input.Title == "" || input.Body == "" || input.OriginalURL == "" || input.PublishedAt.IsZero() {
		return errors.New("required notice field missing")
	}
	if utf8.RuneCountInString(input.ExternalID) > 240 || utf8.RuneCountInString(input.Title) > 300 || utf8.RuneCountInString(input.Body) > 200000 || utf8.RuneCountInString(input.OriginalURL) > 1000 {
		return errors.New("notice field exceeds maximum length")
	}
	parsedURL, err := url.ParseRequestURI(input.OriginalURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return errors.New("original_url must be an absolute HTTP URL")
	}
	importance := defaultImportance(input.Importance)
	if importance != "normal" && importance != "important" && importance != "urgent" {
		return errors.New("importance must be normal, important or urgent")
	}
	if len(input.Attachments) > 20 {
		return errors.New("attachments cannot exceed 20 items")
	}
	if len(input.Audience) > 20 {
		return errors.New("audience cannot exceed 20 selectors")
	}
	for _, selector := range input.Audience {
		if selector == "all_verified_users" {
			continue
		}
		parts := strings.SplitN(selector, ":", 2)
		if len(parts) != 2 || (parts[0] != "school" && parts[0] != "major") {
			return errors.New("audience selector must be all_verified_users, school:<uuid> or major:<uuid>")
		}
		if _, err := uuid.Parse(parts[1]); err != nil {
			return errors.New("audience selector id must be UUID")
		}
	}
	for _, attachment := range input.Attachments {
		if !strings.HasPrefix(attachment.ObjectKey, "notice_attachment/") || utf8.RuneCountInString(attachment.ObjectKey) > 500 || !isSHA256(attachment.SHA256) {
			return errors.New("invalid attachment object_key or sha256")
		}
		if attachment.ContentType != "image/jpeg" && attachment.ContentType != "image/png" && attachment.ContentType != "image/webp" && attachment.ContentType != "application/pdf" {
			return errors.New("unsupported attachment content_type")
		}
		if attachment.SizeBytes < 1 || attachment.SizeBytes > 20<<20 {
			return errors.New("attachment size_bytes out of range")
		}
	}
	sum := sha256.Sum256([]byte(input.Body))
	actual := hex.EncodeToString(sum[:])
	if input.ContentSHA256 != actual {
		return errors.New("content_sha256 does not match body")
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func defaultImportance(value string) string {
	if value == "" {
		return "normal"
	}
	return value
}

var errVersionConflict = errors.New("resource version conflict")

func ensureRequestID(ctx *gin.Context) string {
	value := strings.TrimSpace(ctx.GetHeader("X-Request-Id"))
	if value == "" {
		value = "req_" + uuid.NewString()
	}
	ctx.Header("X-Request-Id", value)
	return value
}

func writeSuccess(ctx *gin.Context, data any) {
	ctx.JSON(http.StatusOK, gin.H{"data": data, "request_id": ensureRequestID(ctx)})
}

func writeError(ctx *gin.Context, status int, code, message string, details any) {
	ctx.JSON(status, gin.H{"error": gin.H{"code": code, "message": message, "details": details}, "request_id": ensureRequestID(ctx)})
}
