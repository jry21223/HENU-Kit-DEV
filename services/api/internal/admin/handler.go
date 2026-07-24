package admin

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/notification"
	"final-review-platform/services/api/internal/orderstate"
	"final-review-platform/services/api/internal/paymentincident"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/response"
)

const maxUploadBytes = 20 * 1024 * 1024
const defaultOperationLogRetentionDays = 180
const defaultOperationLogExportLimit = 5000

var (
	errUnsafeSelfUpdate          = errors.New("unsafe_self_update")
	errSuperAdminRequired        = errors.New("super_admin_required")
	errGrantUserNotFound         = errors.New("grant_user_not_found")
	errGrantResourceNotFound     = errors.New("grant_resource_not_found")
	errGrantResourceNotPublished = errors.New("grant_resource_not_published")
	errGrantMaterialNotGrantable = errors.New("grant_material_not_grantable")
	errGrantResourceSelection    = errors.New("grant_resource_selection")
	errGrantExpirationInPast     = errors.New("grant_expiration_in_past")
	errPackageReferenceNotFound  = errors.New("package_reference_not_found")
	errPackageReferenceMismatch  = errors.New("package_reference_mismatch")
	errPackageItemUnsupported    = errors.New("package_item_unsupported")
	errPaymentIncidentNotOpen    = errors.New("payment_incident_not_open")
	errUnsafeStorageKey          = errUnsafeStorageKey
	errUnsupportedFileType       = errUnsupportedFileType
	errFileNotFound              = errFileNotFound
	errInvalidFile               = errInvalidFile
	errInvalidFileContent        = errInvalidFileContent
	errUnsafeFileName            = errUnsafeFileName
)

var allowedUploadExtensions = map[string]bool{
	".pdf":  true,
	".txt":  true,
	".md":   true,
	".docx": true,
}

type Handler struct {
	db                            *gorm.DB
	uploadDir                     string
	operationLogRetentionDays     int
	operationLogExportLimit       int
	paymentIncidentOverdueMinutes int
	paymentIncidentAlerts         config.PaymentIncidentAlertConfig
	paymentIncidentEnvironment    string
}

func NewHandler(db *gorm.DB, uploadDir string, operationLogRetentionDays int, operationLogExportLimit int, paymentIncidentAlerts config.PaymentIncidentAlertConfig, environment string) Handler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	if operationLogRetentionDays <= 0 {
		operationLogRetentionDays = defaultOperationLogRetentionDays
	}
	if operationLogExportLimit <= 0 {
		operationLogExportLimit = defaultOperationLogExportLimit
	}
	paymentIncidentOverdueMinutes := paymentIncidentAlerts.OverdueMinutes
	if paymentIncidentOverdueMinutes <= 0 {
		paymentIncidentOverdueMinutes = 30
	}
	return Handler{
		db:                            db,
		uploadDir:                     uploadDir,
		operationLogRetentionDays:     operationLogRetentionDays,
		operationLogExportLimit:       operationLogExportLimit,
		paymentIncidentOverdueMinutes: paymentIncidentOverdueMinutes,
		paymentIncidentAlerts:         paymentIncidentAlerts,
		paymentIncidentEnvironment:    strings.TrimSpace(environment),
	}
}

type schoolRequest struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	EmailDomains string `json:"emailDomains"`
	Status       string `json:"status"`
}

type collegeRequest struct {
	SchoolID string `json:"schoolId"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

type majorRequest struct {
	SchoolID  string `json:"schoolId"`
	CollegeID string `json:"collegeId"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Status    string `json:"status"`
}

type courseRequest struct {
	SchoolID    string `json:"schoolId"`
	CollegeID   string `json:"collegeId"`
	MajorID     string `json:"majorId"`
	Grade       string `json:"grade"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	ExamScope   string `json:"examScope"`
	Status      string `json:"status"`
}

type coursePackageRequest struct {
	SchoolID    string  `json:"schoolId"`
	CollegeID   string  `json:"collegeId"`
	MajorID     string  `json:"majorId"`
	CourseID    *string `json:"courseId"`
	Grade       string  `json:"grade"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	PriceFen    *int64  `json:"priceFen"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
}

type packageItemRequest struct {
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	SortOrder    int    `json:"sortOrder"`
}

type materialRequest struct {
	CourseID       string `json:"courseId"`
	Title          string `json:"title"`
	Type           string `json:"type"`
	Description    string `json:"description"`
	StorageKey     string `json:"storageKey"`
	FileName       string `json:"fileName"`
	FileSize       int64  `json:"fileSize"`
	PreviewContent string `json:"previewContent"`
	AccessLevel    string `json:"accessLevel"`
	Status         string `json:"status"`
}

type materialStatusRequest struct {
	Status string `json:"status"`
}

type materialReviewRequest struct {
	ReviewReason string `json:"reviewReason"`
}

type userUpdateRequest struct {
	Name   *string `json:"name"`
	Role   *string `json:"role"`
	Status *string `json:"status"`
}

type accessGrantRequest struct {
	UserID     string `json:"userId"`
	MaterialID string `json:"materialId"`
	PackageID  string `json:"packageId"`
	ExpiresAt  string `json:"expiresAt"`
}

type resolvePaymentIncidentRequest struct {
	Status     string `json:"status"`
	HandleNote string `json:"handleNote"`
}

type cleanupMediaAssetsRequest struct {
	OlderThanHours *int  `json:"olderThanHours"`
	DryRun         *bool `json:"dryRun"`
	Limit          *int  `json:"limit"`
}

type accessGrantRow struct {
	Grant    model.MaterialAccessGrant `json:"grant"`
	User     *model.User               `json:"user,omitempty"`
	Material *model.Material           `json:"material,omitempty"`
	Package  *model.CoursePackage      `json:"package,omitempty"`
	Active   bool                      `json:"active"`
}

type orderRow struct {
	Order              model.Order          `json:"order"`
	User               *model.User          `json:"user,omitempty"`
	Package            *model.CoursePackage `json:"package,omitempty"`
	EntitlementGranted bool                 `json:"entitlementGranted"`
}

type paymentReconciliationIssue struct {
	IssueType       string `json:"issueType"`
	Severity        string `json:"severity"`
	Message         string `json:"message"`
	OrderID         string `json:"orderId,omitempty"`
	OutTradeNo      string `json:"outTradeNo,omitempty"`
	OrderStatus     string `json:"orderStatus,omitempty"`
	PaymentProvider string `json:"paymentProvider,omitempty"`
	AmountTotal     int64  `json:"amountTotal,omitempty"`
	RiskFlag        string `json:"riskFlag,omitempty"`
	UserID          string `json:"userId,omitempty"`
	UserEmail       string `json:"userEmail,omitempty"`
	PackageID       string `json:"packageId,omitempty"`
	PackageTitle    string `json:"packageTitle,omitempty"`
	PaymentRecordID string `json:"paymentRecordId,omitempty"`
	TransactionID   string `json:"transactionId,omitempty"`
	GrantID         string `json:"grantId,omitempty"`
	IncidentID      string `json:"incidentId,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
}

type paymentReconciliationSummary struct {
	Total    int            `json:"total"`
	Critical int            `json:"critical"`
	High     int            `json:"high"`
	Medium   int            `json:"medium"`
	Low      int            `json:"low"`
	Types    map[string]int `json:"types"`
}

type paymentIncidentSummary struct {
	Total                   int64            `json:"total"`
	Open                    int64            `json:"open"`
	Resolved                int64            `json:"resolved"`
	Ignored                 int64            `json:"ignored"`
	OverdueOpen             int64            `json:"overdueOpen"`
	OverdueThresholdMinutes int              `json:"overdueThresholdMinutes"`
	OpenCritical            int64            `json:"openCritical"`
	OpenHigh                int64            `json:"openHigh"`
	OpenMedium              int64            `json:"openMedium"`
	OpenLow                 int64            `json:"openLow"`
	OpenBySeverity          map[string]int64 `json:"openBySeverity"`
	OpenByType              map[string]int64 `json:"openByType"`
	OldestOpenAt            string           `json:"oldestOpenAt,omitempty"`
	OldestOpenAgeMinutes    int64            `json:"oldestOpenAgeMinutes,omitempty"`
}

type paymentIncidentRow struct {
	ID             string     `json:"id"`
	OrderID        *string    `json:"orderId,omitempty"`
	Provider       string     `json:"provider"`
	IncidentType   string     `json:"incidentType"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	OutTradeNo     string     `json:"outTradeNo"`
	TransactionID  string     `json:"transactionId"`
	TradeState     string     `json:"tradeState"`
	ExpectedAmount int64      `json:"expectedAmount"`
	ActualAmount   int64      `json:"actualAmount"`
	Message        string     `json:"message"`
	HandledBy      *string    `json:"handledBy,omitempty"`
	HandledAt      *time.Time `json:"handledAt,omitempty"`
	HandleNote     string     `json:"handleNote,omitempty"`
	AlertCount     int        `json:"alertCount"`
	LastAlertedAt  *time.Time `json:"lastAlertedAt,omitempty"`
	LastAlertedBy  *string    `json:"lastAlertedBy,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type paymentIncidentAlertSummary struct {
	AlertCount    int
	LastAlertedAt *time.Time
	LastAlertedBy *string
}

type mediaAssetRow struct {
	Asset   model.MediaAsset `json:"asset"`
	Owner   *model.User      `json:"owner,omitempty"`
	HasFile bool             `json:"hasFile"`
}

type mediaCleanupSummary struct {
	DryRun         bool            `json:"dryRun"`
	OlderThanHours int             `json:"olderThanHours"`
	Cutoff         string          `json:"cutoff"`
	Candidates     int             `json:"candidates"`
	DeletedFiles   int             `json:"deletedFiles"`
	MissingFiles   int             `json:"missingFiles"`
	ArchivedRows   int64           `json:"archivedRows"`
	Assets         []mediaAssetRow `json:"assets"`
}

type packageItemRow struct {
	Item     model.CoursePackageItem `json:"item"`
	Material *model.Material         `json:"material,omitempty"`
}

func (h Handler) ListUsers(ctx *gin.Context) {
	query := h.db.Model(&model.User{})
	if email := strings.TrimSpace(ctx.Query("email")); email != "" {
		query = query.Where("email LIKE ?", "%"+email+"%")
	}
	if role := strings.TrimSpace(ctx.Query("role")); role != "" {
		normalized, ok := normalizeUserRole(role, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_role", nil)
			return
		}
		query = query.Where("role = ?", normalized)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		normalized, ok := normalizeUserStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", normalized)
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var users []model.User
	if err := query.Order("created_at desc").Limit(limit).Find(&users).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"users": users})
}

