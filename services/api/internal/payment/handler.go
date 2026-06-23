package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	ErrWeChatMockNotifySecretMissing = errors.New("wechat_mock_notify_secret_missing")
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

type mockNotifyPayload struct {
	OutTradeNo    string `json:"outTradeNo"`
	TransactionID string `json:"transactionId"`
	TradeState    string `json:"tradeState"`
	AmountTotal   int64  `json:"amountTotal"`
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
		result, err := createLiveNativePayment(ctx.Request.Context(), payCfg, order, coursePackage)
		if err != nil {
			response.Error(ctx, http.StatusBadGateway, response.CodeInternalServer, err.Error(), nil)
			return
		}
		if err := h.markOrderPaying(order.ID, result.CodeURL, result.ExpiresAt); err != nil {
			response.Error(ctx, http.StatusInternalServerError, response.CodeInternalServer, "payment_create_failed", nil)
			return
		}
		response.OK(ctx, nativeResponse{
			OrderID:     order.ID,
			CodeURL:     result.CodeURL,
			ExpiresAt:   result.ExpiresAt,
			Status:      model.OrderPaying,
			AmountTotal: order.AmountTotal,
			Currency:    order.Currency,
			Title:       coursePackage.Title,
			Mock:        false,
		})
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

func (h Handler) WeChatNotify(ctx *gin.Context) {
	payCfg := normalizedWeChatConfig(h.cfg.WeChatPay)
	if err := ValidateWeChatNativeConfig(h.cfg.Environment, payCfg); err != nil {
		wechatNotifyFailure(ctx, err.Error(), http.StatusBadRequest)
		return
	}
	if payCfg.Mode == wechatModeLive {
		wechatNotifyFailure(ctx, ErrWeChatLiveNotImplemented.Error(), http.StatusNotImplemented)
		return
	}
	if payCfg.APIV3Key == "" {
		wechatNotifyFailure(ctx, ErrWeChatMockNotifySecretMissing.Error(), http.StatusBadRequest)
		return
	}

	body, err := ctx.GetRawData()
	if err != nil || len(body) == 0 {
		wechatNotifyFailure(ctx, "invalid_request", http.StatusBadRequest)
		return
	}
	if !validMockNotifySignature(body, ctx.GetHeader("X-WeChat-Mock-Signature"), payCfg.APIV3Key) {
		wechatNotifyFailure(ctx, "invalid_signature", http.StatusBadRequest)
		return
	}

	var payload mockNotifyPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		wechatNotifyFailure(ctx, "invalid_request", http.StatusBadRequest)
		return
	}
	result, err := h.processMockNotify(payload, datatypes.JSON(body))
	if err != nil {
		wechatNotifyFailure(ctx, err.Error(), http.StatusBadRequest)
		return
	}
	wechatNotifySuccess(ctx, result)
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
		if cfg.APIBaseURL == "" || cfg.AppID == "" || cfg.MchID == "" || cfg.APIV3Key == "" || cfg.MerchantSerialNo == "" || cfg.NotifyURL == "" {
			return ErrWeChatLiveConfigMissing
		}
		if cfg.MerchantPrivateKey == "" && cfg.MerchantPrivateKeyPath == "" {
			return ErrWeChatLiveConfigMissing
		}
		if cfg.PlatformCertsDir == "" {
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
	cfg.APIBaseURL = strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.mch.weixin.qq.com"
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

func validMockNotifySignature(body []byte, signature string, secret string) bool {
	signature = strings.TrimSpace(signature)
	if signature == "" || secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(strings.ToLower(signature)))
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

func (h Handler) processMockNotify(payload mockNotifyPayload, raw datatypes.JSON) (gin.H, error) {
	payload.OutTradeNo = strings.TrimSpace(payload.OutTradeNo)
	payload.TransactionID = strings.TrimSpace(payload.TransactionID)
	payload.TradeState = strings.ToUpper(strings.TrimSpace(payload.TradeState))
	if payload.OutTradeNo == "" || payload.TransactionID == "" || payload.TradeState == "" {
		return nil, errors.New("invalid_notify_payload")
	}

	var order model.Order
	if err := h.db.First(&order, "out_trade_no = ? AND payment_provider = ?", payload.OutTradeNo, providerWeChatNative).Error; err != nil {
		return nil, errors.New("order_not_found")
	}
	if payload.AmountTotal != order.AmountTotal {
		_ = h.db.Model(&model.Order{}).Where("id = ?", order.ID).Update("risk_flag", "wechat_amount_mismatch").Error
		return nil, errors.New("amount_mismatch")
	}
	if payload.TradeState == "SUCCESS" && order.Status != model.OrderPending && order.Status != model.OrderPaying && order.Status != model.OrderPaid {
		return nil, errors.New("order_not_payable")
	}

	if payload.TradeState != "SUCCESS" {
		if err := h.recordPaymentNotify(order, payload, raw, nil); err != nil {
			return nil, err
		}
		return gin.H{"processed": true, "paid": false, "tradeState": payload.TradeState}, nil
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.First(&order, "id = ?", order.ID).Error; err != nil {
			return err
		}
		if err := h.recordPaymentNotifyTx(tx, order, payload, raw, &now); err != nil {
			return err
		}
		if order.Status != model.OrderPaid {
			if err := tx.Model(&model.Order{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
				"status":    model.OrderPaid,
				"paid_at":   &now,
				"risk_flag": "",
			}).Error; err != nil {
				return err
			}
		}
		if order.ProductType != productTypeCoursePackage {
			return errors.New("unsupported_product_type")
		}
		var coursePackage model.CoursePackage
		if err := tx.First(&coursePackage, "id = ? AND status = ?", order.ProductID, model.StatusPublished).Error; err != nil {
			return errors.New("package_not_found")
		}
		return ensurePackageGrantTx(tx, order)
	})
	if err != nil {
		return nil, err
	}
	return gin.H{"processed": true, "paid": true}, nil
}

func (h Handler) recordPaymentNotify(order model.Order, payload mockNotifyPayload, raw datatypes.JSON, processedAt *time.Time) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		return h.recordPaymentNotifyTx(tx, order, payload, raw, processedAt)
	})
}

