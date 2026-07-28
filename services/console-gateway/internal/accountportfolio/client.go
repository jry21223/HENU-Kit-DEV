// Package accountportfolio is Console Gateway's private client for the
// Account Portfolio owner. It signs the Console Session operator as the
// actor; browsers cannot select an operator identity or receive the owner key.
package accountportfolio

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

	"github.com/google/uuid"

	"henukit.dev/console-gateway/internal/serviceauth"
)

var (
	ErrUnauthorized = errors.New("account portfolio rejected the Console actor")
	ErrForbidden    = errors.New("account portfolio denied the Console caller")
	ErrConflict     = errors.New("account portfolio ticket command conflicted")
	ErrNotFound     = errors.New("account portfolio ticket was not found")
	ErrInvalid      = errors.New("account portfolio request or response is invalid")
	ErrUnavailable  = errors.New("account portfolio is unavailable")
)

type Client struct {
	baseURL    string
	signer     *serviceauth.Signer
	httpClient *http.Client
}

type requestIDKey struct{}

// WithRequestID preserves Console Gateway's trace identifier across this
// controlled owner boundary without trusting a browser-generated actor.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// New creates a private client. HTTP is limited to local compose and test
// origins; public owner endpoints must use HTTPS.
func New(baseURL, clientID, clientSecret, keyID string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	host := ""
	if err == nil {
		host = parsed.Hostname()
	}
	ip := net.ParseIP(host)
	loopback := err == nil && parsed.Scheme == "http" && (host == "localhost" || host == "account-portfolio" || strings.HasSuffix(host, ".local") || (ip != nil && ip.IsLoopback()))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !loopback) || strings.TrimSpace(clientID) == "" || len(clientSecret) < 32 || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("invalid Account Portfolio Console client configuration")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	signer, err := serviceauth.New(clientID, clientSecret, keyID)
	if err != nil {
		return nil, errors.New("invalid Account Portfolio Console client configuration")
	}
	return &Client{baseURL: strings.TrimRight(parsed.String(), "/"), signer: signer, httpClient: httpClient}, nil
}

func TicketPath(ticketID string) string {
	return strings.ReplaceAll(TicketPathTemplate, "{ticket_id}", url.PathEscape(ticketID))
}

func TicketRepliesPath(ticketID string) string {
	return strings.ReplaceAll(TicketRepliesPathTemplate, "{ticket_id}", url.PathEscape(ticketID))
}

func TicketTransitionsPath(ticketID string) string {
	return strings.ReplaceAll(TicketTransitionsPathTemplate, "{ticket_id}", url.PathEscape(ticketID))
}

func (c *Client) Tickets(ctx context.Context, actorUserID string) (json.RawMessage, error) {
	return c.get(ctx, TicketsPath, actorUserID, validateTicketQueue)
}

func (c *Client) Ticket(ctx context.Context, actorUserID, ticketID string) (json.RawMessage, error) {
	if !validUUID(ticketID) {
		return nil, ErrInvalid
	}
	return c.get(ctx, TicketPath(ticketID), actorUserID, validateTicketDetail)
}

func (c *Client) Reply(ctx context.Context, actorUserID, ticketID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	if !validUUID(ticketID) {
		return nil, ErrInvalid
	}
	return c.command(ctx, TicketRepliesPath(ticketID), actorUserID, idempotencyKey, raw)
}

func (c *Client) Transition(ctx context.Context, actorUserID, ticketID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	if !validUUID(ticketID) {
		return nil, ErrInvalid
	}
	return c.command(ctx, TicketTransitionsPath(ticketID), actorUserID, idempotencyKey, raw)
}

func (c *Client) get(ctx context.Context, path, actorUserID string, validate responseValidator) (json.RawMessage, error) {
	return c.call(ctx, http.MethodGet, path, actorUserID, "", nil, http.StatusOK, validate)
}

func (c *Client) command(ctx context.Context, path, actorUserID, idempotencyKey string, raw []byte) (json.RawMessage, error) {
	if !validIdempotencyKey(idempotencyKey) || len(raw) == 0 || len(raw) > 64<<10 {
		return nil, ErrInvalid
	}
	return c.call(ctx, http.MethodPost, path, actorUserID, idempotencyKey, raw, http.StatusOK, validateTicketCommand)
}

type responseValidator func(json.RawMessage) error

func (c *Client) call(ctx context.Context, method, path, actorUserID, idempotencyKey string, raw []byte, expectedStatus int, validate responseValidator) (json.RawMessage, error) {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	if c == nil || c.signer == nil || c.httpClient == nil || !validUUID(actorUserID) || strings.TrimSpace(requestID) == "" || validate == nil {
		return nil, ErrUnavailable
	}
	var body io.Reader
	if raw != nil {
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create Account Portfolio Console request: %w", ErrUnavailable)
	}
	request.Header.Set("X-Request-Id", requestID)
	if raw != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if err := c.signer.SignWithActor(request, raw, actorUserID); err != nil {
		return nil, fmt.Errorf("sign Account Portfolio Console request: %w", ErrUnavailable)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Account Portfolio: %w", ErrUnavailable)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case expectedStatus:
	case http.StatusBadRequest:
		return nil, ErrInvalid
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusForbidden:
		return nil, ErrForbidden
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusConflict:
		return nil, ErrConflict
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("account portfolio returned %d: %w", response.StatusCode, ErrUnavailable)
	}
	var envelope struct {
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&envelope); err != nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" || strings.TrimSpace(envelope.RequestID) == "" {
		return nil, ErrInvalid
	}
	if err := validate(envelope.Data); err != nil {
		return nil, ErrInvalid
	}
	return envelope.Data, nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func validIdempotencyKey(value string) bool {
	if len(value) < 8 || len(value) > 200 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == ':' || char == '-' {
			continue
		}
		return false
	}
	return true
}