func (h Handler) UpdateUser(ctx *gin.Context) {
	current, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req userUpdateRequest
	if !bindJSON(ctx, &req) {
		return
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_name", nil)
			return
		}
		updates["name"] = name
	}
	if req.Role != nil {
		role, ok := normalizeUserRole(*req.Role, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_role", nil)
			return
		}
		updates["role"] = role
	}
	if req.Status != nil {
		status, ok := normalizeUserStatus(*req.Status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		updates["status"] = status
		if status == "active" {
			updates["frozen_until"] = nil
		}
	}
	if len(updates) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "empty_update", nil)
		return
	}

	targetID := ctx.Param("id")
	var updated model.User
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var target model.User
		if err := tx.First(&target, "id = ?", targetID).Error; err != nil {
			return err
		}
		if target.ID == current.ID && (req.Role != nil || req.Status != nil) {
			return errUnsafeSelfUpdate
		}
		if (target.Role == model.RoleSuperAdmin || updates["role"] == model.RoleSuperAdmin) && current.Role != model.RoleSuperAdmin {
			return errSuperAdminRequired
		}
		if err := tx.Model(&target).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&updated, "id = ?", target.ID).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "user.update", "user", target.ID, map[string]interface{}{
			"fields":         sortedKeys(updates),
			"previousRole":   target.Role,
			"role":           updated.Role,
			"previousStatus": target.Status,
			"status":         updated.Status,
		})
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "user_not_found", nil)
		case errors.Is(err, errUnsafeSelfUpdate):
			response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "unsafe_self_update", nil)
		case errors.Is(err, errSuperAdminRequired):
			response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "super_admin_required", nil)
		default:
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "update_failed", nil)
		}
		return
	}
	response.OK(ctx, gin.H{"user": updated})
}

func (h Handler) ListMediaAssets(ctx *gin.Context) {
	query := h.db.Model(&model.MediaAsset{})
	if usage := strings.TrimSpace(ctx.Query("usage")); usage != "" {
		if usage != "moment_image" {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_usage", nil)
			return
		}
		query = query.Where("usage = ?", usage)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		if !validMediaAssetStatus(status) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", status)
	}
	if ownerEmail := strings.TrimSpace(ctx.Query("ownerEmail")); ownerEmail != "" {
		var owners []model.User
		if err := h.db.Where("email LIKE ?", "%"+ownerEmail+"%").Limit(200).Find(&owners).Error; err != nil {
			response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
			return
		}
		ownerIDs := make([]string, 0, len(owners))
		for _, owner := range owners {
			ownerIDs = append(ownerIDs, owner.ID)
		}
		if len(ownerIDs) == 0 {
			response.OK(ctx, gin.H{"assets": []mediaAssetRow{}})
			return
		}
		query = query.Where("owner_id IN ?", ownerIDs)
	}
	if momentID := strings.TrimSpace(ctx.Query("momentId")); momentID != "" {
		query = query.Where("moment_id = ?", momentID)
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var assets []model.MediaAsset
	if err := query.Order("created_at desc").Limit(limit).Find(&assets).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	rows, err := h.mediaAssetRows(assets)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"assets": rows})
}

func (h Handler) CleanupMediaAssets(ctx *gin.Context) {
	current, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req cleanupMediaAssetsRequest
	if !bindJSON(ctx, &req) {
		return
	}
	olderThanHours := 24
	if req.OlderThanHours != nil {
		olderThanHours = *req.OlderThanHours
	}
	if olderThanHours <= 0 || olderThanHours > 24*30 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_older_than_hours", nil)
		return
	}
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	limit := 200
	if req.Limit != nil {
		limit = *req.Limit
	}
	if limit <= 0 || limit > 500 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	cutoff := time.Now().Add(-time.Duration(olderThanHours) * time.Hour)
	var assets []model.MediaAsset
	if err := h.db.
		Where("usage = ? AND status = ? AND moment_id IS NULL AND created_at < ?", "moment_image", "uploaded", cutoff).
		Order("created_at asc").
		Limit(limit).
		Find(&assets).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	rows, err := h.mediaAssetRows(assets)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	summary := mediaCleanupSummary{
		DryRun:         dryRun,
		OlderThanHours: olderThanHours,
		Cutoff:         cutoff.UTC().Format(time.RFC3339),
		Candidates:     len(assets),
		Assets:         rows,
	}
	if dryRun || len(assets) == 0 {
		response.OK(ctx, gin.H{"cleanup": summary})
		return
	}
	ids := make([]string, 0, len(assets))
	for _, asset := range assets {
		ids = append(ids, asset.ID)
		path, err := adminSafeStoragePath(h.uploadDir, asset.StorageKey)
		if err != nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsafe_storage_key", nil)
			return
		}
		if err := os.Remove(path); err == nil {
			summary.DeletedFiles++
		} else if errors.Is(err, os.ErrNotExist) {
			summary.MissingFiles++
		} else {
			response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "cleanup_failed", nil)
			return
		}
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&model.MediaAsset{}).
			Where("id IN ? AND status = ? AND moment_id IS NULL", ids, "uploaded").
			Update("status", model.StatusArchived)
		if update.Error != nil {
			return update.Error
		}
		summary.ArchivedRows = update.RowsAffected
		return audit.Record(ctx, tx, "media_asset.cleanup", "media_asset", "", map[string]interface{}{
			"operatorId":     current.ID,
			"olderThanHours": olderThanHours,
			"candidateCount": len(assets),
			"deletedFiles":   summary.DeletedFiles,
			"missingFiles":   summary.MissingFiles,
			"archivedRows":   summary.ArchivedRows,
			"limit":          limit,
		})
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "cleanup_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"cleanup": summary})
}

func (h Handler) ListAccessGrants(ctx *gin.Context) {
	query := h.db.Model(&model.MaterialAccessGrant{})
	if userID := strings.TrimSpace(ctx.Query("userId")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if materialID := strings.TrimSpace(ctx.Query("materialId")); materialID != "" {
		query = query.Where("material_id = ?", materialID)
	}
	if packageID := strings.TrimSpace(ctx.Query("packageId")); packageID != "" {
		query = query.Where("package_id = ?", packageID)
	}
	if source := strings.TrimSpace(ctx.Query("source")); source != "" {
		query = query.Where("source = ?", source)
	}
	now := time.Now()
	if active := strings.TrimSpace(ctx.Query("active")); active != "" {
		switch active {
		case "true":
			query = query.Where("expires_at IS NULL OR expires_at > ?", now)
		case "false":
			query = query.Where("expires_at IS NOT NULL AND expires_at <= ?", now)
		default:
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_active_filter", nil)
			return
		}
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var grants []model.MaterialAccessGrant
	if err := query.Order("created_at desc").Limit(limit).Find(&grants).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	rows, err := h.accessGrantRows(grants, now)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"grants": rows})
}

func (h Handler) CreateAccessGrant(ctx *gin.Context) {
	var req accessGrantRequest
	if !bindJSON(ctx, &req) {
		return
	}
	userID := strings.TrimSpace(req.UserID)
	materialID := strings.TrimSpace(req.MaterialID)
	packageID := strings.TrimSpace(req.PackageID)
	expiresAt, ok := parseGrantExpiresAt(req.ExpiresAt)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_expires_at", nil)
		return
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "expires_at_in_past", nil)
		return
	}

	var grant model.MaterialAccessGrant
	alreadyGranted := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		created, existing, err := h.createAccessGrant(ctx, tx, userID, materialID, packageID, expiresAt)
		if err != nil {
			return err
		}
		grant = created
		alreadyGranted = existing
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errGrantUserNotFound):
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "user_not_found", nil)
		case errors.Is(err, errGrantResourceNotFound):
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "grant_resource_not_found", nil)
		case errors.Is(err, errGrantResourceNotPublished):
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "grant_resource_not_published", nil)
		case errors.Is(err, errGrantMaterialNotGrantable):
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "material_not_grantable", nil)
		case errors.Is(err, errGrantResourceSelection):
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_grant_resource", nil)
		case errors.Is(err, errGrantExpirationInPast):
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "expires_at_in_past", nil)
		default:
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		}
		return
	}
	response.OK(ctx, gin.H{"grant": grant, "alreadyGranted": alreadyGranted})
}

func (h Handler) RevokeAccessGrant(ctx *gin.Context) {
	grantID := ctx.Param("id")
	var grant model.MaterialAccessGrant
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&grant, "id = ?", grantID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&grant).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "access_grant.revoke", "access_grant", grant.ID, map[string]interface{}{
			"userId":     grant.UserID,
			"materialId": grant.MaterialID,
			"packageId":  grant.PackageID,
			"source":     grant.Source,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "grant_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "revoke_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"revoked": true})
}

func (h Handler) ListOrders(ctx *gin.Context) {
	if err := orderstate.ExpireAllStale(h.db, time.Now()); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	query := h.db.Model(&model.Order{})
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		if !validOrderStatus(status) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", status)
	}
	if provider := strings.TrimSpace(ctx.Query("paymentProvider")); provider != "" {
		query = query.Where("payment_provider = ?", provider)
	}
	if productType := strings.TrimSpace(ctx.Query("productType")); productType != "" {
		query = query.Where("product_type = ?", productType)
	}
	if packageID := strings.TrimSpace(ctx.Query("packageId")); packageID != "" {
		query = query.Where("product_type = ? AND product_id = ?", "course_package", packageID)
	}
	if userID := strings.TrimSpace(ctx.Query("userId")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if outTradeNo := strings.TrimSpace(ctx.Query("outTradeNo")); outTradeNo != "" {
		query = query.Where("out_trade_no LIKE ?", "%"+outTradeNo+"%")
	}
	if riskFlag := strings.TrimSpace(ctx.Query("riskFlag")); riskFlag != "" {
		if len(riskFlag) > 120 {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_risk_flag", nil)
			return
		}
		query = query.Where("risk_flag LIKE ?", "%"+riskFlag+"%")
	}
	if strings.EqualFold(strings.TrimSpace(ctx.Query("riskOnly")), "true") {
		query = query.Where("risk_flag <> ''")
	}
	if userEmail := strings.TrimSpace(ctx.Query("userEmail")); userEmail != "" {
		var users []model.User
		if err := h.db.Where("email LIKE ?", "%"+userEmail+"%").Limit(200).Find(&users).Error; err != nil {
			response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
			return
		}
		if len(users) == 0 {
			response.OK(ctx, gin.H{"orders": []orderRow{}})
			return
		}
		userIDs := make([]string, 0, len(users))
		for _, user := range users {
			userIDs = append(userIDs, user.ID)
		}
		query = query.Where("user_id IN ?", userIDs)
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var orders []model.Order
	if err := query.Order("created_at desc").Limit(limit).Find(&orders).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	rows, err := h.orderRows(orders, time.Now())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"orders": rows})
}

func (h Handler) ListPaymentIncidents(ctx *gin.Context) {
	status := strings.TrimSpace(ctx.Query("status"))
	if status == "" {
		status = model.PaymentIncidentOpen
	}
	if status != "all" && !validPaymentIncidentStatus(status) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 100, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Model(&model.PaymentIncident{})
	if status != "all" {
		query = query.Where("status = ?", status)
	}
	if incidentType := strings.TrimSpace(ctx.Query("incidentType")); incidentType != "" {
		if len(incidentType) > 80 {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_incident_type", nil)
			return
		}
		query = query.Where("incident_type = ?", incidentType)
	}
	if severity := strings.TrimSpace(ctx.Query("severity")); severity != "" {
		if !validPaymentReconciliationSeverity(severity) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_severity", nil)
			return
		}
		query = query.Where("severity = ?", severity)
	}
	if overdueOnly, ok := parseOptionalBool(ctx.Query("overdue")); !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_overdue", nil)
		return
	} else if overdueOnly {
		if status != model.PaymentIncidentOpen && status != "all" {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_overdue_status", nil)
			return
		}
		overdueMinutes := h.paymentIncidentOverdueMinutes
		if overdueMinutes <= 0 {
			overdueMinutes = 30
		}
		overdueCutoff := time.Now().UTC().Add(-time.Duration(overdueMinutes) * time.Minute)
		query = query.Where("status = ? AND created_at <= ?", model.PaymentIncidentOpen, overdueCutoff)
	}
	if orderID := strings.TrimSpace(ctx.Query("orderId")); orderID != "" {
		if _, err := uuid.Parse(orderID); err != nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_order_id", nil)
			return
		}
		query = query.Where("order_id = ?", orderID)
	}
	if outTradeNo := strings.TrimSpace(ctx.Query("outTradeNo")); outTradeNo != "" {
		query = query.Where("out_trade_no LIKE ?", "%"+outTradeNo+"%")
	}
	if transactionID := strings.TrimSpace(ctx.Query("transactionId")); transactionID != "" {
		query = query.Where("transaction_id LIKE ?", "%"+transactionID+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	var incidents []model.PaymentIncident
	if err := query.Order("created_at desc").Limit(limit).Find(&incidents).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	alerts, err := h.paymentIncidentAlertSummaries(incidents)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"incidents": paymentIncidentRows(incidents, alerts), "total": total})
}

