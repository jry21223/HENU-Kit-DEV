package payment

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/config"
)

const wechatNativePath = "/v3/pay/transactions/native"

var (
	ErrWeChatNativeRequestFailed = errors.New("wechat_native_request_failed")
	ErrWeChatCloseRequestFailed  = errors.New("wechat_close_request_failed")
	ErrWeChatResponseInvalid     = errors.New("wechat_response_invalid")
)

type weChatNativeResult struct {
	CodeURL   string
	ExpiresAt time.Time
}

type weChatNativeRequest struct {
	AppID       string             `json:"appid"`
	MchID       string             `json:"mchid"`
	Description string             `json:"description"`
	OutTradeNo  string             `json:"out_trade_no"`
	NotifyURL   string             `json:"notify_url"`
	Amount      weChatNativeAmount `json:"amount"`
}

type weChatNativeAmount struct {
	Total    int64  `json:"total"`
	Currency string `json:"currency,omitempty"`
}

type weChatCloseRequest struct {
	MchID string `json:"mchid"`
}

type weChatNativeResponse struct {
	CodeURL string `json:"code_url"`
}

func createLiveNativePayment(ctx context.Context, payCfg config.WeChatPayConfig, order model.Order, coursePackage model.CoursePackage) (weChatNativeResult, error) {
	payCfg = normalizedWeChatConfig(payCfg)
	privateKey, err := LoadMerchantPrivateKey(payCfg)
	if err != nil {
		return weChatNativeResult{}, err
	}
	description := truncateRunes(coursePackage.Title, 127)
	if description == "" {
		description = "课程复习包"
	}
	requestBody, err := json.Marshal(weChatNativeRequest{
		AppID:       payCfg.AppID,
		MchID:       payCfg.MchID,
		Description: description,
		OutTradeNo:  order.OutTradeNo,
		NotifyURL:   payCfg.NotifyURL,
		Amount: weChatNativeAmount{
			Total:    order.AmountTotal,
			Currency: normalizedWeChatCurrency(order.Currency),
		},
	})
	if err != nil {
		return weChatNativeResult{}, err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return weChatNativeResult{}, err
	}
	now := time.Now().UTC()
	authHeader, err := BuildSignedWeChatAuthorizationHeader(
		http.MethodPost,
		wechatNativePath,
		string(requestBody),
		now.Unix(),
		nonce,
		payCfg.MchID,
		payCfg.MerchantSerialNo,
		privateKey,
	)
	if err != nil {
		return weChatNativeResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payCfg.APIBaseURL+wechatNativePath, bytes.NewReader(requestBody))
	if err != nil {
		return weChatNativeResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "final-review-platform/0.1 wechat-pay-native")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return weChatNativeResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return weChatNativeResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return weChatNativeResult{}, fmt.Errorf("%w: status=%d body=%s", ErrWeChatNativeRequestFailed, resp.StatusCode, string(responseBody))
	}
	if err := verifyWeChatHTTPResponse(payCfg.PlatformCertsDir, resp.Header, responseBody); err != nil {
		return weChatNativeResult{}, err
	}
	var nativeResponse weChatNativeResponse
	if err := json.Unmarshal(responseBody, &nativeResponse); err != nil {
		return weChatNativeResult{}, ErrWeChatResponseInvalid
	}
	nativeResponse.CodeURL = strings.TrimSpace(nativeResponse.CodeURL)
	if nativeResponse.CodeURL == "" {
		return weChatNativeResult{}, ErrWeChatResponseInvalid
	}
	return weChatNativeResult{
		CodeURL:   nativeResponse.CodeURL,
		ExpiresAt: now.Add(time.Duration(payCfg.NativeExpireMinutes) * time.Minute),
	}, nil
}

func closeLiveNativeOrder(ctx context.Context, payCfg config.WeChatPayConfig, order model.Order) error {
	payCfg = normalizedWeChatConfig(payCfg)
	privateKey, err := LoadMerchantPrivateKey(payCfg)
	if err != nil {
		return err
	}
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(order.OutTradeNo) + "/close"
	requestBody, err := json.Marshal(weChatCloseRequest{MchID: payCfg.MchID})
	if err != nil {
		return err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	authHeader, err := BuildSignedWeChatAuthorizationHeader(
		http.MethodPost,
		path,
		string(requestBody),
		now.Unix(),
		nonce,
		payCfg.MchID,
		payCfg.MerchantSerialNo,
		privateKey,
	)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payCfg.APIBaseURL+path, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "final-review-platform/0.1 wechat-pay-native")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status=%d body=%s", ErrWeChatCloseRequestFailed, resp.StatusCode, string(responseBody))
	}
	if len(responseBody) > 0 {
		if err := verifyWeChatHTTPResponse(payCfg.PlatformCertsDir, resp.Header, responseBody); err != nil {
			return err
		}
	}
	return nil
}

func verifyWeChatHTTPResponse(platformCertsDir string, headers http.Header, body []byte) error {
	timestamp := headers.Get("Wechatpay-Timestamp")
	nonce := headers.Get("Wechatpay-Nonce")
	signature := headers.Get("Wechatpay-Signature")
	serial := headers.Get("Wechatpay-Serial")
	if strings.TrimSpace(timestamp) == "" || strings.TrimSpace(nonce) == "" || strings.TrimSpace(signature) == "" || strings.TrimSpace(serial) == "" {
		return ErrInvalidSignature
	}
	publicKey, err := LoadPlatformPublicKeyBySerial(platformCertsDir, serial)
	if err != nil {
		return err
	}
	message := BuildWeChatNotifySignatureMessage(timestamp, nonce, body)
	return VerifyWeChatMessage(message, signature, publicKey)
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func normalizedWeChatCurrency(currency string) string {
	value := strings.ToUpper(strings.TrimSpace(currency))
	if value == "" {
		return "CNY"
	}
	return value
}
