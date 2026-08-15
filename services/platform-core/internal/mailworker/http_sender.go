package mailworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"henukit.dev/platform-core/internal/verificationmail"
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
	// Production must use HTTPS. Docker Compose may use http://service:port on the
	// internal network when PLATFORM_CORE_SMTP_ALLOW_DOCKER=1 (single-label host or private IP).
	if parsed.Scheme != "https" && !allowInsecureMailProviderHost(parsed.Hostname()) {
		return nil, errors.New("mail provider endpoint must use HTTPS")
	}
	return &HTTPSender{endpoint: endpoint, token: token, client: client}, nil
}

func allowInsecureMailProviderHost(host string) bool {
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}
	// Compose service DNS names have no dots (e.g. platform-smtp-provider).
	if os.Getenv("PLATFORM_CORE_SMTP_ALLOW_DOCKER") == "1" && host != "" && !strings.Contains(host, ".") {
		return true
	}
	return false
}

// providerVariables is the provider wire shape shared by the verification_code
// and career_digest templates. The verification fields keep their exact names
// and emit order; the digest fields are omitted entirely for verification mail
// so the verification request body stays byte-identical.
type providerVariables struct {
	Code      string `json:"code,omitempty"`
	Purpose   string `json:"purpose,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`

	SearchID     string                 `json:"search_id,omitempty"`
	CompletedAt  string                 `json:"completed_at,omitempty"`
	SourceCount  int                    `json:"source_count,omitempty"`
	JobCount     int                    `json:"job_count,omitempty"`
	MatchedCount int                    `json:"matched_count,omitempty"`
	Summary      string                 `json:"summary,omitempty"`
	CareerURL    string                 `json:"career_url,omitempty"`
	TopJobs      []verificationmail.Job `json:"top_jobs,omitempty"`
}

func (s *HTTPSender) Send(ctx context.Context, message Message) (string, error) {
	template := "henukit_verification_code"
	variables := providerVariables{
		Code: message.Code, Purpose: message.Purpose,
		ExpiresAt: message.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if message.Template == "henukit_career_digest" && message.Digest != nil {
		template = "henukit_career_digest"
		variables = providerVariables{
			SearchID: message.Digest.SearchID, CompletedAt: message.Digest.CompletedAt,
			SourceCount: message.Digest.SourceCount, JobCount: message.Digest.JobCount,
			MatchedCount: message.Digest.MatchedCount, Summary: message.Digest.Summary,
			CareerURL: message.Digest.CareerURL, TopJobs: message.Digest.TopJobs,
		}
	}
	payload, err := json.Marshal(struct {
		Recipient      string            `json:"recipient"`
		Template       string            `json:"template"`
		Variables      providerVariables `json:"variables"`
		RequestID      string            `json:"request_id"`
		IdempotencyKey string            `json:"idempotency_key"`
	}{
		Recipient: message.Recipient, Template: template, RequestID: message.RequestID, IdempotencyKey: message.IdempotencyKey,
		Variables: variables,
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
