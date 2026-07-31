package accountportfolio

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const easyPayProviderName = "easypay"

// EasyPayConfig binds Account Portfolio to its dedicated HENU Kit tenant on
// the existing EasyPay-compatible gateway. The key is never shared with a
// browser or another gateway tenant.
type EasyPayConfig struct {
	BaseURL    string
	PID        string
	Key        string
	NotifyURL  string
	ReturnURL  string
	HTTPClient *http.Client
}

type EasyPayProvider struct {
	baseURL   *url.URL
	pid       string
	key       string
	notifyURL string
	returnURL string
	client    *http.Client
	now       func() time.Time
}

func NewEasyPayProvider(config EasyPayConfig) (*EasyPayProvider, error) {
	baseURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"))
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "https" && !isLoopbackHTTP(baseURL)) {
		return nil, errors.New("EasyPay base URL must use HTTPS")
	}
	notifyURL, notifyErr := url.Parse(strings.TrimSpace(config.NotifyURL))
	returnURL, returnErr := url.Parse(strings.TrimSpace(config.ReturnURL))
	if strings.TrimSpace(config.PID) == "" || strings.TrimSpace(config.Key) == "" ||
		notifyErr != nil || notifyURL.Scheme != "https" || notifyURL.Host == "" ||
		returnErr != nil || returnURL.Scheme != "https" || returnURL.Host == "" {
		return nil, errors.New("EasyPay HENU tenant configuration is incomplete")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &EasyPayProvider{
		baseURL:   baseURL,
		pid:       config.PID,
		key:       config.Key,
		notifyURL: notifyURL.String(),
		returnURL: returnURL.String(),
		client:    client,
		now:       time.Now,
	}, nil
}

