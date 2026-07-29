// Package accountportfolio is Portal Gateway's private client for the Account
// Portfolio owner. It forwards only the authenticated Portal Session actor;
// browser callers never choose an account identity or receive service secrets.
package accountportfolio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

func TicketPath(ticketID string) string {
	return TicketsPath + "/" + ticketID
}

func TicketFollowUpsPath(ticketID string) string {
	return TicketPath(ticketID) + "/follow-ups"
}

func NotificationReadPath(notificationID string) string {
	return NotificationsPath + "/" + notificationID + "/read"
}

var (
	ErrUnauthorized = errors.New("account portfolio rejected the authenticated actor")
	ErrBadRequest   = errors.New("account portfolio rejected the command")
	ErrNotFound     = errors.New("account portfolio resource was not found")
	ErrConflict     = errors.New("account portfolio command conflicted")
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
	return c.PointsPage(ctx, actorUserID, requestID, "", 20)
}

// PointsPage forwards an opaque owner cursor and a bounded page size. The
// request path deliberately retains its query string for HMAC signing, while
// response validation remains bound to the static owner route.
func (c *Client) PointsPage(ctx context.Context, actorUserID, requestID, cursor string, limit int) (json.RawMessage, error) {
	if limit < 1 || limit > 50 || len(cursor) > 512 || (cursor != "" && strings.TrimSpace(cursor) != cursor) {
		return nil, ErrBadRequest
	}
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	return c.getAt(ctx, PointsPath+"?"+query.Encode(), PointsPath, actorUserID, requestID)
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

func (c *Client) Ticket(ctx context.Context, actorUserID, requestID, ticketID string) (json.RawMessage, error) {
	if !validUUID(ticketID) {
		return nil, ErrBadRequest
	}
	return c.get(ctx, TicketPath(ticketID), actorUserID, requestID)
}

func (c *Client) CreateTicket(ctx context.Context, actorUserID, requestID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	return c.command(ctx, TicketsPath, actorUserID, requestID, idempotencyKey, raw, http.StatusCreated)
}

func (c *Client) FollowUp(ctx context.Context, actorUserID, requestID, ticketID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	if !validUUID(ticketID) {
		return nil, ErrBadRequest
	}
	return c.command(ctx, TicketFollowUpsPath(ticketID), actorUserID, requestID, idempotencyKey, raw, http.StatusOK)
}

func (c *Client) MarkNotificationRead(ctx context.Context, actorUserID, requestID, notificationID, idempotencyKey string) (json.RawMessage, error) {
	if !validUUID(notificationID) {
		return nil, ErrBadRequest
	}
	return c.command(ctx, NotificationReadPath(notificationID), actorUserID, requestID, idempotencyKey, nil, http.StatusOK)
}

func (c *Client) MembershipOrders(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	return c.get(ctx, MembershipOrdersPath, actorUserID, requestID)
}

func (c *Client) get(ctx context.Context, path, actorUserID, requestID string) (json.RawMessage, error) {
	return c.getAt(ctx, path, path, actorUserID, requestID)
}

func (c *Client) getAt(ctx context.Context, requestPath, validationPath, actorUserID, requestID string) (json.RawMessage, error) {
	return c.call(ctx, http.MethodGet, requestPath, validationPath, actorUserID, requestID, "", nil, http.StatusOK, validateData)
}

func (c *Client) command(ctx context.Context, path, actorUserID, requestID, idempotencyKey string, raw []byte, expectedStatus int) (json.RawMessage, error) {
	if !validIdempotencyKey(idempotencyKey) || len(raw) > 64<<10 {
		return nil, ErrBadRequest
	}
	return c.call(ctx, http.MethodPost, path, path, actorUserID, requestID, idempotencyKey, raw, expectedStatus, validateCommandData)
}

type responseValidator func(string, json.RawMessage) error

func (c *Client) call(ctx context.Context, method, requestPath, validationPath, actorUserID, requestID, idempotencyKey string, raw []byte, expectedStatus int, validate responseValidator) (json.RawMessage, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || strings.TrimSpace(actorUserID) == "" || strings.TrimSpace(requestID) == "" {
		return nil, ErrUnavailable
	}
	var body io.Reader
	if raw != nil {
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, body)
	if err != nil {
		return nil, fmt.Errorf("create Account Portfolio request: %w", ErrUnavailable)
	}
	request.Header.Set("X-Request-Id", requestID)
	if raw != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if err := c.signer.SignWithActor(request, actorUserID); err != nil {
		return nil, fmt.Errorf("sign Account Portfolio request: %w", ErrUnavailable)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Account Portfolio: %w", ErrUnavailable)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case expectedStatus:
	case http.StatusBadRequest:
		return nil, ErrBadRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusConflict:
		return nil, ErrConflict
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
	if err := validate(validationPath, envelope.Data); err != nil {
		return nil, ErrInvalid
	}
	return envelope.Data, nil
}
