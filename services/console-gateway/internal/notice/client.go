package notice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"henukit.dev/console-gateway/internal/serviceauth"
)

var (
	ErrUnauthorized = errors.New("notice rejected service or actor")
	ErrForbidden    = errors.New("notice denied permission or scope")
	ErrConflict     = errors.New("notice operation conflict")
	ErrNotFound     = errors.New("notice resource not found")
	ErrInvalid      = errors.New("notice request invalid")
)

type Client struct {
	baseURL    string
	signer     *serviceauth.Signer
	httpClient *http.Client
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func New(baseURL, clientID, clientSecret, keyID string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	loopback := err == nil && parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !loopback) || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || clientID == "" || len(clientSecret) < 32 || keyID == "" {
		return nil, errors.New("invalid Notice client configuration")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	signer, err := serviceauth.New(clientID, clientSecret, keyID)
	if err != nil {
		return nil, errors.New("invalid Notice client configuration")
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), signer: signer, httpClient: httpClient}, nil
}

func (c *Client) Snapshot(ctx context.Context, actorID string) (json.RawMessage, error) {
	return c.request(ctx, http.MethodGet, SnapshotPath, actorID, "notice.read", "", nil)
}
func (c *Client) CreateSource(ctx context.Context, actorID, key string, body []byte) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPost, SourcePath, actorID, "notice.manage", key, body)
}
func (c *Client) CreateVersion(ctx context.Context, actorID, sourceID, key string, body []byte) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPost, strings.ReplaceAll(VersionPath, "{source_id}", sourceID), actorID, "notice.manage", key, body)
}
func (c *Client) Review(ctx context.Context, actorID, versionID, key string, body []byte) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPost, strings.ReplaceAll(ReviewPath, "{version_id}", versionID), actorID, "notice.review", key, body)
}
func (c *Client) Distribute(ctx context.Context, actorID, versionID, key string, body []byte) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPost, strings.ReplaceAll(DistributionPath, "{version_id}", versionID), actorID, "notice.distribute", key, body)
}
func (c *Client) Operation(ctx context.Context, actorID, operation, key string) (json.RawMessage, error) {
	return c.request(ctx, http.MethodGet, strings.ReplaceAll(OperationPath, "{operation}", operation), actorID, "notice.read", key, nil)
}

func (c *Client) request(ctx context.Context, method, path, actorID, permission, key string, body []byte) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.signer.Sign(req, body); err != nil {
		return nil, err
	}
	req.Header.Set("X-Actor-User-Id", actorID)
	req.Header.Set("X-Permission-Code", permission)
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "notice")
	if requestID, _ := ctx.Value(requestIDKey{}).(string); requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if err := responseError(response.StatusCode); err != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return nil, errors.New("invalid Notice response")
	}
	return envelope.Data, nil
}

func responseError(status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusConflict:
		return ErrConflict
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusBadRequest:
		return ErrInvalid
	default:
		return fmt.Errorf("notice returned %d", status)
	}
}