func isLoopbackHTTP(value *url.URL) bool {
	if value == nil || value.Scheme != "http" {
		return false
	}
	host := value.Hostname()
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func (p *EasyPayProvider) Name() string { return easyPayProviderName }

func (p *EasyPayProvider) Sign(_ context.Context, request PaymentOrderRequest) (SignedPaymentOrder, error) {
	if p == nil || !validMembershipMerchantOrderID(request.MerchantOrderID) ||
		request.AmountCents != lifetimeMembershipAmountCents ||
		request.Currency != lifetimeMembershipCurrency || request.Plan != lifetimeMembershipPlan {
		return SignedPaymentOrder{}, errors.New("EasyPay payment order is invalid")
	}
	params := p.createParams(request)
	return SignedPaymentOrder{Request: request, Signature: easyPaySign(params, p.key)}, nil
}

func (p *EasyPayProvider) CreateOrder(ctx context.Context, signed SignedPaymentOrder) (ProviderOrder, error) {
	if p == nil {
		return ProviderOrder{}, errors.New("EasyPay provider is unavailable")
	}
	expected, err := p.Sign(ctx, signed.Request)
	if err != nil {
		return ProviderOrder{}, err
	}
	params := p.createParams(signed.Request)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signed.Signature)), []byte(expected.Signature)) != 1 {
		return ProviderOrder{}, errors.New("EasyPay payment signature is invalid")
	}
	params["sign"] = signed.Signature
	params["sign_type"] = "MD5"
	raw, err := json.Marshal(params)
	if err != nil {
		return ProviderOrder{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/submit.php"), bytes.NewReader(raw))
	if err != nil {
		return ProviderOrder{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return ProviderOrder{}, fmt.Errorf("create EasyPay order: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return ProviderOrder{}, fmt.Errorf("read EasyPay create response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return ProviderOrder{}, fmt.Errorf("create EasyPay order: status %d", response.StatusCode)
	}
	var result struct {
		Code    int    `json:"code"`
		CodeURL string `json:"code_url"`
	}
	if json.Unmarshal(body, &result) != nil || result.Code != 1 || strings.TrimSpace(result.CodeURL) == "" {
		return ProviderOrder{}, errors.New("EasyPay create response is invalid")
	}
	return providerOrderFor(signed.Request.MerchantOrderID, MembershipOrderPendingPayment), nil
}

func (p *EasyPayProvider) QueryOrder(ctx context.Context, externalOrderID string) (ProviderOrder, error) {
	if p == nil || !validMembershipMerchantOrderID(externalOrderID) {
		return ProviderOrder{}, errors.New("EasyPay merchant order is invalid")
	}
	params := map[string]string{"pid": p.pid, "out_trade_no": externalOrderID}
	params["sign"] = easyPaySign(params, p.key)
	params["sign_type"] = "MD5"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/api/query.php"), strings.NewReader(url.Values{
		"pid":          {params["pid"]},
		"out_trade_no": {params["out_trade_no"]},
		"sign":         {params["sign"]},
		"sign_type":    {"MD5"},
	}.Encode()))
	if err != nil {
		return ProviderOrder{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := p.client.Do(request)
	if err != nil {
		return ProviderOrder{}, fmt.Errorf("query EasyPay order: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ProviderOrder{}, fmt.Errorf("query EasyPay order: status %d", response.StatusCode)
	}
	var result map[string]string
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil ||
		result["pid"] != p.pid || result["out_trade_no"] != externalOrderID ||
		result["type"] != "wxpay" ||
		!easyPayVerify(result, p.key) {
		return ProviderOrder{}, errors.New("EasyPay query response is invalid")
	}
	amount, err := easyPayCents(result["money"])
	if err != nil || amount != lifetimeMembershipAmountCents {
		return ProviderOrder{}, errors.New("EasyPay query amount is invalid")
	}
	status, err := easyPayStatus(result["trade_status"])
	if err != nil {
		return ProviderOrder{}, err
	}
	return providerOrderFor(externalOrderID, status), nil
}

func (p *EasyPayProvider) VerifyNotification(_ context.Context, raw []byte) (VerifiedPaymentNotification, error) {
	if p == nil || len(raw) == 0 {
		return VerifiedPaymentNotification{}, errors.New("EasyPay notification is invalid")
	}
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return VerifiedPaymentNotification{}, errors.New("EasyPay notification is invalid")
	}
	params := firstEasyPayValues(values)
	if params["pid"] != p.pid || params["type"] != "wxpay" || params["sign_type"] != "MD5" || !easyPayVerify(params, p.key) ||
		!validMembershipMerchantOrderID(params["out_trade_no"]) {
		return VerifiedPaymentNotification{}, errors.New("EasyPay notification is invalid")
	}
	amount, err := easyPayCents(params["money"])
	if err != nil || amount != lifetimeMembershipAmountCents {
		return VerifiedPaymentNotification{}, errors.New("EasyPay notification amount is invalid")
	}
	status, err := easyPayStatus(params["trade_status"])
	if err != nil || status != MembershipOrderPaid {
		return VerifiedPaymentNotification{}, errors.New("EasyPay notification status is invalid")
	}
	tradeID := strings.TrimSpace(params["trade_no"])
	if tradeID == "" || len(tradeID) > 200 {
		return VerifiedPaymentNotification{}, errors.New("EasyPay notification transaction is invalid")
	}
	occurredAt := p.now().UTC()
	if value := strings.TrimSpace(params["paid_at"]); value != "" {
		parsed, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return VerifiedPaymentNotification{}, errors.New("EasyPay notification paid_at is invalid")
		}
		occurredAt = parsed.UTC()
	}
	merchantOrderID := params["out_trade_no"]
	return VerifiedPaymentNotification{
		EventID:         "easypay:" + tradeID + ":paid",
		ExternalOrderID: merchantOrderID,
		MerchantOrderID: merchantOrderID,
		AmountCents:     amount,
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
		Status:          status,
		Sequence:        1,
		OccurredAt:      occurredAt,
	}, nil
}

// Refund submits the full lifetime-membership refund for one HENU merchant
// order. The refund correlation is derived deterministically from that order, so
// a retry reuses it and the gateway settles on a single refund rather than
// issuing a second one. The amount is never sent: the gateway takes it from the
// stored order, so a caller cannot influence how much is refunded.
func (p *EasyPayProvider) Refund(ctx context.Context, externalOrderID string) (PaymentRefund, error) {
	if p == nil || !validMembershipMerchantOrderID(externalOrderID) {
		return PaymentRefund{}, errors.New("EasyPay merchant order is invalid")
	}
	refundID := membershipRefundOrderID(externalOrderID)
	params := map[string]string{
		"pid":           p.pid,
		"out_trade_no":  externalOrderID,
		"out_refund_no": refundID,
	}
	params["sign"] = easyPaySign(params, p.key)
	result, err := p.postSigned(ctx, "/api/refund.php", params)
	if err != nil {
		return PaymentRefund{}, err
	}
	return p.refundFrom(externalOrderID, refundID, result)
}

// QueryRefund reconciles a previously submitted refund against the gateway's
// authoritative refund query, so a refund that was still processing can be
// settled later without re-submitting it.
func (p *EasyPayProvider) QueryRefund(ctx context.Context, externalOrderID string) (PaymentRefund, error) {
	if p == nil || !validMembershipMerchantOrderID(externalOrderID) {
		return PaymentRefund{}, errors.New("EasyPay merchant order is invalid")
	}
	refundID := membershipRefundOrderID(externalOrderID)
	params := map[string]string{
		"pid":           p.pid,
		"out_trade_no":  externalOrderID,
		"out_refund_no": refundID,
	}
	params["sign"] = easyPaySign(params, p.key)
	result, err := p.postSigned(ctx, "/api/refund-query.php", params)
	if err != nil {
		return PaymentRefund{}, err
	}
	return p.refundFrom(externalOrderID, refundID, result)
}

func (p *EasyPayProvider) postSigned(ctx context.Context, path string, params map[string]string) (map[string]string, error) {
	form := url.Values{"sign_type": {"MD5"}}
	for key, value := range params {
		form.Set(key, value)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(path), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call EasyPay %s: %w", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("call EasyPay %s: status %d", path, response.StatusCode)
	}
	var result map[string]string
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil {
		return nil, fmt.Errorf("call EasyPay %s: invalid response", path)
	}
	return result, nil
}

// refundFrom validates that a gateway refund response is signed by this tenant's
// key and describes this exact order and refund before any of it is believed.
func (p *EasyPayProvider) refundFrom(externalOrderID, refundID string, result map[string]string) (PaymentRefund, error) {
	if result["pid"] != p.pid || result["out_trade_no"] != externalOrderID ||
		result["out_refund_no"] != refundID || !easyPayVerify(result, p.key) {
		return PaymentRefund{}, errors.New("EasyPay refund response is invalid")
	}
	amount, err := easyPayCents(result["money"])
	if err != nil || amount != lifetimeMembershipAmountCents {
		return PaymentRefund{}, errors.New("EasyPay refund amount is invalid")
	}
	settled, err := easyPayRefundSettled(result["refund_status"])
	if err != nil {
		return PaymentRefund{}, err
	}
	// A refund that has not settled leaves the order paid. Only a settled refund
	// is a refund fact, and only a refund fact may revoke an entitlement.
	status := MembershipOrderPaid
	if settled {
		status = MembershipOrderRefunded
	}
	return PaymentRefund{
		Notification: VerifiedPaymentNotification{
			EventID:         fmt.Sprintf("easypay-refund-%s", refundID),
			ExternalOrderID: externalOrderID,
			MerchantOrderID: externalOrderID,
			AmountCents:     amount,
			Currency:        "CNY",
			Plan:            lifetimeMembershipPlan,
			Status:          status,
			OccurredAt:      p.now().UTC(),
		},
		RefundID: refundID,
		Settled:  settled,
	}, nil
}

// easyPayRefundSettled maps the gateway's reconciled refund status. `abnormal`
// is deliberately an error: it needs operator handling and must never be read as
// either a completed or a harmlessly pending refund.
func easyPayRefundSettled(value string) (bool, error) {
	switch strings.TrimSpace(value) {
	case "succeeded":
		return true, nil
	case "processing":
		return false, nil
	case "closed":
		return false, nil
	case "abnormal":
		return false, errors.New("EasyPay refund is abnormal and needs operator handling")
	default:
		return false, errors.New("EasyPay refund status is invalid")
	}
}

func (p *EasyPayProvider) createParams(request PaymentOrderRequest) map[string]string {
	return map[string]string{
		"pid":          p.pid,
		"type":         "wxpay",
		"out_trade_no": request.MerchantOrderID,
		"notify_url":   p.notifyURL,
		"return_url":   p.returnURL,
		"name":         "HENU Kit 终身会员",
		"money":        "9.90",
	}
}

func (p *EasyPayProvider) endpoint(path string) string {
	value := *p.baseURL
	value.Path = strings.TrimRight(value.Path, "/") + path
	return value.String()
}

func providerOrderFor(merchantOrderID string, status MembershipOrderStatus) ProviderOrder {
	return ProviderOrder{
		ExternalOrderID: merchantOrderID,
		MerchantOrderID: merchantOrderID,
		AmountCents:     lifetimeMembershipAmountCents,
		Currency:        lifetimeMembershipCurrency,
		Plan:            lifetimeMembershipPlan,
		Status:          status,
	}
}

func easyPaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if name != "sign" && name != "sign_type" && value != "" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	var input strings.Builder
	for index, name := range keys {
		if index > 0 {
			input.WriteByte('&')
		}
		input.WriteString(name)
		input.WriteByte('=')
		input.WriteString(params[name])
	}
	input.WriteString(key)
	digest := md5.Sum([]byte(input.String())) // EasyPay compatibility contract.
	return hex.EncodeToString(digest[:])
}

