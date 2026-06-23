package payment

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"final-review-platform/services/api/internal/auth"
	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/config"
	"final-review-platform/services/api/pkg/response"
)

const (
	providerWeChatNative     = "wechat_native"
	productTypeCoursePackage = "course_package"
	wechatModeMock           = "mock"
	wechatModeLive           = "live"
	defaultNativeExpireMins  = 15
)

var (
	ErrInvalidWeChatMode             = errors.New("wechat_invalid_mode")
	ErrWeChatMockForbiddenProduction = errors.New("wechat_mock_forbidden_in_production")
	ErrWeChatLiveConfigMissing       = errors.New("wechat_live_config_missing")
	ErrWeChatLiveNotImplemented      = errors.New("wechat_live_not_implemented")
)

type Handler struct {
	db  *gorm.DB
	cfg config.Config
}

type nativeRequest struct {
	OrderID string `json:"orderId"`
}

type nativeResponse struct {
	OrderID     string    `json:"orderId"`
	CodeURL     string    `json:"codeUrl"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Status      string    `json:"status"`
	AmountTotal int64     `json:"amountTotal"`
	Currency    string    `json:"currency"`
	Title       string    `json:"title"`
	Mock        bool      `json:"mock"`
}

func NewHandler(db *gorm.DB, cfg config.Config) Handler {
	return Handler{db: db, cfg: cfg}
}

func (h Handler) WeChatNative(ctx *gin.Context) {
	user, ok := auth.CurrentUser(ctx)
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, response.CodeUnauthorized, "unauthorized", nil)
		return
	}
	var req nativeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_request", nil)
		return
	}
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "missing_order_id", nil)
		return
	}

	var order model.Order
	if err := h.db.First(&order, "id = ?", orderID).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "order_not_found", nil)
		return
	}
	if order.UserID != user.ID {
		response.Error(ctx, http.StatusForbidden, response.CodeForbidden, "forbidden", nil)
		return
	}
	if order.PaymentProvider != providerWeChatNative {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsupported_payment_provider", nil)
		return
	}
	if order.Status != model.OrderPending && order.Status != model.OrderPaying {
		response.Error(ctx, http.StatusConflict, response.CodeConflict, "order_not_payable", gin.H{"status": order.Status})
		return
	}
	if order.AmountTotal <= 0 {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "invalid_amount", nil)
		return
	}

	coursePackage, ok := h.coursePackageForOrder(ctx, order)
	if !ok {
		return
	}

	payCfg := normalizedWeChatConfig(h.cfg.WeChatPay)
	if err := ValidateWeChatNativeConfig(h.cfg.Environment, payCfg); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, err.Error(), nil)
		return
	}
	if payCfg.Mode == wechatModeLive {
		response.Error(ctx, http.StatusNotImplemented, response.CodeInternalServer, ErrWeChatLiveNotImplemented.Error(), nil)
		return
	}

	expiresAt := time.Now().UTC().Add(time.Duration(payCfg.NativeExpireMinutes) * time.Minute)
	codeURL := mockNativeCodeURL(order.OutTradeNo)
	if err := h.markOrderPaying(order.ID, codeURL, expiresAt); err != nil {
		response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "payment_create_failed", nil)
		return
	}

	response.OK(ctx, nativeResponse{
		OrderID:     order.ID,
		CodeURL:     codeURL,
		ExpiresAt:   expiresAt,
		Status:      model.OrderPaying,
		AmountTotal: order.AmountTotal,
		Currency:    order.Currency,
		Title:       coursePackage.Title,
		Mock:        true,
	})
}

func ValidateWeChatNativeConfig(environment string, cfg config.WeChatPayConfig) error {
	cfg = normalizedWeChatConfig(cfg)
	switch cfg.Mode {
	case wechatModeMock:
		if strings.EqualFold(strings.TrimSpace(environment), "production") {
			return ErrWeChatMockForbiddenProduction
		}
		return nil
	case wechatModeLive:
		if cfg.AppID == "" || cfg.MchID == "" || cfg.APIV3Key == "" || cfg.MerchantSerialNo == "" || cfg.NotifyURL == "" {
			return ErrWeChatLiveConfigMissing
		}
		if cfg.MerchantPrivateKey == "" && cfg.MerchantPrivateKeyPath == "" {
			return ErrWeChatLiveConfigMissing
		}
		return nil
	default:
		return ErrInvalidWeChatMode
	}
}

func normalizedWeChatConfig(cfg config.WeChatPayConfig) config.WeChatPayConfig {
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.Mode == "" {
		cfg.Mode = wechatModeMock
	}
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.MchID = strings.TrimSpace(cfg.MchID)
	cfg.APIV3Key = strings.TrimSpace(cfg.APIV3Key)
	cfg.MerchantSerialNo = strings.TrimSpace(cfg.MerchantSerialNo)
	cfg.MerchantPrivateKey = strings.TrimSpace(cfg.MerchantPrivateKey)
	cfg.MerchantPrivateKeyPath = strings.TrimSpace(cfg.MerchantPrivateKeyPath)
	cfg.PlatformCertsDir = strings.TrimSpace(cfg.PlatformCertsDir)
	cfg.NotifyURL = strings.TrimSpace(cfg.NotifyURL)
	if cfg.NativeExpireMinutes <= 0 {
		cfg.NativeExpireMinutes = defaultNativeExpireMins
	}
	return cfg
}

func (h Handler) coursePackageForOrder(ctx *gin.Context, order model.Order) (model.CoursePackage, bool) {
	if order.ProductType != productTypeCoursePackage {
		response.Error(ctx, http.StatusBadRequest, response.CodeBadRequest, "unsupported_product_type", nil)
		return model.CoursePackage{}, false
	}
	var coursePackage model.CoursePackage
	if err := h.db.First(&coursePackage, "id = ? AND status = ?", order.ProductID, model.StatusPublished).Error; err != nil {
		response.Error(ctx, http.StatusNotFound, response.CodeNotFound, "package_not_found", nil)
		return model.CoursePackage{}, false
	}
	return coursePackage, true
}

func (h Handler) markOrderPaying(orderID string, codeURL string, expiresAt time.Time) error {
	metadata, err := json.Marshal(map[string]interface{}{
		"wechatNative": map[string]interface{}{
			"mode":      wechatModeMock,
			"codeUrl":   codeURL,
			"expiresAt": expiresAt.Format(time.RFC3339),
		},
	})
	if err != nil {
		return err
	}
	return h.db.Model(&model.Order{}).
		Where("id = ? AND status IN ?", orderID, []string{model.OrderPending, model.OrderPaying}).
		Updates(map[string]interface{}{
			"status":   model.OrderPaying,
			"metadata": datatypes.JSON(metadata),
		}).Error
}

func mockNativeCodeURL(outTradeNo string) string {
	return fmt.Sprintf("weixin://wxpay/mock/%s", url.PathEscape(outTradeNo))
}
