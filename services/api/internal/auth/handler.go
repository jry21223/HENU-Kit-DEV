package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/response"
)

type Handler struct {
	cfg    config.Config
	db     *gorm.DB
	tokens *TokenManager
}

func NewHandler(cfg config.Config, db *gorm.DB, tokens *TokenManager) Handler {
	return Handler{cfg: cfg, db: db, tokens: tokens}
}

type sendCodeRequest struct {
	Email string `json:"email"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	SchoolID string `json:"schoolId"`
	MajorID  string `json:"majorId"`
	Grade    string `json:"grade"`
}

type updateMeRequest struct {
	Name     string `json:"name"`
	SchoolID string `json:"schoolId"`
	MajorID  string `json:"majorId"`
	Grade    string `json:"grade"`
}

func (h Handler) SendCode(ctx *gin.Context) {
	var req sendCodeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	email := normalizeEmail(req.Email)
	if !validEmail(email) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_email", nil)
		return
	}
	if !h.emailDomainAllowed(email) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "email_domain_not_allowed", nil)
		return
	}

	code, err := h.generateCode()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "code_generation_failed", nil)
		return
	}
	record := model.EmailVerificationCode{
		Email:     email,
		CodeHash:  hashCode(email, code),
		Purpose:   "login",
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if err := h.db.Create(&record).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "send_code_failed", nil)
		return
	}

	data := gin.H{"expiresInSeconds": 600}
	if h.cfg.Environment != "production" {
		data["devCode"] = code
	}
	response.OK(ctx, data)
}

func (h Handler) Login(ctx *gin.Context) {
	var req loginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	email := normalizeEmail(req.Email)
	code := strings.TrimSpace(req.Code)
	if !validEmail(email) || code == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_credentials", nil)
		return
	}

	var verification model.EmailVerificationCode
	err := h.db.Where("email = ? AND purpose = ? AND used_at IS NULL", email, "login").
		Order("created_at desc").
		First(&verification).Error
	if err != nil || verification.ExpiresAt.Before(time.Now()) || verification.CodeHash != hashCode(email, code) {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_or_expired_code", nil)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}

	now := time.Now()
	user := model.User{}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("email = ?", email).First(&user).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				return err
			}
			user = model.User{
				Email:         email,
				Name:          name,
				Role:          model.RoleUser,
				Status:        "active",
				SchoolID:      nullableUUID(req.SchoolID),
				MajorID:       nullableUUID(req.MajorID),
				Grade:         strings.TrimSpace(req.Grade),
				EmailVerified: true,
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		} else {
			updates := map[string]interface{}{
				"name":           name,
				"email_verified": true,
				"school_id":      nullableUUID(req.SchoolID),
				"major_id":       nullableUUID(req.MajorID),
				"grade":          strings.TrimSpace(req.Grade),
			}
			if err := tx.Model(&user).Updates(updates).Error; err != nil {
				return err
			}
			if err := tx.First(&user, "email = ?", email).Error; err != nil {
				return err
			}
		}
		return tx.Model(&verification).Update("used_at", now).Error
	})
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "login_failed", nil)
		return
	}

	h.issueSession(ctx, user)
}

func (h Handler) Refresh(ctx *gin.Context) {
	refreshToken, err := ctx.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	claims, err := h.tokens.Parse(refreshToken, TokenTypeRefresh)
	if err != nil {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var user model.User
	if err := h.db.First(&user, "id = ?", claims.UserID).Error; err != nil {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	if claims.TokenVersion != user.TokenVersion {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "session_revoked", nil)
		return
	}
	h.issueSession(ctx, user)
}

func (h Handler) Logout(ctx *gin.Context) {
	clearCookie(ctx, "access_token")
	clearCookie(ctx, "refresh_token")
	response.OK(ctx, gin.H{"ok": true})
}

func (h Handler) Me(ctx *gin.Context) {
	user, ok := CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	response.OK(ctx, publicUser(*user))
}

func (h Handler) UpdateMe(ctx *gin.Context) {
	user, ok := CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	if user.Status != "active" {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "user_frozen", nil)
		return
	}

	var req updateMeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}

	schoolID, majorID, valid := h.validateProfileBinding(ctx, req.SchoolID, req.MajorID)
	if !valid {
		return
	}

	updates := map[string]interface{}{
		"school_id": schoolID,
		"major_id":  majorID,
		"grade":     strings.TrimSpace(req.Grade),
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		updates["name"] = name
	}

	if err := h.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "update_failed", nil)
		return
	}
	var updated model.User
	if err := h.db.First(&updated, "id = ?", user.ID).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, publicUser(updated))
}

func (h Handler) issueSession(ctx *gin.Context, user model.User) {
	accessToken, accessExpiresAt, err := h.tokens.Issue(user.ID, user.Email, user.Role, TokenTypeAccess, user.TokenVersion)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "token_issue_failed", nil)
		return
	}
	refreshToken, refreshExpiresAt, err := h.tokens.Issue(user.ID, user.Email, user.Role, TokenTypeRefresh, user.TokenVersion)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "token_issue_failed", nil)
		return
	}
	setCookie(ctx, "access_token", accessToken, accessExpiresAt)
	setCookie(ctx, "refresh_token", refreshToken, refreshExpiresAt)
	response.OK(ctx, gin.H{
		"user":        publicUser(user),
		"tokenType":   "Bearer",
		"accessToken": accessToken,
		"expiresAt":   accessExpiresAt,
	})
}

func (h Handler) generateCode() (string, error) {
	if h.cfg.Environment != "production" && h.cfg.DevFixedCode != "" {
		return h.cfg.DevFixedCode, nil
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (h Handler) emailDomainAllowed(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := strings.ToLower(parts[1])

	var schools []model.School
	if err := h.db.Where("status <> ?", model.StatusArchived).Find(&schools).Error; err != nil {
		return false
	}
	if len(schools) == 0 && h.cfg.Environment != "production" {
		return domain == "henu.edu.cn" || domain == "stu.henu.edu.cn"
	}
	for _, school := range schools {
		for _, allowed := range strings.Split(school.EmailDomains, ",") {
			if strings.EqualFold(strings.TrimSpace(allowed), domain) {
				return true
			}
		}
	}
	return false
}

func hashCode(email string, code string) string {
	sum := sha256.Sum256([]byte(normalizeEmail(email) + ":" + strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	return strings.Contains(email, "@") && strings.Contains(strings.Split(email, "@")[1], ".")
}

func nullableUUID(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil
	}
	return &value
}

func (h Handler) validateProfileBinding(ctx *gin.Context, rawSchoolID string, rawMajorID string) (*string, *string, bool) {
	var schoolID *string
	var majorID *string

	if strings.TrimSpace(rawSchoolID) != "" {
		parsed := nullableUUID(rawSchoolID)
		if parsed == nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_school_id", nil)
			return nil, nil, false
		}
		var school model.School
		if err := h.db.First(&school, "id = ? AND status <> ?", *parsed, model.StatusArchived).Error; err != nil {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "school_not_found", nil)
			return nil, nil, false
		}
		schoolID = parsed
	}

	if strings.TrimSpace(rawMajorID) != "" {
		parsed := nullableUUID(rawMajorID)
		if parsed == nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_major_id", nil)
			return nil, nil, false
		}
		var major model.Major
		if err := h.db.First(&major, "id = ? AND status <> ?", *parsed, model.StatusArchived).Error; err != nil {
			response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "major_not_found", nil)
			return nil, nil, false
		}
		if schoolID == nil {
			majorSchoolID := major.SchoolID
			schoolID = &majorSchoolID
		} else if *schoolID != major.SchoolID {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "major_school_mismatch", nil)
			return nil, nil, false
		}
		majorID = parsed
	}

	return schoolID, majorID, true
}

func publicUser(user model.User) gin.H {
	return gin.H{
		"id":            user.ID,
		"email":         user.Email,
		"name":          user.Name,
		"role":          user.Role,
		"status":        user.Status,
		"schoolId":      user.SchoolID,
		"majorId":       user.MajorID,
		"grade":         user.Grade,
		"emailVerified": user.EmailVerified,
	}
}

func setCookie(ctx *gin.Context, name string, value string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(name, value, maxAge, "/", "", false, true)
}

func clearCookie(ctx *gin.Context, name string) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(name, "", -1, "/", "", false, true)
}