func easyPayVerify(params map[string]string, key string) bool {
	received := strings.ToLower(strings.TrimSpace(params["sign"]))
	expected := easyPaySign(params, key)
	return len(received) == len(expected) && subtle.ConstantTimeCompare([]byte(received), []byte(expected)) == 1
}

func firstEasyPayValues(values url.Values) map[string]string {
	result := make(map[string]string, len(values))
	for name, candidates := range values {
		if len(candidates) > 0 {
			result[name] = candidates[0]
		}
	}
	return result
}

func easyPayCents(value string) (int, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[1]) != 2 {
		return 0, errors.New("invalid EasyPay amount")
	}
	yuan, err := strconv.Atoi(parts[0])
	if err != nil || yuan < 0 {
		return 0, errors.New("invalid EasyPay amount")
	}
	fen, err := strconv.Atoi(parts[1])
	if err != nil || fen < 0 || fen > 99 {
		return 0, errors.New("invalid EasyPay amount")
	}
	return yuan*100 + fen, nil
}

func easyPayStatus(value string) (MembershipOrderStatus, error) {
	switch value {
	case "WAIT_BUYER_PAY":
		return MembershipOrderPendingPayment, nil
	case "TRADE_SUCCESS":
		return MembershipOrderPaid, nil
	case "TRADE_CLOSED":
		return MembershipOrderClosed, nil
	default:
		return "", errors.New("EasyPay order status is invalid")
	}
}