func (h Handler) recordPaymentNotifyTx(tx *gorm.DB, order model.Order, payload mockNotifyPayload, raw datatypes.JSON, processedAt *time.Time) error {
	record := model.PaymentRecord{
		OrderID:        order.ID,
		Provider:       providerWeChatNative,
		TransactionID:  payload.TransactionID,
		TradeState:     payload.TradeState,
		AmountTotal:    payload.AmountTotal,
		RawNotify:      raw,
		IdempotencyKey: providerWeChatNative + ":" + payload.TransactionID,
		ProcessedAt:    processedAt,
	}
	var existing model.PaymentRecord
	err := tx.First(&existing, "idempotency_key = ?", record.IdempotencyKey).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&record).Error
}

func ensurePackageGrantTx(tx *gorm.DB, order model.Order) error {
	var existing model.MaterialAccessGrant
	err := tx.First(&existing, "user_id = ? AND package_id = ? AND order_id = ? AND source = ?", order.UserID, order.ProductID, order.ID, "order").Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	grant := model.MaterialAccessGrant{
		UserID:    order.UserID,
		PackageID: &order.ProductID,
		Source:    "order",
		OrderID:   &order.ID,
	}
	return tx.Create(&grant).Error
}

func wechatNotifySuccess(ctx *gin.Context, data gin.H) {
	ctx.JSON(http.StatusOK, gin.H{
		"code":    "SUCCESS",
		"message": "成功",
		"data":    data,
	})
}

func wechatNotifyFailure(ctx *gin.Context, message string, status int) {
	ctx.JSON(status, gin.H{
		"code":    "FAIL",
		"message": message,
	})
}
