package orderstate

import (
	"time"

	"gorm.io/gorm"

	"final-review-platform/services/api/internal/platform/model"
)

const ProviderWeChatNative = "wechat_native"

func IsExpirable(status string) bool {
	return status == model.OrderPending || status == model.OrderPaying
}

func IsExpired(order model.Order, now time.Time) bool {
	if order.PaymentProvider != ProviderWeChatNative || !IsExpirable(order.Status) || order.ExpiresAt == nil {
		return false
	}
	return !order.ExpiresAt.After(now.UTC())
}

func ExpireOrderIfNeeded(db *gorm.DB, order *model.Order, now time.Time) (bool, error) {
	if !IsExpired(*order, now) {
		return false, nil
	}
	result := db.Model(&model.Order{}).
		Where("id = ? AND payment_provider = ? AND status IN ? AND expires_at IS NOT NULL AND expires_at <= ?", order.ID, ProviderWeChatNative, []string{model.OrderPending, model.OrderPaying}, now.UTC()).
		Updates(map[string]interface{}{
			"status":    model.OrderExpired,
			"risk_flag": "",
		})
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected > 0 {
		order.Status = model.OrderExpired
		return true, nil
	}
	if err := db.First(order, "id = ?", order.ID).Error; err != nil {
		return false, err
	}
	return false, nil
}

func ExpireStalePackageOrders(db *gorm.DB, userID string, productType string, productID string, provider string, now time.Time) error {
	return db.Model(&model.Order{}).
		Where("user_id = ? AND product_type = ? AND product_id = ? AND payment_provider = ?", userID, productType, productID, provider).
		Where("status IN ? AND expires_at IS NOT NULL AND expires_at <= ?", []string{model.OrderPending, model.OrderPaying}, now.UTC()).
		Updates(map[string]interface{}{
			"status":    model.OrderExpired,
			"risk_flag": "",
		}).Error
}

func ExpireAllStale(db *gorm.DB, now time.Time) error {
	return db.Model(&model.Order{}).
		Where("payment_provider = ? AND status IN ? AND expires_at IS NOT NULL AND expires_at <= ?", ProviderWeChatNative, []string{model.OrderPending, model.OrderPaying}, now.UTC()).
		Updates(map[string]interface{}{
			"status":    model.OrderExpired,
			"risk_flag": "",
		}).Error
}