func (h Handler) PaymentIncidentSummary(ctx *gin.Context) {
	overdueMinutes := h.paymentIncidentOverdueMinutes
	if overdueMinutes <= 0 {
		overdueMinutes = 30
	}
	summary := paymentIncidentSummary{
		OverdueThresholdMinutes: overdueMinutes,
		OpenBySeverity:          map[string]int64{},
		OpenByType:              map[string]int64{},
	}
	var statusRows []struct {
		Status string
		Count  int64
	}
	if err := h.db.Model(&model.PaymentIncident{}).
		Select("status, count(*) as count").
		Group("status").
		Scan(&statusRows).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	for _, row := range statusRows {
		summary.Total += row.Count
		switch row.Status {
		case model.PaymentIncidentOpen:
			summary.Open = row.Count
		case model.PaymentIncidentResolved:
			summary.Resolved = row.Count
		case model.PaymentIncidentIgnored:
			summary.Ignored = row.Count
		}
	}

	var severityRows []struct {
		Severity string
		Count    int64
	}
	if err := h.db.Model(&model.PaymentIncident{}).
		Select("severity, count(*) as count").
		Where("status = ?", model.PaymentIncidentOpen).
		Group("severity").
		Scan(&severityRows).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	for _, row := range severityRows {
		summary.OpenBySeverity[row.Severity] = row.Count
		switch row.Severity {
		case "critical":
			summary.OpenCritical = row.Count
		case "high":
			summary.OpenHigh = row.Count
		case "medium":
			summary.OpenMedium = row.Count
		case "low":
			summary.OpenLow = row.Count
		}
	}

	var typeRows []struct {
		IncidentType string
		Count        int64
	}
	if err := h.db.Model(&model.PaymentIncident{}).
		Select("incident_type, count(*) as count").
		Where("status = ?", model.PaymentIncidentOpen).
		Group("incident_type").
		Scan(&typeRows).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	for _, row := range typeRows {
		summary.OpenByType[row.IncidentType] = row.Count
	}

	overdueCutoff := time.Now().UTC().Add(-time.Duration(overdueMinutes) * time.Minute)
	var openIncidents []model.PaymentIncident
	if err := h.db.Where("status = ?", model.PaymentIncidentOpen).Order("created_at asc").Find(&openIncidents).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	for _, incident := range openIncidents {
		if !incident.CreatedAt.UTC().After(overdueCutoff) {
			summary.OverdueOpen++
		}
	}
	if len(openIncidents) > 0 {
		oldestAt := openIncidents[0].CreatedAt.UTC()
		summary.OldestOpenAt = oldestAt.Format(time.RFC3339)
		summary.OldestOpenAgeMinutes = int64(time.Since(oldestAt).Minutes())
	}

	response.OK(ctx, summary)
}

func (h Handler) ListPaymentReconciliation(ctx *gin.Context) {
	if err := orderstate.ExpireAllStale(h.db, time.Now()); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 1000)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	issueType := strings.TrimSpace(ctx.Query("issueType"))
	if len(issueType) > 80 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_issue_type", nil)
		return
	}
	severity := strings.TrimSpace(ctx.Query("severity"))
	if severity != "" && !validPaymentReconciliationSeverity(severity) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_severity", nil)
		return
	}
	issues, err := h.paymentReconciliationIssues(time.Now().UTC())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	filtered := make([]paymentReconciliationIssue, 0, len(issues))
	for _, issue := range issues {
		if issueType != "" && issue.IssueType != issueType {
			continue
		}
		if severity != "" && issue.Severity != severity {
			continue
		}
		filtered = append(filtered, issue)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return paymentIssueRank(filtered[i]) < paymentIssueRank(filtered[j])
	})
	total := len(filtered)
	summary := paymentReconciliationSummaryFor(filtered)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	response.OK(ctx, gin.H{
		"issues":  filtered,
		"total":   total,
		"summary": summary,
	})
}

func (h Handler) AlertPaymentIncident(ctx *gin.Context) {
	var incident model.PaymentIncident
	if err := h.db.First(&incident, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "payment_incident_not_found", nil)
		return
	}
	if incident.Status != model.PaymentIncidentOpen {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "payment_incident_not_open", gin.H{"status": incident.Status})
		return
	}
	result, err := paymentincident.SendAlert(ctx.Request.Context(), h.paymentIncidentAlerts, h.paymentIncidentEnvironment, paymentincident.EventRealerted, incident)
	if err != nil {
		if errors.Is(err, paymentincident.ErrWebhookNotConfigured) {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "payment_incident_webhook_not_configured", nil)
			return
		}
		if errors.Is(err, paymentincident.ErrInvalidWebhookURL) {
			response.Error(ctx, http.StatusBadGateway, response.CodeInternalServer, "payment_incident_webhook_invalid_url", nil)
			return
		}
		response.Error(ctx, http.StatusBadGateway, response.CodeInternalServer, "payment_incident_webhook_failed", gin.H{"statusCode": result.StatusCode})
		return
	}
	if err := audit.Record(ctx, h.db, "payment_incident.alert", "payment_incident", incident.ID, map[string]interface{}{
		"incidentType":  incident.IncidentType,
		"severity":      incident.Severity,
		"outTradeNo":    incident.OutTradeNo,
		"transactionId": incident.TransactionID,
		"statusCode":    result.StatusCode,
	}); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "payment_incident_alert_audit_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"alertSent": result.Sent, "statusCode": result.StatusCode})
}

func (h Handler) ResolvePaymentIncident(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req resolvePaymentIncidentRequest
	if ctx.Request.Body != nil && ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
			return
		}
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = model.PaymentIncidentResolved
	}
	if status != model.PaymentIncidentResolved && status != model.PaymentIncidentIgnored {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	note := strings.TrimSpace(req.HandleNote)
	if len([]rune(note)) > 1000 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "handle_note_too_long", nil)
		return
	}

	var incident model.PaymentIncident
	if err := h.db.First(&incident, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "payment_incident_not_found", nil)
		return
	}
	if incident.Status != model.PaymentIncidentOpen {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "payment_incident_not_open", gin.H{"status": incident.Status})
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.PaymentIncident{}).
			Where("id = ? AND status = ?", incident.ID, model.PaymentIncidentOpen).
			Updates(map[string]interface{}{
				"status":      status,
				"handled_by":  user.ID,
				"handled_at":  gorm.Expr("CURRENT_TIMESTAMP"),
				"handle_note": note,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errPaymentIncidentNotOpen
		}
		return audit.Record(ctx, tx, "payment_incident."+status, "payment_incident", incident.ID, map[string]interface{}{
			"orderId":       incident.OrderID,
			"incidentType":  incident.IncidentType,
			"outTradeNo":    incident.OutTradeNo,
			"transactionId": incident.TransactionID,
			"status":        status,
		})
	})
	if err != nil {
		if errors.Is(err, errPaymentIncidentNotOpen) {
			response.Error(ctx, http.StatusConflict, response.CodeConflict, "payment_incident_not_open", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "payment_incident_update_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"handled": true, "status": status})
}

func (h Handler) OperationLogs(ctx *gin.Context) {
	query, ok := h.operationLogQuery(ctx)
	if !ok {
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var logs []model.OperationLog
	if err := query.Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"operationLogs": logs})
}

func (h Handler) ExportOperationLogs(ctx *gin.Context) {
	query, ok := h.operationLogQuery(ctx)
	if !ok {
		return
	}
	limit, ok := parseLimit(ctx.Query("limit"), h.operationLogExportLimit, h.operationLogExportLimit)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var logs []model.OperationLog
	if err := query.Order("created_at desc").Limit(limit).Find(&logs).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}

	fileName := "operation-logs-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	ctx.Header("Content-Type", "text/csv; charset=utf-8")
	ctx.Header("Content-Disposition", "attachment; filename="+fileName)
	writer := csv.NewWriter(ctx.Writer)
	_ = writer.Write([]string{"id", "created_at", "operator_id", "action", "target_type", "target_id", "ip", "user_agent", "metadata"})
	for _, log := range logs {
		_ = writer.Write([]string{
			safeCSVCell(log.ID),
			safeCSVCell(log.CreatedAt.UTC().Format(time.RFC3339)),
			safeCSVCell(log.OperatorID),
			safeCSVCell(log.Action),
			safeCSVCell(log.TargetType),
			safeCSVCell(log.TargetID),
			safeCSVCell(log.IP),
			safeCSVCell(log.UserAgent),
			safeCSVCell(jsonString(log.Metadata)),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "export_failed", nil)
		return
	}
}

func (h Handler) OperationLogRetention(ctx *gin.Context) {
	response.OK(ctx, gin.H{
		"retentionDays":     h.operationLogRetentionDays,
		"exportLimit":       h.operationLogExportLimit,
		"automaticDeletion": false,
		"policy":            "Operation logs are retained for audit review; automatic deletion is not enabled in the MVP.",
		"recommendedReview": "Review and export high-risk admin mutations before manual retention cleanup.",
	})
}

func (h Handler) operationLogQuery(ctx *gin.Context) (*gorm.DB, bool) {
	query := h.db.Model(&model.OperationLog{})
	if operatorID := strings.TrimSpace(ctx.Query("operatorId")); operatorID != "" {
		query = query.Where("operator_id = ?", operatorID)
	}
	if action := strings.TrimSpace(ctx.Query("action")); action != "" {
		query = query.Where("action = ?", action)
	}
	if targetType := strings.TrimSpace(ctx.Query("targetType")); targetType != "" {
		query = query.Where("target_type = ?", targetType)
	}
	if targetID := strings.TrimSpace(ctx.Query("targetId")); targetID != "" {
		query = query.Where("target_id = ?", targetID)
	}
	if createdFrom := strings.TrimSpace(ctx.Query("createdFrom")); createdFrom != "" {
		parsed, ok := parseLogTime(createdFrom, false)
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_created_from", nil)
			return nil, false
		}
		query = query.Where("created_at >= ?", parsed)
	}
	if createdTo := strings.TrimSpace(ctx.Query("createdTo")); createdTo != "" {
		parsed, ok := parseLogTime(createdTo, true)
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_created_to", nil)
			return nil, false
		}
		query = query.Where("created_at <= ?", parsed)
	}
	return query, true
}

func (h Handler) CreateSchool(ctx *gin.Context) {
	var req schoolRequest
	if !bindJSON(ctx, &req) {
		return
	}
	school := model.School{
		Name:         required(req.Name),
		Slug:         required(req.Slug),
		EmailDomains: strings.TrimSpace(req.EmailDomains),
		Status:       defaultStatus(req.Status),
	}
	if school.Name == "" || school.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&school).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "school.create", "school", school.ID, map[string]interface{}{
			"slug":   school.Slug,
			"status": school.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"school": school})
}

