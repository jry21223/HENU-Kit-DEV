// Package accountportfolio is Portal Gateway's private client for the Account
// Portfolio owner. It forwards only the authenticated Portal Session actor;
// browser callers never choose an account identity or receive service secrets.
package accountportfolio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

const (
	SummaryPath          = "/api/v1/account/summary"
	PointsPath           = "/api/v1/account/points"
	MembershipPath       = "/api/v1/account/membership"
	NotificationsPath    = "/api/v1/account/notifications"
	TicketsPath          = "/api/v1/account/tickets"
	MembershipOrdersPath = "/api/v1/account/membership-orders"
)

var (
	ErrUnauthorized = errors.New("account portfolio rejected the authenticated actor")
	ErrUnavailable  = errors.New("account portfolio is unavailable")
	ErrInvalid      = errors.New("account portfolio returned an invalid response")
)

// Client is a typed internal boundary for Account Portfolio reads.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
}

// NewClient creates a private owner client. It accepts HTTP only for local
// compose and test origins; public deployments use an isolated Docker network.
func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(clientID) == "" || len(clientSecret) < 32 || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("invalid Account Portfolio client configuration")
	}
	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}, nil
}

func (c *Client) Summary(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.get(ctx, SummaryPath, actorUserID, requestID)
}

func (c *Client) Points(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.get(ctx, PointsPath, actorUserID, requestID)
}

func (c *Client) Membership(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.get(ctx, MembershipPath, actorUserID, requestID)
}

func (c *Client) Notifications(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.get(ctx, NotificationsPath, actorUserID, requestID)
}

func (c *Client) Tickets(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.get(ctx, TicketsPath, actorUserID, requestID)
}

func (c *Client) MembershipOrders(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.get(ctx, MembershipOrdersPath, actorUserID, requestID)
}

func (c *Client) get(ctx context.Context, path, actorUserID, requestID string) (json.RawMessage, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || strings.TrimSpace(actorUserID) == "" || strings.TrimSpace(requestID) == "" {
		return nil, ErrUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create Account Portfolio request: %w", ErrUnavailable)
	}
	request.Header.Set("X-Request-Id", requestID)
	if err := c.signer.SignWithActor(request, actorUserID); err != nil {
		return nil, fmt.Errorf("sign Account Portfolio request: %w", ErrUnavailable)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Account Portfolio: %w", ErrUnavailable)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("account portfolio status %d: %w", response.StatusCode, ErrUnavailable)
	}

	var envelope struct {
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" || strings.TrimSpace(envelope.RequestID) == "" {
		return nil, ErrInvalid
	}
	if err := validateData(path, envelope.Data); err != nil {
		return nil, ErrInvalid
	}
	return envelope.Data, nil
}
