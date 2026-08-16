// Package careerclient is career-mcp's private client for the Career
// Opportunities owner. It mirrors the portal-gateway career contract
// (six-line actor-bound signing, envelope unwrap, verbatim error passthrough)
// without depending on that service's internal packages. The resume upload is
// forwarded as multipart/form-data exactly like the Gateway forwards it.
package careerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"henukit.dev/career-mcp/internal/signing"
)

const (
	extractionsPath  = "/api/v1/career/profile/extractions"
	maxUploadBody    = (10 << 20) + (1 << 20) // 10 MiB file + multipart framing
	maxResponseBytes = 128 << 10
)

// UpstreamError carries a non-2xx Career response so tool handlers can present
// the exact error code (e.g. AI_UNCONFIGURED, EXTRACT_RATE_LIMITED) to the
// model.
type UpstreamError struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("Career responded %d %s: %s", e.StatusCode, e.Code, e.Message)
}

// ErrUnavailable wraps transport-level failures (Career down, unconfigured).
var ErrUnavailable = errors.New("Career service is unavailable")

// ErrInvalidResponse wraps a response that is not a usable Career envelope.
var ErrInvalidResponse = errors.New("Career returned an invalid response")

// Client talks to services/career-opportunities over its signed HTTP contract.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *signing.Signer
}

// NewClient validates the base URL and the Career credential ring. A partially
// configured ring must fail startup rather than leaving tools to fail later;
// public deployment placeholders are rejected at this remote boundary.
func NewClient(baseURL, clientID, secret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid Career client configuration")
	}
	if strings.TrimSpace(clientID) == "" || secret == "" || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("Career credential is incomplete")
	}
	if len(secret) < 32 || isPlaceholderSecret(secret) {
		return nil, errors.New("Career credential secret is not deployment-safe")
	}
	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		signer:     signing.NewSigner(strings.TrimSpace(clientID), secret, strings.TrimSpace(keyID)),
	}, nil
}

func isPlaceholderSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	return strings.HasPrefix(value, "replace-") ||
		strings.HasPrefix(value, "change-me") ||
		strings.HasPrefix(value, "example-") ||
		strings.Contains(value, "placeholder")
}

// CreateExtraction uploads one resume file for the actor and returns the
// queued extraction. The multipart body is built here and signed byte-for-byte
// with the service credential, exactly like the Portal Gateway forwards it.
func (c *Client) CreateExtraction(ctx context.Context, actorUserID, fileName string, content []byte) (map[string]any, error) {
	if actorUserID == "" || strings.TrimSpace(fileName) == "" || len(content) == 0 || len(content) > maxUploadBody {
		return nil, fmt.Errorf("%w: invalid resume upload", ErrInvalidResponse)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("%w: build multipart body", ErrInvalidResponse)
	}
	if _, err := part.Write(content); err != nil {
		return nil, fmt.Errorf("%w: build multipart body", ErrInvalidResponse)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("%w: build multipart body", ErrInvalidResponse)
	}
	return c.call(ctx, http.MethodPost, extractionsPath, actorUserID, body.Bytes(), writer.FormDataContentType())
}

// Extraction reads one actor-scoped extraction status and result.
func (c *Client) Extraction(ctx context.Context, actorUserID, extractionID string) (map[string]any, error) {
	if actorUserID == "" || extractionID == "" {
		return nil, fmt.Errorf("%w: invalid extraction id", ErrInvalidResponse)
	}
	return c.call(ctx, http.MethodGet, extractionsPath+"/"+url.PathEscape(extractionID), actorUserID, nil, "")
}

func (c *Client) call(ctx context.Context, method, path, actorUserID string, body []byte, contentType string) (map[string]any, error) {
	if c == nil || c.baseURL == "" {
		return nil, ErrUnavailable
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if err := c.signer.SignWithActor(request, actorUserID); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("%w: response exceeds the size limit", ErrInvalidResponse)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, parseUpstreamError(response.StatusCode, raw)
	}
	var envelope struct {
		Data      map[string]any `json:"data"`
		RequestID string         `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Data == nil || envelope.RequestID == "" {
		return nil, fmt.Errorf("%w: missing data or request_id", ErrInvalidResponse)
	}
	envelope.Data["request_id"] = envelope.RequestID
	return envelope.Data, nil
}

func parseUpstreamError(status int, raw []byte) error {
	var envelope struct {
		Error     map[string]string `json:"error"`
		RequestID string            `json:"request_id"`
	}
	_ = json.Unmarshal(raw, &envelope)
	code := envelope.Error["code"]
	message := envelope.Error["message"]
	if code == "" {
		code = "UNKNOWN"
	}
	if message == "" {
		message = strconv.Itoa(status)
	}
	return &UpstreamError{StatusCode: status, Code: code, Message: message, RequestID: envelope.RequestID}
}