func (h Handler) UpdateSchool(ctx *gin.Context) {
	var req schoolRequest
	if !bindJSON(ctx, &req) {
		return
	}
	updates := compactMap(map[string]interface{}{
		"name":          strings.TrimSpace(req.Name),
		"slug":          strings.TrimSpace(req.Slug),
		"email_domains": strings.TrimSpace(req.EmailDomains),
		"status":        strings.TrimSpace(req.Status),
	})
	h.updateByID(ctx, &model.School{}, updates, "school")
}

func (h Handler) ArchiveSchool(ctx *gin.Context) {
	h.archiveByID(ctx, &model.School{}, "school")
}

func (h Handler) CreateCollege(ctx *gin.Context) {
	var req collegeRequest
	if !bindJSON(ctx, &req) {
		return
	}
	college := model.College{
		SchoolID: strings.TrimSpace(req.SchoolID),
		Name:     required(req.Name),
		Status:   defaultStatus(req.Status),
	}
	if college.SchoolID == "" || college.Name == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&college).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "college.create", "college", college.ID, map[string]interface{}{
			"schoolId": college.SchoolID,
			"status":   college.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"college": college})
}

func (h Handler) UpdateCollege(ctx *gin.Context) {
	var req collegeRequest
	if !bindJSON(ctx, &req) {
		return
	}
	updates := compactMap(map[string]interface{}{
		"school_id": strings.TrimSpace(req.SchoolID),
		"name":      strings.TrimSpace(req.Name),
		"status":    strings.TrimSpace(req.Status),
	})
	h.updateByID(ctx, &model.College{}, updates, "college")
}

func (h Handler) ArchiveCollege(ctx *gin.Context) {
	h.archiveByID(ctx, &model.College{}, "college")
}

func (h Handler) CreateMajor(ctx *gin.Context) {
	var req majorRequest
	if !bindJSON(ctx, &req) {
		return
	}
	major := model.Major{
		SchoolID:  strings.TrimSpace(req.SchoolID),
		CollegeID: strings.TrimSpace(req.CollegeID),
		Name:      required(req.Name),
		Slug:      required(req.Slug),
		Status:    defaultStatus(req.Status),
	}
	if major.SchoolID == "" || major.CollegeID == "" || major.Name == "" || major.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&major).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "major.create", "major", major.ID, map[string]interface{}{
			"schoolId":  major.SchoolID,
			"collegeId": major.CollegeID,
			"slug":      major.Slug,
			"status":    major.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"major": major})
}

func (h Handler) UpdateMajor(ctx *gin.Context) {
	var req majorRequest
	if !bindJSON(ctx, &req) {
		return
	}
	updates := compactMap(map[string]interface{}{
		"school_id":  strings.TrimSpace(req.SchoolID),
		"college_id": strings.TrimSpace(req.CollegeID),
		"name":       strings.TrimSpace(req.Name),
		"slug":       strings.TrimSpace(req.Slug),
		"status":     strings.TrimSpace(req.Status),
	})
	h.updateByID(ctx, &model.Major{}, updates, "major")
}

func (h Handler) ArchiveMajor(ctx *gin.Context) {
	h.archiveByID(ctx, &model.Major{}, "major")
}

func (h Handler) CreateCourse(ctx *gin.Context) {
	var req courseRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status, ok := normalizeCourseStatus(req.Status, model.StatusPublished)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	course := model.Course{
		SchoolID:    strings.TrimSpace(req.SchoolID),
		CollegeID:   strings.TrimSpace(req.CollegeID),
		MajorID:     strings.TrimSpace(req.MajorID),
		Grade:       required(req.Grade),
		Name:        required(req.Name),
		Slug:        required(req.Slug),
		Description: strings.TrimSpace(req.Description),
		ExamScope:   strings.TrimSpace(req.ExamScope),
		Status:      status,
	}
	if course.SchoolID == "" || course.CollegeID == "" || course.MajorID == "" || course.Grade == "" || course.Name == "" || course.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&course).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "course.create", "course", course.ID, map[string]interface{}{
			"schoolId":  course.SchoolID,
			"collegeId": course.CollegeID,
			"majorId":   course.MajorID,
			"grade":     course.Grade,
			"slug":      course.Slug,
			"status":    course.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"course": course})
}

func (h Handler) ListCourses(ctx *gin.Context) {
	query := h.db.Model(&model.Course{})
	if schoolID := strings.TrimSpace(ctx.Query("schoolId")); schoolID != "" {
		query = query.Where("school_id = ?", schoolID)
	}
	if majorID := strings.TrimSpace(ctx.Query("majorId")); majorID != "" {
		query = query.Where("major_id = ?", majorID)
	}
	if grade := strings.TrimSpace(ctx.Query("grade")); grade != "" {
		query = query.Where("grade = ?", grade)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		normalized, ok := normalizeCourseStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", normalized)
	}
	var courses []model.Course
	if err := query.Order("updated_at desc").Limit(500).Find(&courses).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"courses": courses})
}

func (h Handler) UpdateCourse(ctx *gin.Context) {
	var req courseRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != "" {
		normalized, ok := normalizeCourseStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		status = normalized
	}
	updates := compactMap(map[string]interface{}{
		"school_id":   strings.TrimSpace(req.SchoolID),
		"college_id":  strings.TrimSpace(req.CollegeID),
		"major_id":    strings.TrimSpace(req.MajorID),
		"grade":       strings.TrimSpace(req.Grade),
		"name":        strings.TrimSpace(req.Name),
		"slug":        strings.TrimSpace(req.Slug),
		"description": strings.TrimSpace(req.Description),
		"exam_scope":  strings.TrimSpace(req.ExamScope),
		"status":      status,
	})
	h.updateByID(ctx, &model.Course{}, updates, "course")
}

func (h Handler) ArchiveCourse(ctx *gin.Context) {
	h.archiveByID(ctx, &model.Course{}, "course")
}

func (h Handler) ListCoursePackages(ctx *gin.Context) {
	query := h.db.Model(&model.CoursePackage{})
	if schoolID := strings.TrimSpace(ctx.Query("schoolId")); schoolID != "" {
		query = query.Where("school_id = ?", schoolID)
	}
	if majorID := strings.TrimSpace(ctx.Query("majorId")); majorID != "" {
		query = query.Where("major_id = ?", majorID)
	}
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	if grade := strings.TrimSpace(ctx.Query("grade")); grade != "" {
		query = query.Where("grade = ?", grade)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		normalized, ok := normalizeCourseStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", normalized)
	}
	limit, ok := parseLimit(ctx.Query("limit"), 200, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	var packages []model.CoursePackage
	if err := query.Order("updated_at desc").Limit(limit).Find(&packages).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"packages": packages})
}

func (h Handler) CreateCoursePackage(ctx *gin.Context) {
	var req coursePackageRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status, ok := normalizeCourseStatus(req.Status, model.StatusDraft)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	priceFen := int64(0)
	if req.PriceFen != nil {
		if *req.PriceFen < 0 {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_price", nil)
			return
		}
		priceFen = *req.PriceFen
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = "CNY"
	}
	coursePackage := model.CoursePackage{
		SchoolID:    strings.TrimSpace(req.SchoolID),
		CollegeID:   strings.TrimSpace(req.CollegeID),
		MajorID:     strings.TrimSpace(req.MajorID),
		CourseID:    nullableTrimmedString(req.CourseID),
		Grade:       required(req.Grade),
		Title:       required(req.Title),
		Slug:        required(req.Slug),
		Description: strings.TrimSpace(req.Description),
		PriceFen:    priceFen,
		Currency:    currency,
		Status:      status,
	}
	if coursePackage.SchoolID == "" || coursePackage.CollegeID == "" || coursePackage.MajorID == "" || coursePackage.Grade == "" || coursePackage.Title == "" || coursePackage.Slug == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := validateCoursePackageReferences(tx, coursePackage); err != nil {
			return err
		}
		if err := tx.Create(&coursePackage).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "course_package.create", "course_package", coursePackage.ID, map[string]interface{}{
			"schoolId":  coursePackage.SchoolID,
			"collegeId": coursePackage.CollegeID,
			"majorId":   coursePackage.MajorID,
			"courseId":  coursePackage.CourseID,
			"grade":     coursePackage.Grade,
			"slug":      coursePackage.Slug,
			"priceFen":  coursePackage.PriceFen,
			"status":    coursePackage.Status,
		})
	}); err != nil {
		writeCoursePackageError(ctx, err, "create_failed")
		return
	}
	response.OK(ctx, gin.H{"package": coursePackage})
}

func (h Handler) UpdateCoursePackage(ctx *gin.Context) {
	var req coursePackageRequest
	if !bindJSON(ctx, &req) {
		return
	}
	updates := map[string]interface{}{}
	applyStringUpdate(updates, "school_id", req.SchoolID)
	applyStringUpdate(updates, "college_id", req.CollegeID)
	applyStringUpdate(updates, "major_id", req.MajorID)
	applyStringUpdate(updates, "grade", req.Grade)
	applyStringUpdate(updates, "title", req.Title)
	applyStringUpdate(updates, "slug", req.Slug)
	applyStringUpdate(updates, "description", req.Description)
	if req.CourseID != nil {
		if courseID := strings.TrimSpace(*req.CourseID); courseID != "" {
			updates["course_id"] = courseID
		} else {
			updates["course_id"] = nil
		}
	}
	if req.PriceFen != nil {
		if *req.PriceFen < 0 {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_price", nil)
			return
		}
		updates["price_fen"] = *req.PriceFen
	}
	if currency := strings.ToUpper(strings.TrimSpace(req.Currency)); currency != "" {
		updates["currency"] = currency
	}
	if status := strings.TrimSpace(req.Status); status != "" {
		normalized, ok := normalizeCourseStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		updates["status"] = normalized
	}
	if len(updates) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "empty_update", nil)
		return
	}

	targetID := ctx.Param("id")
	var updated model.CoursePackage
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var existing model.CoursePackage
		if err := tx.First(&existing, "id = ?", targetID).Error; err != nil {
			return err
		}
		candidate := existing
		if value, ok := updates["school_id"].(string); ok {
			candidate.SchoolID = value
		}
		if value, ok := updates["college_id"].(string); ok {
			candidate.CollegeID = value
		}
		if value, ok := updates["major_id"].(string); ok {
			candidate.MajorID = value
		}
		if value, ok := updates["grade"].(string); ok {
			candidate.Grade = value
		}
		if value, ok := updates["course_id"].(string); ok {
			candidate.CourseID = &value
		} else if _, ok := updates["course_id"]; ok {
			candidate.CourseID = nil
		}
		if err := validateCoursePackageReferences(tx, candidate); err != nil {
			return err
		}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&updated, "id = ?", existing.ID).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "course_package.update", "course_package", existing.ID, updateMetadata(updates))
	})
	if err != nil {
		writeCoursePackageError(ctx, err, "update_failed")
		return
	}
	response.OK(ctx, gin.H{"package": updated})
}

func (h Handler) ArchiveCoursePackage(ctx *gin.Context) {
	h.archiveByID(ctx, &model.CoursePackage{}, "course_package")
}

