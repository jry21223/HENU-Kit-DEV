package library

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
	ErrUnauthorized = errors.New("library rejected service or actor")
	ErrForbidden    = errors.New("library denied permission or scope")
	ErrConflict     = errors.New("library operation conflict")
	ErrInvalid      = errors.New("library request invalid")
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
	host := ""
	if err == nil {
		host = parsed.Hostname()
	}
	ip := net.ParseIP(host)
	loopback := err == nil && parsed.Scheme == "http" && (host == "localhost" || host == "study-api" || host == "platform-core" || host == "portal-api" || strings.HasSuffix(host, ".local") || (ip != nil && ip.IsLoopback()))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !loopback) || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || clientID == "" || len(clientSecret) < 32 || keyID == "" {
		return nil, errors.New("invalid library client configuration")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	signer, err := serviceauth.New(clientID, clientSecret, keyID)
	if err != nil {
		return nil, errors.New("invalid library client configuration")
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), signer: signer, httpClient: httpClient}, nil
}

func (c *Client) Workspace(ctx context.Context, actorID string) (json.RawMessage, error) {
	return c.request(ctx, http.MethodGet, WorkspacePath, actorID, "library.read", "", nil)
}
func (c *Client) Command(ctx context.Context, actorID, key string, body []byte) (json.RawMessage, error) {
	return c.request(ctx, http.MethodPost, CommandPath, actorID, permissionForCommand(body), key, body)
}
func (c *Client) Operation(ctx context.Context, actorID, operation, key string) (json.RawMessage, error) {
	return c.request(ctx, http.MethodGet, strings.ReplaceAll(OperationPath, "{operation}", url.PathEscape(operation)), actorID, permissionForOperation(operation), key, nil)
}

func permissionForCommand(body []byte) string {
	var value struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(body, &value)
	return permissionForOperation(value.Kind)
}

func permissionForOperation(operation string) string {
	if strings.HasPrefix(operation, "submission_") || strings.HasPrefix(operation, "correction_") {
		return "library.review"
	}
	return "library.manage"
}

func (c *Client) request(ctx context.Context, method, path, actorID, permission, key string, body []byte) (json.RawMessage, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if err := c.signer.Sign(request, body); err != nil {
		return nil, err
	}
	request.Header.Set("X-Actor-User-Id", actorID)
	request.Header.Set("X-Permission-Code", permission)
	request.Header.Set("X-Scope-Kind", "product")
	request.Header.Set("X-Product-Code", "library")
	if requestID, _ := ctx.Value(requestIDKey{}).(string); requestID != "" {
		request.Header.Set("X-Request-Id", requestID)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := c.httpClient.Do(request)
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
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return nil, errors.New("invalid library response")
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
	case http.StatusBadRequest:
		return ErrInvalid
	default:
		return fmt.Errorf("library returned %d", status)
	}
}
