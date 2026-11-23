package career

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"henukit.dev/console-gateway/internal/clientutil"
	"henukit.dev/console-gateway/internal/serviceauth"
)

var (
	ErrUnauthorized = errors.New("career rejected service or actor")
	ErrForbidden    = errors.New("career denied permission or scope")
	ErrNotFound     = errors.New("career resource not found")
	ErrConflict     = errors.New("career operation conflict")
	ErrInvalid      = errors.New("career request invalid")
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
	loopback := err == nil && clientutil.IsTrustedLoopbackHTTP(parsed, "career-opportunities")
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !loopback) || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || clientID == "" || len(clientSecret) < 32 || keyID == "" {
		return nil, errors.New("invalid career console client configuration")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	signer, err := serviceauth.New(clientID, clientSecret, keyID)
	if err != nil {
		return nil, errors.New("invalid career console client configuration")
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), signer: signer, httpClient: httpClient}, nil
}

func (c *Client) Workspace(ctx context.Context, actorID string) (json.RawMessage, error) {
	return c.request(ctx, http.MethodGet, "/api/v1/console/workspace", actorID, "career.read", "", nil)
}

func (c *Client) RetryDigest(ctx context.Context, actorID, searchID, idempotencyKey string) (json.RawMessage, error) {
	path := "/api/v1/console/searches/" + url.PathEscape(searchID) + "/retry-digest"
	return c.request(ctx, http.MethodPost, path, actorID, "career.manage", idempotencyKey, []byte("{}"))
}

func (c *Client) request(ctx context.Context, method, path, actorID, permission, key string, body []byte) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.signer.SignWithActor(req, body, actorID); err != nil {
		return nil, err
	}
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "career")
	req.Header.Set("X-Permission-Code", permission)
	if requestID, _ := ctx.Value(requestIDKey{}).(string); requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := responseError(resp.StatusCode); err != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return nil, err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return nil, errors.New("invalid career response")
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
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	case http.StatusBadRequest:
		return ErrInvalid
	default:
		return fmt.Errorf("career returned %d", status)
	}
}