func (h Handler) ListCoursePackageItems(ctx *gin.Context) {
	packageID := ctx.Param("id")
	var coursePackage model.CoursePackage
	if err := h.db.First(&coursePackage, "id = ?", packageID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "package_not_found", nil)
		return
	}
	var items []model.CoursePackageItem
	if err := h.db.Where("package_id = ?", packageID).Order("sort_order asc, created_at asc").Find(&items).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	rows, err := h.packageItemRows(items)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"package": coursePackage, "items": rows})
}

func (h Handler) CreateCoursePackageItem(ctx *gin.Context) {
	var req packageItemRequest
	if !bindJSON(ctx, &req) {
		return
	}
	resourceType := strings.TrimSpace(req.ResourceType)
	if resourceType == "" {
		resourceType = "material"
	}
	if resourceType != "material" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsupported_resource_type", nil)
		return
	}
	resourceID := strings.TrimSpace(req.ResourceID)
	if resourceID == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	packageID := ctx.Param("id")
	var item model.CoursePackageItem
	alreadyExists := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var coursePackage model.CoursePackage
		if err := tx.First(&coursePackage, "id = ?", packageID).Error; err != nil {
			return err
		}
		var material model.Material
		if err := tx.First(&material, "id = ?", resourceID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errGrantResourceNotFound
			}
			return err
		}
		if err := validatePackageMaterialScope(tx, coursePackage, material); err != nil {
			return err
		}
		if err := tx.Where("package_id = ? AND resource_type = ? AND resource_id = ?", packageID, resourceType, resourceID).First(&item).Error; err == nil {
			alreadyExists = true
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		item = model.CoursePackageItem{
			PackageID:    packageID,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			SortOrder:    req.SortOrder,
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "course_package_item.create", "course_package_item", item.ID, map[string]interface{}{
			"packageId":    packageID,
			"resourceType": resourceType,
			"resourceId":   resourceID,
			"sortOrder":    item.SortOrder,
		})
	})
	if err != nil {
		writeCoursePackageError(ctx, err, "create_failed")
		return
	}
	response.OK(ctx, gin.H{"item": item, "alreadyExists": alreadyExists})
}

func (h Handler) DeleteCoursePackageItem(ctx *gin.Context) {
	packageID := ctx.Param("id")
	itemID := ctx.Param("itemId")
	var item model.CoursePackageItem
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("package_id = ? AND id = ?", packageID, itemID).First(&item).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&item).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "course_package_item.delete", "course_package_item", item.ID, map[string]interface{}{
			"packageId":    packageID,
			"resourceType": item.ResourceType,
			"resourceId":   item.ResourceID,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "package_item_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "delete_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"deleted": true})
}

func (h Handler) CreateMaterial(ctx *gin.Context) {
	var req materialRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status, ok := normalizeMaterialStatus(req.Status, model.StatusDraft)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	materialType, ok := normalizeMaterialType(req.Type, "other")
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_material_type", nil)
		return
	}
	accessLevel, ok := normalizeAccessLevel(req.AccessLevel, model.MaterialAccessLoginRequired)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_access_level", nil)
		return
	}
	material := model.Material{
		CourseID:       strings.TrimSpace(req.CourseID),
		Title:          required(req.Title),
		Type:           materialType,
		Description:    strings.TrimSpace(req.Description),
		StorageKey:     strings.TrimSpace(req.StorageKey),
		FileName:       strings.TrimSpace(req.FileName),
		FileSize:       req.FileSize,
		PreviewContent: strings.TrimSpace(req.PreviewContent),
		AccessLevel:    accessLevel,
		Status:         status,
	}
	if material.CourseID == "" || material.Title == "" || material.StorageKey == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	if hasUnsafePath(material.StorageKey) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsafe_storage_key", nil)
		return
	}
	fileName, fileSize, err := h.validateMaterialStorageReference(material.StorageKey, material.FileName)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	material.FileName = fileName
	material.FileSize = fileSize
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&material).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "material.create", "material", material.ID, map[string]interface{}{
			"courseId":    material.CourseID,
			"type":        material.Type,
			"accessLevel": material.AccessLevel,
			"status":      material.Status,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"material": material})
}

func (h Handler) ListMaterials(ctx *gin.Context) {
	query := h.db.Model(&model.Material{})
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		if _, ok := normalizeMaterialStatus(status, ""); !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		query = query.Where("status = ?", status)
	}
	var materials []model.Material
	if err := query.Order("updated_at desc").Limit(500).Find(&materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"materials": materials})
}

func (h Handler) ListMaterialReviews(ctx *gin.Context) {
	status := strings.TrimSpace(ctx.Query("status"))
	if status == "" {
		status = model.StatusPending
	}
	if !isMaterialReviewListStatus(status) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	query := h.db.Model(&model.Material{}).Where("status = ?", status)
	if courseID := strings.TrimSpace(ctx.Query("courseId")); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}
	var materials []model.Material
	if err := query.Order("updated_at desc").Limit(500).Find(&materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"materials": materials})
}

func (h Handler) UpdateMaterial(ctx *gin.Context) {
	if !rejectMaterialFileFieldUpdates(ctx) {
		return
	}
	var req materialRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status := strings.TrimSpace(req.Status)
	if status != "" {
		normalized, ok := normalizeMaterialStatus(status, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
			return
		}
		status = normalized
	}
	materialType := strings.TrimSpace(req.Type)
	if materialType != "" {
		normalized, ok := normalizeMaterialType(materialType, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_material_type", nil)
			return
		}
		materialType = normalized
	}
	accessLevel := strings.TrimSpace(req.AccessLevel)
	if accessLevel != "" {
		normalized, ok := normalizeAccessLevel(accessLevel, "")
		if !ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_access_level", nil)
			return
		}
		accessLevel = normalized
	}
	updates := compactMap(map[string]interface{}{
		"course_id":       strings.TrimSpace(req.CourseID),
		"title":           strings.TrimSpace(req.Title),
		"type":            materialType,
		"description":     strings.TrimSpace(req.Description),
		"preview_content": strings.TrimSpace(req.PreviewContent),
		"access_level":    accessLevel,
		"status":          status,
	})
	if status == model.StatusPublished {
		fileName, fileSize, err := h.materialPublicationFileMetadata(ctx.Param("id"))
		if err != nil {
			writeMaterialFileValidationError(ctx, err, "update_failed")
			return
		}
		updates["file_name"] = fileName
		updates["file_size"] = fileSize
	}
	h.updateByID(ctx, &model.Material{}, updates, "material")
}

func (h Handler) UpdateMaterialStatus(ctx *gin.Context) {
	var req materialStatusRequest
	if !bindJSON(ctx, &req) {
		return
	}
	status, ok := normalizeMaterialStatus(req.Status, "")
	if !ok || status == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	materialID := ctx.Param("id")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var material model.Material
		if err := tx.First(&material, "id = ?", materialID).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{"status": status}
		if status == model.StatusPublished {
			fileName, fileSize, err := h.validateMaterialStorageReference(material.StorageKey, material.FileName)
			if err != nil {
				return err
			}
			updates["file_name"] = fileName
			updates["file_size"] = fileSize
		}
		result := tx.Model(&model.Material{}).Where("id = ?", materialID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return audit.Record(ctx, tx, "material.status_update", "material", materialID, map[string]interface{}{
			"status": status,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
			return
		}
		writeMaterialFileValidationError(ctx, err, "update_failed")
		return
	}
	response.OK(ctx, gin.H{"updated": true, "status": status})
}

func (h Handler) ApproveMaterial(ctx *gin.Context) {
	h.reviewMaterial(ctx, model.StatusPublished)
}

func (h Handler) RejectMaterial(ctx *gin.Context) {
	h.reviewMaterial(ctx, model.StatusRejected)
}

func (h Handler) reviewMaterial(ctx *gin.Context, status string) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req materialReviewRequest
	if ctx.Request.Body != nil && ctx.Request.ContentLength != 0 {
		if !bindJSON(ctx, &req) {
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
	var material model.Material
	if err := h.db.First(&material, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
		return
	}
	if material.Status != model.StatusPending {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "material_not_reviewable", gin.H{"status": material.Status})
		return
	}
	previousStatus := material.Status
	action := "material.approved"
	if status == model.StatusRejected {
		action = "material.rejected"
	}
	updates := map[string]interface{}{
		"status":        status,
		"reviewer_id":   user.ID,
		"reviewed_at":   gorm.Expr("CURRENT_TIMESTAMP"),
		"review_reason": reason,
	}
	if status == model.StatusPublished {
		fileName, fileSize, err := h.validateMaterialStorageReference(material.StorageKey, material.FileName)
		if err != nil {
			writeMaterialFileValidationError(ctx, err, "review_failed")
			return
		}
		updates["file_name"] = fileName
		updates["file_size"] = fileSize
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Material{}).Where("id = ?", material.ID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if material.CreatedBy != nil {
			if err := notification.CreateReviewNotification(tx, notification.ReviewNotificationInput{
				UserID:        *material.CreatedBy,
				ResourceType:  "material",
				ResourceID:    material.ID,
				ResourceTitle: material.Title,
				Status:        status,
				Reason:        reason,
			}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, action, "material", material.ID, map[string]interface{}{
			"courseId":       material.CourseID,
			"previousStatus": previousStatus,
			"status":         status,
			"reviewReason":   reason,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "review_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"reviewed": true, "status": status, "reviewReason": reason})
}

func (h Handler) ArchiveMaterial(ctx *gin.Context) {
	h.archiveByID(ctx, &model.Material{}, "material")
}

func (h Handler) UploadMaterial(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxUploadBytes+1024)
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "file_too_large", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_file", nil)
		return
	}
	defer file.Close()

	if header.Size <= 0 || header.Size > maxUploadBytes {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "file_too_large", nil)
		return
	}
	originalName, ext, err := safeUploadFileName(header)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	if err := validateUploadContent(file, ext); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}

	courseID := strings.TrimSpace(ctx.PostForm("courseId"))
	title := required(ctx.PostForm("title"))
	if courseID == "" || title == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_required_fields", nil)
		return
	}
	status, ok := normalizeMaterialStatus(ctx.PostForm("status"), model.StatusDraft)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_status", nil)
		return
	}
	materialType, ok := normalizeMaterialType(ctx.PostForm("type"), "other")
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_material_type", nil)
		return
	}
	accessLevel, ok := normalizeAccessLevel(ctx.PostForm("accessLevel"), model.MaterialAccessLoginRequired)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_access_level", nil)
		return
	}
	storageKey := filepath.ToSlash(filepath.Join("materials", courseID, uuid.NewString()+ext))
	targetPath := filepath.Join(h.uploadDir, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}
	defer target.Close()
	written, err := io.Copy(target, io.LimitReader(file, maxUploadBytes+1))
	if err != nil || written > maxUploadBytes {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "upload_failed", nil)
		return
	}

	material := model.Material{
		CourseID:       courseID,
		Title:          title,
		Type:           materialType,
		Description:    strings.TrimSpace(ctx.PostForm("description")),
		StorageKey:     storageKey,
		FileName:       originalName,
		FileSize:       written,
		PreviewContent: strings.TrimSpace(ctx.PostForm("previewContent")),
		AccessLevel:    accessLevel,
		Status:         status,
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&material).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "material.upload", "material", material.ID, map[string]interface{}{
			"courseId":    material.CourseID,
			"type":        material.Type,
			"accessLevel": material.AccessLevel,
			"status":      material.Status,
			"fileName":    material.FileName,
			"fileSize":    material.FileSize,
		})
	}); err != nil {
		_ = os.Remove(targetPath)
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"material": material})
}

