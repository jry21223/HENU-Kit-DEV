package member

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/audit"
	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const (
	statusActive  = "active"
	statusRevoked = "revoked"
	sourceAdmin   = "manual_admin"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type grantRequest struct {
	UserID    string `json:"userId"`
	PlanCode  string `json:"planCode"`
	ExpiresAt string `json:"expiresAt"`
	Source    string `json:"source"`
	Note      string `json:"note"`
}

type revokeRequest struct {
	Reason string `json:"reason"`
}

type membershipRow struct {
	Membership model.Membership      `json:"membership"`
	User       *model.User           `json:"user,omitempty"`
	Plan       *model.MembershipPlan `json:"plan,omitempty"`
	Active     bool                  `json:"active"`
}

func (h Handler) Plans(ctx *gin.Context) {
	var plans []model.MembershipPlan
	if err := h.db.Where("status = ?", model.StatusPublished).Order("price_fen asc, created_at asc").Find(&plans).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"plans": plans})
}

func (h Handler) Me(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	now := time.Now()
	var memberships []model.Membership
	if err := h.db.Where("user_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", user.ID, statusActive, now).
		Order("expires_at desc, created_at desc").
		Find(&memberships).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	plansByCode, err := h.plansByCodeForMemberships(memberships)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	rows := membershipRows(memberships, nil, plansByCode)
	response.OK(ctx, gin.H{"memberships": rows, "current": currentMembership(rows)})
}

func (h Handler) AdminMemberships(ctx *gin.Context) {
	limit, ok := parseLimit(ctx.Query("limit"), 100, 500)
	if !ok {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_limit", nil)
		return
	}
	query := h.db.Model(&model.Membership{})
	if userID := strings.TrimSpace(ctx.Query("userId")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if planCode := strings.TrimSpace(ctx.Query("planCode")); planCode != "" {
		query = query.Where("plan_code = ?", planCode)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	var memberships []model.Membership
	if err := query.Order("created_at desc").Limit(limit).Find(&memberships).Error; err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	usersByID, err := h.usersByIDForMemberships(memberships)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	plansByCode, err := h.plansByCodeForMemberships(memberships)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	response.OK(ctx, gin.H{"memberships": membershipRows(memberships, usersByID, plansByCode)})
}

func (h Handler) Grant(ctx *gin.Context) {
	operator, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req grantRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	userID := strings.TrimSpace(req.UserID)
	planCode := strings.TrimSpace(req.PlanCode)
	if userID == "" || planCode == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "user_and_plan_required", nil)
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = sourceAdmin
	}
	if len([]rune(source)) > 40 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "source_too_long", nil)
		return
	}
	expiresAt, err := parseOptionalFutureTime(req.ExpiresAt)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	note := strings.TrimSpace(req.Note)
	if len([]rune(note)) > 500 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "note_too_long", nil)
		return
	}

	var membership model.Membership
	created := false
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.First(&user, "id = ?", userID).Error; err != nil {
			return errString("user_not_found")
		}
		var plan model.MembershipPlan
		if err := tx.First(&plan, "code = ? AND status = ?", planCode, model.StatusPublished).Error; err != nil {
			return errString("plan_not_found")
		}
		now := time.Now()
		err := tx.Where("user_id = ? AND plan_code = ? AND source = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", userID, planCode, source, statusActive, now).
			Order("created_at desc").
			First(&membership).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			membership = model.Membership{
				UserID:    userID,
				PlanCode:  planCode,
				Status:    statusActive,
				Source:    source,
				ExpiresAt: expiresAt,
			}
			if err := tx.Create(&membership).Error; err != nil {
				return err
			}
			created = true
		} else if err := tx.Model(&model.Membership{}).Where("id = ?", membership.ID).Updates(map[string]interface{}{
			"status":     statusActive,
			"expires_at": expiresAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.First(&membership, "id = ?", membership.ID).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "membership.granted", "membership", membership.ID, map[string]interface{}{
			"operatorId": operator.ID,
			"userId":     userID,
			"planCode":   planCode,
			"source":     source,
			"expiresAt":  expiresAt,
			"created":    created,
			"note":       note,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	response.OK(ctx, gin.H{"membership": membership, "created": created})
}

func (h Handler) Revoke(ctx *gin.Context) {
	operator, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req revokeRequest
	if ctx.Request.Body != nil && ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
			return
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if len([]rune(reason)) > 500 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "reason_too_long", nil)
		return
	}
	var membership model.Membership
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&membership, "id = ?", ctx.Param("id")).Error; err != nil {
			return errString("membership_not_found")
		}
		if membership.Status != statusRevoked {
			if err := tx.Model(&model.Membership{}).Where("id = ?", membership.ID).Updates(map[string]interface{}{
				"status":     statusRevoked,
				"expires_at": time.Now(),
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.First(&membership, "id = ?", membership.ID).Error; err != nil {
			return err
		}
		return audit.Record(ctx, tx, "membership.revoked", "membership", membership.ID, map[string]interface{}{
			"operatorId": operator.ID,
			"userId":     membership.UserID,
			"planCode":   membership.PlanCode,
			"reason":     reason,
		})
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, err.Error(), nil)
		return
	}
	response.OK(ctx, gin.H{"membership": membership})
}

