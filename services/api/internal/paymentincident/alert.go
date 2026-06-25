package paymentincident

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"final-review-platform/services/api/internal/platform/model"
	"final-review-platform/services/api/pkg/config"
)

const (
	EventOpened    = "payment_incident.opened"
	EventRealerted = "payment_incident.realerted"
)

var (
	ErrWebhookNotConfigured = errors.New("payment_incident_webhook_not_configured")
	ErrInvalidWebhookURL    = errors.New("payment_incident_webhook_invalid_url")
)

type SendResult struct {
	Sent       bool
	StatusCode int
}

type alertPayload struct {
	Event       string        `json:"event"`
	Provider    string        `json:"provider"`
	Environment string        `json:"environment"`
	Incident    alertIncident `json:"incident"`
}

type alertIncident struct {
	ID             string    `json:"id"`
	OrderID        *string   `json:"orderId,omitempty"`
	IncidentType   string    `json:"incidentType"`
	Severity       string    `json:"severity"`
	Status         string    `json:"status"`
	OutTradeNo     string    `json:"outTradeNo"`
	TransactionID  string    `json:"transactionId"`
	TradeState     string    `json:"tradeState"`
	ExpectedAmount int64     `json:"expectedAmount"`
	ActualAmount   int64     `json:"actualAmount"`
	Message        string    `json:"message"`
	CreatedAt      time.Time `json:"createdAt"`
}

func SendAlert(ctx context.Context, alertCfg config.PaymentIncidentAlertConfig, environment string, event string, incident model.PaymentIncident) (SendResult, error) {
	endpoint := strings.TrimSpace(alertCfg.WebhookURL)
	if endpoint == "" {
		return SendResult{}, ErrWebhookNotConfigured
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return SendResult{}, ErrInvalidWebhookURL
	}
	if strings.TrimSpace(event) == "" {
		event = EventOpened
	}
	timeoutSeconds := alertCfg.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}
	if timeoutSeconds > 10 {
		timeoutSeconds = 10
	}
	provider := strings.TrimSpace(incident.Provider)
	if provider == "" {
		provider = "wechat_native"
	}
	payload := alertPayload{
		Event:       event,
		Provider:    provider,
		Environment: strings.TrimSpace(environment),
		Incident: alertIncident{
			ID:             incident.ID,
			OrderID:        incident.OrderID,
			IncidentType:   incident.IncidentType,
			Severity:       incident.Severity,
			Status:         incident.Status,
			OutTradeNo:     incident.OutTradeNo,
			TransactionID:  incident.TransactionID,
			TradeState:     incident.TradeState,
			ExpectedAmount: incident.ExpectedAmount,
			ActualAmount:   incident.ActualAmount,
			Message:        incident.Message,
			CreatedAt:      incident.CreatedAt,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return SendResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SendResult{}, err
	}
	timestamp := fmt.Sprintf("%d", time.Now().UTC().Unix())
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "final-review-platform-payment-alert/1.0")
	request.Header.Set("X-Final-Review-Event", payload.Event)
	request.Header.Set("X-Final-Review-Incident-Id", incident.ID)
	request.Header.Set("X-Final-Review-Timestamp", timestamp)
	if secret := strings.TrimSpace(alertCfg.WebhookSecret); secret != "" {
		request.Header.Set("X-Final-Review-Signature", Signature(secret, timestamp, body))
	}
	client := http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return SendResult{}, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return SendResult{StatusCode: response.StatusCode}, fmt.Errorf("payment incident webhook returned %d", response.StatusCode)
	}
	return SendResult{Sent: true, StatusCode: response.StatusCode}, nil
}

func Signature(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
