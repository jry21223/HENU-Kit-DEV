package order

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/orderstate"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/response"
)

const (
	productTypeCoursePackage = "course_package"
	paymentProviderWechat    = "wechat_native"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) Handler {
	return Handler{db: db}
}

type createOrderRequest struct {
	PackageID string `json:"packageId"`
}

type orderRow struct {
	Order              model.Order          `json:"order"`
	Package            *model.CoursePackage `json:"package,omitempty"`
	AlreadyOwned       bool                 `json:"alreadyOwned"`
	AlreadyPending     bool                 `json:"alreadyPending"`
	EntitlementGranted bool                 `json:"entitlementGranted"`
}

type orderStatus struct {
	OrderID            string     `json:"orderId"`
	Status             string     `json:"status"`
	PaidAt             *time.Time `json:"paidAt,omitempty"`
	EntitlementGranted bool       `json:"entitlementGranted"`
	PaymentProvider    string     `json:"paymentProvider"`
	ProductType        string     `json:"productType"`
	ProductID          string     `json:"productId"`
	AmountTotal        int64      `json:"amountTotal"`
	Currency           string     `json:"currency"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	PackageTitle       string     `json:"packageTitle,omitempty"`
}

func (h Handler) Create(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req createOrderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	packageID := strings.TrimSpace(req.PackageID)
	if packageID == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_package_id", nil)
		return
	}

	var coursePackage model.CoursePackage
	if err := h.db.First(&coursePackage, "id = ? AND status = ?", packageID, model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "package_not_found", nil)
		return
	}
	if h.hasActivePackageGrant(user.ID, coursePackage.ID) {
		response.OK(ctx, gin.H{
			"alreadyOwned":       true,
			"alreadyPending":     false,
			"entitlementGranted": true,
			"package":            coursePackage,
		})
		return
	}
	if err := orderstate.ExpireStalePackageOrders(h.db, user.ID, productTypeCoursePackage, coursePackage.ID, paymentProviderWechat, time.Now().UTC()); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	}
	if existing, ok, err := h.latestPendingOrder(user.ID, coursePackage.ID); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return
	} else if ok {
		response.OK(ctx, orderRow{
			Order:              existing,
			Package:            &coursePackage,
			AlreadyPending:     true,
			EntitlementGranted: false,
		})
		return
	}

	order := model.Order{
		UserID:          user.ID,
		ProductType:     productTypeCoursePackage,
		ProductID:       coursePackage.ID,
		OutTradeNo:      newOutTradeNo(),
		PaymentProvider: paymentProviderWechat,
		Status:          model.OrderPending,
		AmountTotal:     coursePackage.PriceFen,
		Currency:        normalizedCurrency(coursePackage.Currency),
	}
	if err := h.db.Create(&order).Error; err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "create_failed", nil)
		return
	}
	response.OK(ctx, orderRow{
		Order:              order,
		Package:            &coursePackage,
		AlreadyOwned:       false,
		AlreadyPending:     false,
		EntitlementGranted: false,
	})
}

func (h Handler) Detail(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	order, coursePackage, found := h.readOrderForUser(ctx, user)
	if !found {
		return
	}
	response.OK(ctx, orderRow{
		Order:              order,
		Package:            coursePackage,
		EntitlementGranted: h.entitlementGranted(order),
	})
}

func (h Handler) Status(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	order, coursePackage, found := h.readOrderForUser(ctx, user)
	if !found {
		return
	}
	status := orderStatus{
		OrderID:            order.ID,
		Status:             order.Status,
		PaidAt:             order.PaidAt,
		EntitlementGranted: h.entitlementGranted(order),
		PaymentProvider:    order.PaymentProvider,
		ProductType:        order.ProductType,
		ProductID:          order.ProductID,
		AmountTotal:        order.AmountTotal,
		Currency:           order.Currency,
		ExpiresAt:          order.ExpiresAt,
	}
	if coursePackage != nil {
		status.PackageTitle = coursePackage.Title
	}
	response.OK(ctx, status)
}

func (h Handler) readOrderForUser(ctx *gin.Context, user *model.User) (model.Order, *model.CoursePackage, bool) {
	var order model.Order
	if err := h.db.First(&order, "id = ?", ctx.Param("id")).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "order_not_found", nil)
		return model.Order{}, nil, false
	}
	if order.UserID != user.ID && user.Role != model.RoleAdmin && user.Role != model.RoleSuperAdmin {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return model.Order{}, nil, false
	}
	if _, err := orderstate.ExpireOrderIfNeeded(h.db, &order, time.Now().UTC()); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "query_failed", nil)
		return model.Order{}, nil, false
	}
	var coursePackage *model.CoursePackage
	if order.ProductType == productTypeCoursePackage {
		var row model.CoursePackage
		if err := h.db.First(&row, "id = ?", order.ProductID).Error; err == nil {
			coursePackage = &row
		}
	}
	return order, coursePackage, true
}

func (h Handler) latestPendingOrder(userID string, packageID string) (model.Order, bool, error) {
	var existing model.Order
	err := h.db.Where("user_id = ? AND product_type = ? AND product_id = ? AND payment_provider = ? AND status IN ?", userID, productTypeCoursePackage, packageID, paymentProviderWechat, []string{model.OrderPending, model.OrderPaying}).
		Where("expires_at IS NULL OR expires_at > ?", time.Now().UTC()).
		Order("created_at desc").
		First(&existing).Error
	if err == nil {
		return existing, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Order{}, false, nil
	}
	return model.Order{}, false, err
}

func (h Handler) hasActivePackageGrant(userID string, packageID string) bool {
	var count int64
	h.db.Model(&model.MaterialAccessGrant{}).
		Joins("JOIN course_packages ON course_packages.id = material_access_grants.package_id").
		Where("material_access_grants.user_id = ? AND material_access_grants.package_id = ?", userID, packageID).
		Where("course_packages.status = ?", model.StatusPublished).
		Where("material_access_grants.expires_at IS NULL OR material_access_grants.expires_at > ?", time.Now()).
		Count(&count)
	return count > 0
}

func (h Handler) entitlementGranted(order model.Order) bool {
	if order.ProductType != productTypeCoursePackage {
		return false
	}
	return h.hasActivePackageGrant(order.UserID, order.ProductID)
}

func normalizedCurrency(currency string) string {
	value := strings.ToUpper(strings.TrimSpace(currency))
	if value == "" {
		return "CNY"
	}
	return value
}

func newOutTradeNo() string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "FR" + time.Now().UTC().Format("20060102150405")
	}
	return "FR" + time.Now().UTC().Format("20060102150405") + strings.ToUpper(hex.EncodeToString(random))
}
