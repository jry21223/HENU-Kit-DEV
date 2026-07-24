package payment

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"

	"final-review-platform/services/api/pkg/config"
)

var (
	ErrWeChatNotifyDecryptFailed = errors.New("wechat_notify_decrypt_failed")
	ErrWeChatNotifyInvalid       = errors.New("wechat_notify_invalid")
	ErrWeChatNotifyAppMismatch   = errors.New("wechat_notify_appid_mismatch")
	ErrWeChatNotifyMchMismatch   = errors.New("wechat_notify_mchid_mismatch")
)

type weChatOfficialNotifyEnvelope struct {
	ID           string                          `json:"id"`
	CreateTime   string                          `json:"create_time"`
	EventType    string                          `json:"event_type"`
	ResourceType string                          `json:"resource_type"`
	Summary      string                          `json:"summary"`
	Resource     weChatOfficialEncryptedResource `json:"resource"`
}

type weChatOfficialEncryptedResource struct {
	Algorithm      string `json:"algorithm"`
	Ciphertext     string `json:"ciphertext"`
	AssociatedData string `json:"associated_data"`
	Nonce          string `json:"nonce"`
	OriginalType   string `json:"original_type"`
}

type weChatOfficialTransaction struct {
	AppID         string                          `json:"appid"`
	MchID         string                          `json:"mchid"`
	OutTradeNo    string                          `json:"out_trade_no"`
	TransactionID string                          `json:"transaction_id"`
	TradeState    string                          `json:"trade_state"`
	Amount        weChatOfficialTransactionAmount `json:"amount"`
}

type weChatOfficialTransactionAmount struct {
	Total int64 `json:"total"`
}

func (h Handler) processLiveNotify(ctx *gin.Context, payCfg config.WeChatPayConfig, body []byte) (gin.H, error) {
	if err := verifyWeChatHTTPResponse(payCfg.PlatformCertsDir, ctx.Request.Header, body); err != nil {
		return nil, err
	}
	var envelope weChatOfficialNotifyEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, ErrWeChatNotifyInvalid
	}
	if !strings.EqualFold(strings.TrimSpace(envelope.Resource.Algorithm), "AEAD_AES_256_GCM") {
		return nil, ErrWeChatNotifyInvalid
	}
	plaintext, err := DecryptWeChatResource(payCfg.APIV3Key, envelope.Resource.Nonce, envelope.Resource.AssociatedData, envelope.Resource.Ciphertext)
	if err != nil {
		return nil, ErrWeChatNotifyDecryptFailed
	}
	var transaction weChatOfficialTransaction
	if err := json.Unmarshal(plaintext, &transaction); err != nil {
		return nil, ErrWeChatNotifyInvalid
	}
	if strings.TrimSpace(transaction.AppID) != payCfg.AppID {
		return nil, ErrWeChatNotifyAppMismatch
	}
	if strings.TrimSpace(transaction.MchID) != payCfg.MchID {
		return nil, ErrWeChatNotifyMchMismatch
	}
	payload := mockNotifyPayload{
		OutTradeNo:    transaction.OutTradeNo,
		TransactionID: transaction.TransactionID,
		TradeState:    transaction.TradeState,
		AmountTotal:   transaction.Amount.Total,
	}
	return h.processMockNotify(payload, datatypes.JSON(body))
}

func (h Handler) handleLiveNotify(ctx *gin.Context, payCfg config.WeChatPayConfig) {
	body, err := ctx.GetRawData()
	if err != nil || len(body) == 0 {
		wechatNotifyFailure(ctx, "invalid_request", http.StatusBadRequest)
		return
	}
	result, err := h.processLiveNotify(ctx, payCfg, body)
	if err != nil {
		// Map known errors to safe codes; do not leak internal details.
		wechatNotifyFailure(ctx, wechatLiveNotifyErrorCode(err), http.StatusBadRequest)
		return
	}
	wechatNotifySuccess(ctx, result)
}

// wechatLiveNotifyErrorCode maps processLiveNotify errors to safe user-facing codes.
func wechatLiveNotifyErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrWeChatNotifyDecryptFailed):
		return "notify_decrypt_failed"
	case errors.Is(err, ErrWeChatNotifyInvalid):
		return "notify_invalid"
	case errors.Is(err, ErrWeChatNotifyAppMismatch):
		return "notify_appid_mismatch"
	case errors.Is(err, ErrWeChatNotifyMchMismatch):
		return "notify_mchid_mismatch"
	case errors.Is(err, ErrInvalidSignature):
		return "notify_signature_invalid"
	default:
		return "notify_processing_failed"
	}
}
