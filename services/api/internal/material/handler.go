package material

import (
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	db        *gorm.DB
	uploadDir string
}

func NewHandler(db *gorm.DB, uploadDir string) Handler {
	if strings.TrimSpace(uploadDir) == "" {
		uploadDir = "uploads"
	}
	return Handler{db: db, uploadDir: uploadDir}
}

func (h Handler) List(ctx *gin.Context) {
	query := h.db.Where("status = ?", model.StatusPublished)
	if courseID := ctx.Query("courseId"); courseID != "" {
		query = query.Where("course_id = ?", courseID)
	}

	var materials []model.Material
	if err := query.Order("created_at asc").Find(&materials).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"materials": materials})
}

func (h Handler) Detail(ctx *gin.Context) {
	material, ok := h.findPublished(ctx)
	if !ok {
		return
	}
	response.OK(ctx, gin.H{"material": material})
}

func (h Handler) Download(ctx *gin.Context) {
	material, ok := h.findPublished(ctx)
	if !ok {
		return
	}

	user, hasUser := auth.CurrentUser(ctx)
	if err := h.canDownload(material, user, hasUser); err != nil {
		writeAccessError(ctx, err)
		return
	}

	path, err := safeStoragePath(h.uploadDir, material.StorageKey)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "file_not_found", nil)
		return
	}
	if _, err := os.Stat(path); err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "file_not_found", nil)
		return
	}

	fileName := strings.TrimSpace(material.FileName)
	if fileName == "" {
		fileName = filepath.Base(path)
	}
	ctx.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": fileName}))
	ctx.File(path)
}

func (h Handler) findPublished(ctx *gin.Context) (model.Material, bool) {
	var material model.Material
	if err := h.db.First(&material, "id = ? AND status = ?", ctx.Param("id"), model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "material_not_found", nil)
		return material, false
	}
	return material, true
}

func (h Handler) canDownload(material model.Material, user *model.User, hasUser bool) error {
	switch material.AccessLevel {
	case model.MaterialAccessFree:
		return nil
	case model.MaterialAccessLoginRequired:
		if !hasUser {
			return errLoginRequired
		}
		if !user.EmailVerified {
			return errEmailNotVerified
		}
		return nil
	case model.MaterialAccessPaid:
		if !hasUser {
			return errLoginRequired
		}
		if !user.EmailVerified {
			return errEmailNotVerified
		}
		if h.hasMaterialGrant(user.ID, material.ID) || h.hasPackageMaterialGrant(user.ID, material.ID) {
			return nil
		}
		return errEntitlementRequired
	case model.MaterialAccessMemberOnly:
		if !hasUser {
			return errLoginRequired
		}
		if !user.EmailVerified {
			return errEmailNotVerified
		}
		if h.hasActiveMembership(user.ID) || h.hasMaterialGrant(user.ID, material.ID) || h.hasPackageMaterialGrant(user.ID, material.ID) {
			return nil
		}
		return errEntitlementRequired
	default:
		return errEntitlementRequired
	}
}

func (h Handler) hasMaterialGrant(userID string, materialID string) bool {
	now := time.Now()
	var count int64
	h.db.Model(&model.MaterialAccessGrant{}).
		Where("user_id = ? AND material_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, materialID, now).
		Count(&count)
	return count > 0
}

func (h Handler) hasPackageMaterialGrant(userID string, materialID string) bool {
	now := time.Now()
	var count int64
	h.db.Model(&model.MaterialAccessGrant{}).
		Joins("JOIN course_package_items ON course_package_items.package_id = material_access_grants.package_id").
		Joins("JOIN course_packages ON course_packages.id = material_access_grants.package_id").
		Where("material_access_grants.user_id = ?", userID).
		Where("course_package_items.resource_type = ? AND course_package_items.resource_id = ?", "material", materialID).
		Where("course_packages.status = ?", model.StatusPublished).
		Where("course_package_items.deleted_at IS NULL AND course_packages.deleted_at IS NULL").
		Where("material_access_grants.expires_at IS NULL OR material_access_grants.expires_at > ?", now).
		Count(&count)
	return count > 0
}

func (h Handler) hasActiveMembership(userID string) bool {
	now := time.Now()
	var count int64
	h.db.Model(&model.Membership{}).
		Where("user_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", userID, "active", now).
		Count(&count)
	return count > 0
}

func safeStoragePath(uploadDir string, storageKey string) (string, error) {
	key := strings.TrimSpace(storageKey)
	if key == "" {
		return "", errors.New("empty storage key")
	}
	cleanKey := filepath.Clean(filepath.FromSlash(key))
	if filepath.IsAbs(cleanKey) || cleanKey == "." || cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(filepath.Separator)) {
		return "", errors.New("unsafe storage key")
	}
	return filepath.Join(uploadDir, cleanKey), nil
}

var (
	errLoginRequired       = errors.New("login_required")
	errEmailNotVerified    = errors.New("email_not_verified")
	errEntitlementRequired = errors.New("entitlement_required")
)

func writeAccessError(ctx *gin.Context, err error) {
	switch err {
	case errLoginRequired:
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "login_required", nil)
	case errEmailNotVerified:
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "email_not_verified", nil)
	case errEntitlementRequired:
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "entitlement_required", nil)
	default:
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
	}
}