func (h Handler) createAccessGrant(ctx *gin.Context, tx *gorm.DB, userID string, materialID string, packageID string, expiresAt *time.Time) (model.MaterialAccessGrant, bool, error) {
	if userID == "" || (materialID == "" && packageID == "") || (materialID != "" && packageID != "") {
		return model.MaterialAccessGrant{}, false, errGrantResourceSelection
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return model.MaterialAccessGrant{}, false, errGrantExpirationInPast
	}

	var user model.User
	if err := tx.First(&user, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.MaterialAccessGrant{}, false, errGrantUserNotFound
		}
		return model.MaterialAccessGrant{}, false, err
	}

	now := time.Now()
	existingQuery := tx.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, now)
	grant := model.MaterialAccessGrant{
		UserID:    userID,
		Source:    "manual_admin",
		ExpiresAt: expiresAt,
	}
	metadata := map[string]interface{}{
		"userId": userID,
		"source": grant.Source,
	}
	if materialID != "" {
		var material model.Material
		if err := tx.First(&material, "id = ?", materialID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.MaterialAccessGrant{}, false, errGrantResourceNotFound
			}
			return model.MaterialAccessGrant{}, false, err
		}
		if material.Status != model.StatusPublished {
			return model.MaterialAccessGrant{}, false, errGrantResourceNotPublished
		}
		if !isGrantableMaterialAccess(material.AccessLevel) {
			return model.MaterialAccessGrant{}, false, errGrantMaterialNotGrantable
		}
		grant.MaterialID = &materialID
		existingQuery = existingQuery.Where("material_id = ?", materialID)
		metadata["materialId"] = materialID
		metadata["accessLevel"] = material.AccessLevel
	} else {
		var coursePackage model.CoursePackage
		if err := tx.First(&coursePackage, "id = ?", packageID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.MaterialAccessGrant{}, false, errGrantResourceNotFound
			}
			return model.MaterialAccessGrant{}, false, err
		}
		if coursePackage.Status != model.StatusPublished {
			return model.MaterialAccessGrant{}, false, errGrantResourceNotPublished
		}
		grant.PackageID = &packageID
		existingQuery = existingQuery.Where("package_id = ?", packageID)
		metadata["packageId"] = packageID
	}

	var existing model.MaterialAccessGrant
	if err := existingQuery.Order("created_at desc").First(&existing).Error; err == nil {
		return existing, true, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.MaterialAccessGrant{}, false, err
	}
	if err := tx.Create(&grant).Error; err != nil {
		return model.MaterialAccessGrant{}, false, err
	}
	if expiresAt != nil {
		metadata["expiresAt"] = expiresAt.UTC().Format(time.RFC3339)
	}
	if err := audit.Record(ctx, tx, "access_grant.create", "access_grant", grant.ID, metadata); err != nil {
		return model.MaterialAccessGrant{}, false, err
	}
	return grant, false, nil
}