func (h Handler) usersByIDForMemberships(memberships []model.Membership) (map[string]model.User, error) {
	ids := make([]string, 0, len(memberships))
	seen := map[string]struct{}{}
	for _, membership := range memberships {
		if _, ok := seen[membership.UserID]; ok {
			continue
		}
		seen[membership.UserID] = struct{}{}
		ids = append(ids, membership.UserID)
	}
	usersByID := map[string]model.User{}
	if len(ids) == 0 {
		return usersByID, nil
	}
	var users []model.User
	if err := h.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		usersByID[user.ID] = user
	}
	return usersByID, nil
}

func (h Handler) plansByCodeForMemberships(memberships []model.Membership) (map[string]model.MembershipPlan, error) {
	codes := make([]string, 0, len(memberships))
	seen := map[string]struct{}{}
	for _, membership := range memberships {
		if _, ok := seen[membership.PlanCode]; ok {
			continue
		}
		seen[membership.PlanCode] = struct{}{}
		codes = append(codes, membership.PlanCode)
	}
	plansByCode := map[string]model.MembershipPlan{}
	if len(codes) == 0 {
		return plansByCode, nil
	}
	var plans []model.MembershipPlan
	if err := h.db.Where("code IN ?", codes).Find(&plans).Error; err != nil {
		return nil, err
	}
	for _, plan := range plans {
		plansByCode[plan.Code] = plan
	}
	return plansByCode, nil
}

func membershipRows(memberships []model.Membership, usersByID map[string]model.User, plansByCode map[string]model.MembershipPlan) []membershipRow {
	now := time.Now()
	rows := make([]membershipRow, 0, len(memberships))
	for _, membership := range memberships {
		row := membershipRow{
			Membership: membership,
			Active:     membership.Status == statusActive && (membership.ExpiresAt == nil || membership.ExpiresAt.After(now)),
		}
		if user, ok := usersByID[membership.UserID]; ok {
			row.User = &user
		}
		if plan, ok := plansByCode[membership.PlanCode]; ok {
			row.Plan = &plan
		}
		rows = append(rows, row)
	}
	return rows
}

func currentMembership(rows []membershipRow) *membershipRow {
	if len(rows) == 0 {
		return nil
	}
	best := rows[0]
	for _, row := range rows[1:] {
		if planRank(row.Membership.PlanCode) > planRank(best.Membership.PlanCode) {
			best = row
			continue
		}
		if planRank(row.Membership.PlanCode) == planRank(best.Membership.PlanCode) && expiresAfter(row.Membership.ExpiresAt, best.Membership.ExpiresAt) {
			best = row
		}
	}
	return &best
}

func planRank(code string) int {
	switch code {
	case "tier2":
		return 2
	case "tier1":
		return 1
	default:
		return 0
	}
}

func expiresAfter(left *time.Time, right *time.Time) bool {
	if left == nil {
		return true
	}
	if right == nil {
		return false
	}
	return left.After(*right)
}

func parseOptionalFutureTime(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, errString("invalid_expires_at")
	}
	if !parsed.After(time.Now()) {
		return nil, errString("expires_at_in_past")
	}
	return &parsed, nil
}

func parseLimit(value string, fallback int, max int) (int, bool) {
	if strings.TrimSpace(value) == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 || limit > max {
		return 0, false
	}
	return limit, true
}

type errString string

func (e errString) Error() string {
	return string(e)
}
