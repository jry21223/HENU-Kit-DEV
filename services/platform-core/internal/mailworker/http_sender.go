package mailworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type HTTPSender struct {
	endpoint string
	token    string
	client   *http.Client
}

func NewHTTPSender(endpoint, token string, client *http.Client) (*HTTPSender, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || token == "" || client == nil {
		return nil, errors.New("mail provider endpoint, token, and HTTP client are required")
	}
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return nil, errors.New("mail provider endpoint must use HTTPS")
	}
	return &HTTPSender{endpoint: endpoint, token: token, client: client}, nil
}

func (s *HTTPSender) Send(ctx context.Context, message Message) (string, error) {
	payload, err := json.Marshal(struct {
		Recipient string `json:"recipient"`
		Template  string `json:"template"`
		Variables struct {
			Code      string `json:"code"`
			Purpose   string `json:"purpose"`
			ExpiresAt string `json:"expires_at"`
		} `json:"variables"`
		RequestID      string `json:"request_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}{
		Recipient: message.Recipient, Template: "henukit_verification_code", RequestID: message.RequestID, IdempotencyKey: message.IdempotencyKey,
		Variables: struct {
			Code      string `json:"code"`
			Purpose   string `json:"purpose"`
			ExpiresAt string `json:"expires_at"`
		}{Code: message.Code, Purpose: message.Purpose, ExpiresAt: message.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00")},
	})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+s.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", message.IdempotencyKey)
	request.Header.Set("X-Request-ID", message.RequestID)
	attempt := message.AttemptCount
	if attempt < 1 {
		attempt = 1
	}
	request.Header.Set("X-Mail-Attempt", strconv.FormatInt(int64(attempt), 10))
	response, err := s.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		permanent := response.StatusCode >= 400 && response.StatusCode < 500 &&
			response.StatusCode != http.StatusRequestTimeout &&
			response.StatusCode != http.StatusConflict &&
			response.StatusCode != http.StatusTooManyRequests
		return "", &SendError{Code: "PROVIDER_REJECTED", Permanent: permanent}
	}
	var accepted struct {
		MessageID string `json:"message_id"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 16<<10))
	if err := decoder.Decode(&accepted); err != nil || accepted.MessageID == "" {
		return "", &SendError{Code: "PROVIDER_RESPONSE_INVALID", Permanent: false}
	}
	return accepted.MessageID, nil
}