func (h Handler) accessGrantRows(grants []model.MaterialAccessGrant, now time.Time) ([]accessGrantRow, error) {
	userIDs := make([]string, 0, len(grants))
	materialIDs := make([]string, 0, len(grants))
	packageIDs := make([]string, 0, len(grants))
	for _, grant := range grants {
		userIDs = append(userIDs, grant.UserID)
		if grant.MaterialID != nil && *grant.MaterialID != "" {
			materialIDs = append(materialIDs, *grant.MaterialID)
		}
		if grant.PackageID != nil && *grant.PackageID != "" {
			packageIDs = append(packageIDs, *grant.PackageID)
		}
	}
	users, err := h.usersByID(userIDs)
	if err != nil {
		return nil, err
	}
	materials, err := h.adminMaterialsByID(materialIDs)
	if err != nil {
		return nil, err
	}
	packages, err := h.adminPackagesByID(packageIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]accessGrantRow, 0, len(grants))
	for _, grant := range grants {
		row := accessGrantRow{
			Grant:  grant,
			Active: grant.ExpiresAt == nil || grant.ExpiresAt.After(now),
		}
		if user, ok := users[grant.UserID]; ok {
			row.User = &user
		}
		if grant.MaterialID != nil {
			if material, ok := materials[*grant.MaterialID]; ok {
				row.Material = &material
			}
		}
		if grant.PackageID != nil {
			if coursePackage, ok := packages[*grant.PackageID]; ok {
				row.Package = &coursePackage
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (h Handler) orderRows(orders []model.Order, now time.Time) ([]orderRow, error) {
	userIDs := make([]string, 0, len(orders))
	packageIDs := make([]string, 0, len(orders))
	for _, order := range orders {
		userIDs = append(userIDs, order.UserID)
		if order.ProductType == "course_package" {
			packageIDs = append(packageIDs, order.ProductID)
		}
	}
	users, err := h.usersByID(userIDs)
	if err != nil {
		return nil, err
	}
	packages, err := h.adminPackagesByID(packageIDs)
	if err != nil {
		return nil, err
	}
	granted, err := h.activePackageGrantsForOrders(orders, now)
	if err != nil {
		return nil, err
	}
	rows := make([]orderRow, 0, len(orders))
	for _, order := range orders {
		row := orderRow{
			Order:              order,
			EntitlementGranted: granted[order.ID],
		}
		if user, ok := users[order.UserID]; ok {
			row.User = &user
		}
		if order.ProductType == "course_package" {
			if coursePackage, ok := packages[order.ProductID]; ok {
				row.Package = &coursePackage
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (h Handler) activePackageGrantsForOrders(orders []model.Order, now time.Time) (map[string]bool, error) {
	result := map[string]bool{}
	pairs := make([]model.Order, 0, len(orders))
	userIDs := make([]string, 0, len(orders))
	packageIDs := make([]string, 0, len(orders))
	for _, order := range orders {
		if order.ProductType == "course_package" {
			pairs = append(pairs, order)
			userIDs = append(userIDs, order.UserID)
			packageIDs = append(packageIDs, order.ProductID)
		}
	}
	if len(pairs) == 0 {
		return result, nil
	}
	var grants []model.MaterialAccessGrant
	if err := h.db.
		Where("package_id IS NOT NULL").
		Where("user_id IN ?", userIDs).
		Where("package_id IN ?", packageIDs).
		Where("expires_at IS NULL OR expires_at > ?", now).
		Find(&grants).Error; err != nil {
		return nil, err
	}
	for _, order := range pairs {
		for _, grant := range grants {
			if grant.PackageID != nil && grant.UserID == order.UserID && *grant.PackageID == order.ProductID {
				result[order.ID] = true
				break
			}
		}
	}
	return result, nil
}

func (h Handler) paymentReconciliationIssues(now time.Time) ([]paymentReconciliationIssue, error) {
	var orders []model.Order
	if err := h.db.Order("created_at desc").Limit(2000).Find(&orders).Error; err != nil {
		return nil, err
	}
	orderIDs := make([]string, 0, len(orders))
	userIDs := make([]string, 0, len(orders))
	packageIDs := make([]string, 0, len(orders))
	ordersByID := map[string]model.Order{}
	for _, order := range orders {
		orderIDs = append(orderIDs, order.ID)
		userIDs = append(userIDs, order.UserID)
		ordersByID[order.ID] = order
		if order.ProductType == "course_package" {
			packageIDs = append(packageIDs, order.ProductID)
		}
	}

	var records []model.PaymentRecord
	if len(orderIDs) > 0 {
		if err := h.db.Where("order_id IN ?", orderIDs).Order("created_at desc").Find(&records).Error; err != nil {
			return nil, err
		}
	}
	var transactionRecords []model.PaymentRecord
	if err := h.db.Where("transaction_id <> ''").Order("created_at desc").Limit(5000).Find(&transactionRecords).Error; err != nil {
		return nil, err
	}
	records = mergePaymentRecords(records, transactionRecords)
	recordsByOrder := map[string][]model.PaymentRecord{}
	recordsByTransaction := map[string][]model.PaymentRecord{}
	for _, record := range records {
		recordsByOrder[record.OrderID] = append(recordsByOrder[record.OrderID], record)
		if strings.TrimSpace(record.TransactionID) != "" {
			recordsByTransaction[record.TransactionID] = append(recordsByTransaction[record.TransactionID], record)
		}
	}

	var grants []model.MaterialAccessGrant
	if err := h.db.Where("source = ? AND order_id IS NOT NULL", "order").Order("created_at desc").Limit(5000).Find(&grants).Error; err != nil {
		return nil, err
	}
	grantsByOrder := map[string][]model.MaterialAccessGrant{}
	for _, grant := range grants {
		if grant.OrderID == nil {
			continue
		}
		grantsByOrder[*grant.OrderID] = append(grantsByOrder[*grant.OrderID], grant)
		userIDs = append(userIDs, grant.UserID)
		if grant.PackageID != nil {
			packageIDs = append(packageIDs, *grant.PackageID)
		}
	}

	var incidents []model.PaymentIncident
	if err := h.db.Where("status = ?", model.PaymentIncidentOpen).Order("created_at desc").Limit(1000).Find(&incidents).Error; err != nil {
		return nil, err
	}
	for _, incident := range incidents {
		if incident.OrderID != nil {
			if order, ok := ordersByID[*incident.OrderID]; ok {
				userIDs = append(userIDs, order.UserID)
				if order.ProductType == "course_package" {
					packageIDs = append(packageIDs, order.ProductID)
				}
			}
		}
	}

	users, err := h.usersByID(uniqueStrings(userIDs))
	if err != nil {
		return nil, err
	}
	packages, err := h.adminPackagesByID(uniqueStrings(packageIDs))
	if err != nil {
		return nil, err
	}

	issues := make([]paymentReconciliationIssue, 0)
	for _, order := range orders {
		if strings.TrimSpace(order.RiskFlag) != "" {
			issues = append(issues, h.orderPaymentIssue("order_risk_flag", "high", "order carries a payment risk flag", order, users, packages, nil, nil, nil))
		}
		if order.Status != model.OrderPaid {
			continue
		}
		orderRecords := recordsByOrder[order.ID]
		if len(orderRecords) == 0 {
			issues = append(issues, h.orderPaymentIssue("paid_order_missing_payment_record", "critical", "paid order has no payment record", order, users, packages, nil, nil, nil))
		}
		for _, record := range orderRecords {
			if record.AmountTotal != 0 && record.AmountTotal != order.AmountTotal {
				recordCopy := record
				issues = append(issues, h.orderPaymentIssue("paid_order_amount_record_mismatch", "critical", "paid order amount does not match payment record amount", order, users, packages, &recordCopy, nil, nil))
			}
		}
		if order.ProductType == "course_package" && !hasActivePackageOrderGrant(grantsByOrder[order.ID], order, now) {
			issues = append(issues, h.orderPaymentIssue("paid_order_missing_entitlement", "critical", "paid course package order has no active order entitlement", order, users, packages, nil, nil, nil))
		}
	}

	for _, grant := range grants {
		if grant.OrderID == nil || !grantActive(grant, now) {
			continue
		}
		order, ok := ordersByID[*grant.OrderID]
		grantCopy := grant
		if !ok {
			issues = append(issues, h.grantPaymentIssue("order_entitlement_missing_order", "critical", "active order entitlement references a missing order", grantCopy, users, packages))
			continue
		}
		if order.Status != model.OrderPaid {
			issues = append(issues, h.orderPaymentIssue("unpaid_order_has_entitlement", "critical", "non-paid order has an active order entitlement", order, users, packages, nil, &grantCopy, nil))
		}
	}

	for transactionID, rows := range recordsByTransaction {
		orderSet := map[string]bool{}
		for _, row := range rows {
			orderSet[row.OrderID] = true
		}
		if len(orderSet) <= 1 {
			continue
		}
		first := rows[0]
		issue := paymentReconciliationIssue{
			IssueType:       "duplicate_transaction_id",
			Severity:        "critical",
			Message:         "same payment transaction id is attached to multiple orders",
			PaymentRecordID: first.ID,
			TransactionID:   transactionID,
			CreatedAt:       first.CreatedAt.Format(time.RFC3339),
		}
		if order, ok := ordersByID[first.OrderID]; ok {
			issue = h.orderPaymentIssue(issue.IssueType, issue.Severity, issue.Message, order, users, packages, &first, nil, nil)
		}
		issues = append(issues, issue)
	}

	for _, incident := range incidents {
		incidentCopy := incident
		issue := paymentReconciliationIssue{
			IssueType:       "open_payment_incident",
			Severity:        incident.Severity,
			Message:         incident.Message,
			IncidentID:      incident.ID,
			OutTradeNo:      incident.OutTradeNo,
			TransactionID:   incident.TransactionID,
			AmountTotal:     incident.ActualAmount,
			CreatedAt:       incident.CreatedAt.Format(time.RFC3339),
			PaymentProvider: incident.Provider,
		}
		if issue.Severity == "" {
			issue.Severity = "high"
		}
		if incident.OrderID != nil {
			if order, ok := ordersByID[*incident.OrderID]; ok {
				issue = h.orderPaymentIssue(issue.IssueType, issue.Severity, issue.Message, order, users, packages, nil, nil, &incidentCopy)
			} else {
				issue.OrderID = *incident.OrderID
			}
		}
		issues = append(issues, issue)
	}

	return issues, nil
}

func (h Handler) paymentIncidentAlertSummaries(incidents []model.PaymentIncident) (map[string]paymentIncidentAlertSummary, error) {
	ids := make([]string, 0, len(incidents))
	for _, incident := range incidents {
		if strings.TrimSpace(incident.ID) != "" {
			ids = append(ids, incident.ID)
		}
	}
	summaries := map[string]paymentIncidentAlertSummary{}
	if len(ids) == 0 {
		return summaries, nil
	}
	var logs []model.OperationLog
	if err := h.db.
		Where("target_type = ? AND action = ? AND target_id IN ?", "payment_incident", "payment_incident.alert", ids).
		Order("created_at desc").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	for _, log := range logs {
		summary := summaries[log.TargetID]
		summary.AlertCount++
		if summary.LastAlertedAt == nil {
			alertedAt := log.CreatedAt
			summary.LastAlertedAt = &alertedAt
			if strings.TrimSpace(log.OperatorID) != "" {
				operatorID := log.OperatorID
				summary.LastAlertedBy = &operatorID
			}
		}
		summaries[log.TargetID] = summary
	}
	return summaries, nil
}

func paymentIncidentRows(incidents []model.PaymentIncident, alerts map[string]paymentIncidentAlertSummary) []paymentIncidentRow {
	rows := make([]paymentIncidentRow, 0, len(incidents))
	for _, incident := range incidents {
		alert := alerts[incident.ID]
		rows = append(rows, paymentIncidentRow{
			ID:             incident.ID,
			OrderID:        incident.OrderID,
			Provider:       incident.Provider,
			IncidentType:   incident.IncidentType,
			Severity:       incident.Severity,
			Status:         incident.Status,
			OutTradeNo:     incident.OutTradeNo,
			TransactionID:  incident.TransactionID,
			TradeState:     incident.TradeState,
			ExpectedAmount: incident.ExpectedAmount,
			ActualAmount:   incident.ActualAmount,
			Message:        incident.Message,
			HandledBy:      incident.HandledBy,
			HandledAt:      incident.HandledAt,
			HandleNote:     incident.HandleNote,
			AlertCount:     alert.AlertCount,
			LastAlertedAt:  alert.LastAlertedAt,
			LastAlertedBy:  alert.LastAlertedBy,
			CreatedAt:      incident.CreatedAt,
			UpdatedAt:      incident.UpdatedAt,
		})
	}
	return rows
}

func (h Handler) orderPaymentIssue(issueType string, severity string, message string, order model.Order, users map[string]model.User, packages map[string]model.CoursePackage, record *model.PaymentRecord, grant *model.MaterialAccessGrant, incident *model.PaymentIncident) paymentReconciliationIssue {
	issue := paymentReconciliationIssue{
		IssueType:       issueType,
		Severity:        severity,
		Message:         message,
		OrderID:         order.ID,
		OutTradeNo:      order.OutTradeNo,
		OrderStatus:     order.Status,
		PaymentProvider: order.PaymentProvider,
		AmountTotal:     order.AmountTotal,
		RiskFlag:        order.RiskFlag,
		UserID:          order.UserID,
		CreatedAt:       order.CreatedAt.Format(time.RFC3339),
	}
	if user, ok := users[order.UserID]; ok {
		issue.UserEmail = user.Email
	}
	if order.ProductType == "course_package" {
		issue.PackageID = order.ProductID
		if coursePackage, ok := packages[order.ProductID]; ok {
			issue.PackageTitle = coursePackage.Title
		}
	}
	if record != nil {
		issue.PaymentRecordID = record.ID
		issue.TransactionID = record.TransactionID
		if record.AmountTotal != 0 {
			issue.AmountTotal = record.AmountTotal
		}
		if record.CreatedAt.After(order.CreatedAt) {
			issue.CreatedAt = record.CreatedAt.Format(time.RFC3339)
		}
	}
	if grant != nil {
		issue.GrantID = grant.ID
		if grant.CreatedAt.After(order.CreatedAt) {
			issue.CreatedAt = grant.CreatedAt.Format(time.RFC3339)
		}
	}
	if incident != nil {
		issue.IncidentID = incident.ID
		if incident.TransactionID != "" {
			issue.TransactionID = incident.TransactionID
		}
		if incident.ActualAmount != 0 {
			issue.AmountTotal = incident.ActualAmount
		}
		if incident.CreatedAt.After(order.CreatedAt) {
			issue.CreatedAt = incident.CreatedAt.Format(time.RFC3339)
		}
	}
	return issue
}

func (h Handler) grantPaymentIssue(issueType string, severity string, message string, grant model.MaterialAccessGrant, users map[string]model.User, packages map[string]model.CoursePackage) paymentReconciliationIssue {
	issue := paymentReconciliationIssue{
		IssueType: issueType,
		Severity:  severity,
		Message:   message,
		GrantID:   grant.ID,
		UserID:    grant.UserID,
		CreatedAt: grant.CreatedAt.Format(time.RFC3339),
	}
	if grant.OrderID != nil {
		issue.OrderID = *grant.OrderID
	}
	if user, ok := users[grant.UserID]; ok {
		issue.UserEmail = user.Email
	}
	if grant.PackageID != nil {
		issue.PackageID = *grant.PackageID
		if coursePackage, ok := packages[*grant.PackageID]; ok {
			issue.PackageTitle = coursePackage.Title
		}
	}
	return issue
}

func hasActivePackageOrderGrant(grants []model.MaterialAccessGrant, order model.Order, now time.Time) bool {
	for _, grant := range grants {
		if grant.PackageID == nil || *grant.PackageID != order.ProductID || !grantActive(grant, now) {
			continue
		}
		return true
	}
	return false
}

func grantActive(grant model.MaterialAccessGrant, now time.Time) bool {
	return grant.ExpiresAt == nil || grant.ExpiresAt.After(now)
}

func mergePaymentRecords(primary []model.PaymentRecord, extra []model.PaymentRecord) []model.PaymentRecord {
	seen := map[string]bool{}
	merged := make([]model.PaymentRecord, 0, len(primary)+len(extra))
	for _, record := range primary {
		seen[record.ID] = true
		merged = append(merged, record)
	}
	for _, record := range extra {
		if seen[record.ID] {
			continue
		}
		merged = append(merged, record)
	}
	return merged
}

func paymentReconciliationSummaryFor(issues []paymentReconciliationIssue) paymentReconciliationSummary {
	summary := paymentReconciliationSummary{Total: len(issues), Types: map[string]int{}}
	for _, issue := range issues {
		summary.Types[issue.IssueType]++
		switch issue.Severity {
		case "critical":
			summary.Critical++
		case "high":
			summary.High++
		case "medium":
			summary.Medium++
		case "low":
			summary.Low++
		}
	}
	return summary
}

func paymentIssueRank(issue paymentReconciliationIssue) int {
	severityRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
	rank, ok := severityRank[issue.Severity]
	if !ok {
		rank = 4
	}
	return rank*100 + paymentIssueTypeRank(issue.IssueType)
}

func paymentIssueTypeRank(issueType string) int {
	switch issueType {
	case "paid_order_missing_payment_record":
		return 1
	case "paid_order_missing_entitlement":
		return 2
	case "unpaid_order_has_entitlement":
		return 3
	case "order_entitlement_missing_order":
		return 4
	case "paid_order_amount_record_mismatch":
		return 5
	case "duplicate_transaction_id":
		return 6
	case "open_payment_incident":
		return 7
	case "order_risk_flag":
		return 8
	default:
		return 99
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (h Handler) usersByID(ids []string) (map[string]model.User, error) {
	rows := map[string]model.User{}
	if len(ids) == 0 {
		return rows, nil
	}
	var users []model.User
	if err := h.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		rows[user.ID] = user
	}
	return rows, nil
}

func (h Handler) adminMaterialsByID(ids []string) (map[string]model.Material, error) {
	rows := map[string]model.Material{}
	if len(ids) == 0 {
		return rows, nil
	}
	var materials []model.Material
	if err := h.db.Where("id IN ?", ids).Find(&materials).Error; err != nil {
		return nil, err
	}
	for _, material := range materials {
		rows[material.ID] = material
	}
	return rows, nil
}

func (h Handler) adminPackagesByID(ids []string) (map[string]model.CoursePackage, error) {
	rows := map[string]model.CoursePackage{}
	if len(ids) == 0 {
		return rows, nil
	}
	var packages []model.CoursePackage
	if err := h.db.Where("id IN ?", ids).Find(&packages).Error; err != nil {
		return nil, err
	}
	for _, coursePackage := range packages {
		rows[coursePackage.ID] = coursePackage
	}
	return rows, nil
}

func (h Handler) packageItemRows(items []model.CoursePackageItem) ([]packageItemRow, error) {
	materialIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.ResourceType == "material" {
			materialIDs = append(materialIDs, item.ResourceID)
		}
	}
	materials, err := h.adminMaterialsByID(materialIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]packageItemRow, 0, len(items))
	for _, item := range items {
		row := packageItemRow{Item: item}
		if item.ResourceType == "material" {
			if material, ok := materials[item.ResourceID]; ok {
				row.Material = &material
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (h Handler) updateByID(ctx *gin.Context, target interface{}, updates map[string]interface{}, name string) {
	if len(updates) == 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "empty_update", nil)
		return
	}
	targetID := ctx.Param("id")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(target).Where("id = ?", targetID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return audit.Record(ctx, tx, name+".update", name, targetID, updateMetadata(updates))
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, name+"_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "update_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"updated": true})
}

func (h Handler) archiveByID(ctx *gin.Context, target interface{}, name string) {
	targetID := ctx.Param("id")
	err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(target).Where("id = ?", targetID).Update("status", model.StatusArchived)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return audit.Record(ctx, tx, name+".archive", name, targetID, map[string]interface{}{
			"status": model.StatusArchived,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, name+"_not_found", nil)
			return
		}
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "archive_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"archived": true})
}

func validateCoursePackageReferences(tx *gorm.DB, coursePackage model.CoursePackage) error {
	var school model.School
	if err := tx.First(&school, "id = ?", coursePackage.SchoolID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errPackageReferenceNotFound
		}
		return err
	}
	var college model.College
	if err := tx.First(&college, "id = ?", coursePackage.CollegeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errPackageReferenceNotFound
		}
		return err
	}
	if college.SchoolID != coursePackage.SchoolID {
		return errPackageReferenceMismatch
	}
	var major model.Major
	if err := tx.First(&major, "id = ?", coursePackage.MajorID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errPackageReferenceNotFound
		}
		return err
	}
	if major.SchoolID != coursePackage.SchoolID || major.CollegeID != coursePackage.CollegeID {
		return errPackageReferenceMismatch
	}
	if coursePackage.CourseID == nil || *coursePackage.CourseID == "" {
		return nil
	}
	var course model.Course
	if err := tx.First(&course, "id = ?", *coursePackage.CourseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errPackageReferenceNotFound
		}
		return err
	}
	if course.SchoolID != coursePackage.SchoolID || course.CollegeID != coursePackage.CollegeID || course.MajorID != coursePackage.MajorID || course.Grade != coursePackage.Grade {
		return errPackageReferenceMismatch
	}
	return nil
}

func validatePackageMaterialScope(tx *gorm.DB, coursePackage model.CoursePackage, material model.Material) error {
	if coursePackage.CourseID != nil && *coursePackage.CourseID != "" && material.CourseID != *coursePackage.CourseID {
		return errPackageReferenceMismatch
	}
	var course model.Course
	if err := tx.First(&course, "id = ?", material.CourseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errPackageReferenceNotFound
		}
		return err
	}
	if course.SchoolID != coursePackage.SchoolID || course.CollegeID != coursePackage.CollegeID || course.MajorID != coursePackage.MajorID || course.Grade != coursePackage.Grade {
		return errPackageReferenceMismatch
	}
	return nil
}

func writeCoursePackageError(ctx *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "package_not_found", nil)
	case errors.Is(err, errPackageReferenceNotFound):
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "package_reference_not_found", nil)
	case errors.Is(err, errPackageReferenceMismatch):
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "package_reference_mismatch", nil)
	case errors.Is(err, errGrantResourceNotFound):
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "package_item_resource_not_found", nil)
	case errors.Is(err, errPackageItemUnsupported):
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsupported_resource_type", nil)
	default:
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, fallback, nil)
	}
}

func bindJSON(ctx *gin.Context, target interface{}) bool {
	if err := ctx.ShouldBindJSON(target); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return false
	}
	return true
}

func rejectMaterialFileFieldUpdates(ctx *gin.Context) bool {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return false
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return false
	}
	for _, field := range []string{"storageKey", "storage_key", "fileName", "file_name", "fileSize", "file_size"} {
		if _, ok := raw[field]; ok {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "material_file_fields_immutable", nil)
			return false
		}
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	return true
}

func compactMap(values map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, value := range values {
		switch typed := value.(type) {
		case string:
			if typed != "" {
				result[key] = typed
			}
		case int64:
			if typed > 0 {
				result[key] = typed
			}
		default:
			if value != nil {
				result[key] = value
			}
		}
	}
	return result
}

func applyStringUpdate(updates map[string]interface{}, key string, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		updates[key] = trimmed
	}
}

func nullableTrimmedString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func updateMetadata(updates map[string]interface{}) map[string]interface{} {
	if len(updates) == 0 {
		return nil
	}
	fields := sortedKeys(updates)
	metadata := map[string]interface{}{
		"fields": fields,
	}
	for _, key := range []string{"status", "access_level", "type"} {
		if value, ok := updates[key]; ok {
			metadata[key] = value
		}
	}
	return metadata
}

func sortedKeys(values map[string]interface{}) []string {
	fields := make([]string, 0, len(values))
	for key := range values {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
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

func parseOptionalBool(raw string) (bool, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return false, true
	}
	switch value {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func parseLogTime(raw string, endOfDay bool) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		if endOfDay {
			parsed = parsed.Add(24*time.Hour - time.Nanosecond)
		}
		return parsed, true
	}
	return time.Time{}, false
}

func parseGrantExpiresAt(raw string) (*time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, true
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, true
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		endOfDay := parsed.Add(24*time.Hour - time.Nanosecond)
		return &endOfDay, true
	}
	return nil, false
}

func validMediaAssetStatus(value string) bool {
	switch value {
	case "uploaded", "attached", model.StatusArchived:
		return true
	default:
		return false
	}
}

func (h Handler) mediaAssetRows(assets []model.MediaAsset) ([]mediaAssetRow, error) {
	ownerIDs := make([]string, 0, len(assets))
	for _, asset := range assets {
		ownerIDs = append(ownerIDs, asset.OwnerID)
	}
	owners, err := h.usersByID(ownerIDs)
	if err != nil {
		return nil, err
	}
	rows := make([]mediaAssetRow, 0, len(assets))
	for _, asset := range assets {
		row := mediaAssetRow{Asset: asset}
		if owner, ok := owners[asset.OwnerID]; ok {
			row.Owner = &owner
		}
		if path, err := adminSafeStoragePath(h.uploadDir, asset.StorageKey); err == nil {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				row.HasFile = true
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func adminSafeStoragePath(uploadDir string, storageKey string) (string, error) {
	normalizedKey := filepath.ToSlash(strings.TrimSpace(storageKey))
	if normalizedKey == "" || strings.Contains(normalizedKey, `\`) {
		return "", errors.New("unsafe_path")
	}
	for _, part := range strings.Split(normalizedKey, "/") {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("unsafe_path")
		}
	}
	root, err := filepath.Abs(uploadDir)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(normalizedKey)))
	if err != nil {
		return "", err
	}
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return "", errors.New("unsafe_path")
	}
	return target, nil
}

func jsonString(value interface{}) string {
	if value == nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func safeCSVCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	trimmed := strings.TrimLeft(value, " \t")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func required(value string) string {
	return strings.TrimSpace(value)
}

func defaultStatus(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return model.StatusPublished
	}
	return value
}

func normalizeMaterialType(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case "knowledge_note", "mock_paper", "answer", "quick_review", "past_exam", "other":
		return value, true
	default:
		return "", false
	}
}

func normalizeAccessLevel(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.MaterialAccessFree, model.MaterialAccessLoginRequired, model.MaterialAccessPaid, model.MaterialAccessMemberOnly:
		return value, true
	default:
		return "", false
	}
}

func normalizeMaterialStatus(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.StatusDraft, model.StatusPending, model.StatusPublished, model.StatusRejected, model.StatusArchived:
		return value, true
	default:
		return "", false
	}
}

func isMaterialReviewListStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case model.StatusPending, model.StatusPublished, model.StatusRejected:
		return true
	default:
		return false
	}
}

func normalizeCourseStatus(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.StatusDraft, model.StatusPublished, model.StatusArchived:
		return value, true
	default:
		return "", false
	}
}

func validOrderStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case model.OrderPending, model.OrderPaying, model.OrderPaid, model.OrderClosed, model.OrderExpired, model.OrderFailed, model.OrderCancelled, model.OrderRefunded:
		return true
	default:
		return false
	}
}

func validPaymentIncidentStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case model.PaymentIncidentOpen, model.PaymentIncidentResolved, model.PaymentIncidentIgnored:
		return true
	default:
		return false
	}
}

func validPaymentReconciliationSeverity(value string) bool {
	switch strings.TrimSpace(value) {
	case "critical", "high", "medium", "low":
		return true
	default:
		return false
	}
}

func normalizeUserRole(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case model.RoleUser, model.RoleCreator, model.RoleReviewer, model.RoleOperator, model.RoleAdmin, model.RoleSuperAdmin:
		return value, true
	default:
		return "", false
	}
}

func normalizeUserStatus(value string, fallback string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, true
	}
	switch value {
	case "active", "frozen":
		return value, true
	default:
		return "", false
	}
}

func isGrantableMaterialAccess(accessLevel string) bool {
	switch accessLevel {
	case model.MaterialAccessPaid, model.MaterialAccessMemberOnly:
		return true
	default:
		return false
	}
}

func safeUploadFileName(header *multipart.FileHeader) (string, string, error) {
	name := strings.TrimSpace(header.Filename)
	if name == "" || strings.ContainsAny(name, `/\`) || name != filepath.Base(name) {
		return "", "", errUnsafeFileName
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedUploadExtensions[ext] {
		return "", "", errUnsupportedFileType
	}
	return name, ext, nil
}

func validateUploadContent(file multipart.File, ext string) error {
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return errInvalidFile
	}
	content := buffer[:n]
	if ext == ".pdf" && !bytes.HasPrefix(content, []byte("%PDF")) {
		return errInvalidFileContent
	}
	if ext == ".txt" || ext == ".md" {
		if bytes.Contains(content, []byte{0}) {
			return errInvalidFileContent
		}
	}
	if ext == ".docx" &&
		!bytes.HasPrefix(content, []byte("PK\x03\x04")) &&
		!bytes.HasPrefix(content, []byte("PK\x05\x06")) &&
		!bytes.HasPrefix(content, []byte("PK\x07\x08")) {
		return errInvalidFileContent
	}
	return nil
}

func (h Handler) validateMaterialStorageReference(storageKey string, displayName string) (string, int64, error) {
	path, err := adminSafeStoragePath(h.uploadDir, storageKey)
	if err != nil {
		return "", 0, errUnsafeStorageKey
	}
	ext := strings.ToLower(filepath.Ext(storageKey))
	if !allowedUploadExtensions[ext] {
		return "", 0, errUnsupportedFileType
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0, errFileNotFound
		}
		return "", 0, err
	}
	if info.IsDir() {
		return "", 0, errFileNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	if err := validateUploadContent(file, ext); err != nil {
		return "", 0, err
	}
	fileName := strings.TrimSpace(displayName)
	if fileName == "" {
		fileName = filepath.Base(storageKey)
	}
	if fileName == "" || strings.ContainsAny(fileName, `/\`) || fileName != filepath.Base(fileName) {
		return "", 0, errUnsafeFileName
	}
	if strings.ToLower(filepath.Ext(fileName)) != ext {
		return "", 0, errUnsupportedFileType
	}
	return fileName, info.Size(), nil
}

func (h Handler) materialPublicationFileMetadata(materialID string) (string, int64, error) {
	var material model.Material
	if err := h.db.First(&material, "id = ?", materialID).Error; err != nil {
		return "", 0, err
	}
	return h.validateMaterialStorageReference(material.StorageKey, material.FileName)
}

func writeMaterialFileValidationError(ctx *gin.Context, err error, fallback string) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
		return
	}
	for _, sentinel := range []error{errUnsafeStorageKey, errUnsupportedFileType, errFileNotFound, errInvalidFile, errInvalidFileContent, errUnsafeFileName} {
		if errors.Is(err, sentinel) {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, sentinel.Error(), nil)
			return
		}
	}
	response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, fallback, nil)
}

func hasUnsafePath(value string) bool {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(value)))
	return clean == "" || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
