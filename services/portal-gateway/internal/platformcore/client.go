package platformcore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

// Client communicates with Platform Core for auth and authorization.
type Client struct {
	baseURL     string
	redirectURI string
	clientID    string
	httpClient  *http.Client
	signer      *serviceauth.Signer
}

// NewClient creates a Platform Core client.
func NewClient(baseURL, redirectURI, clientID, clientSecret, keyID string) *Client {
	return &Client{
		baseURL:     baseURL,
		redirectURI: redirectURI,
		clientID:    clientID,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		signer:      serviceauth.NewSigner(clientID, clientSecret, keyID),
	}
}

// ExchangeResult is the response from Platform Core's token exchange.
type ExchangeResult struct {
	UserID               string    `json:"user_id"`
	DisplayName          string    `json:"display_name,omitempty"`
	SessionExchangeToken string    `json:"session_exchange_token"`
	ExpiresAt            time.Time `json:"expires_at"`
}

// ExchangeCode exchanges an authorization code for a session exchange token.
func (c *Client) ExchangeCode(ctx context.Context, code, verifier, idempotencyKey string) (ExchangeResult, error) {
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  c.redirectURI,
		"client_id":     c.clientID,
		"code_verifier": verifier,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/oauth/token", bytes.NewReader(body))
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	if err := c.signer.Sign(req); err != nil {
		return ExchangeResult{}, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return ExchangeResult{}, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return ExchangeResult{}, fmt.Errorf("exchange: status %d", resp.StatusCode)
	}

	var envelope struct {
		Data struct {
			User struct {
				UserID      string `json:"user_id"`
				DisplayName string `json:"display_name"`
			} `json:"user"`
			SessionExchangeToken string    `json:"session_exchange_token"`
			ExpiresAt            time.Time `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&envelope); err != nil {
		return ExchangeResult{}, fmt.Errorf("decode: %w", err)
	}
	if envelope.Data.User.UserID == "" || len(envelope.Data.SessionExchangeToken) < 32 || envelope.Data.ExpiresAt.IsZero() {
		return ExchangeResult{}, fmt.Errorf("invalid exchange response")
	}

	return ExchangeResult{
		UserID:               envelope.Data.User.UserID,
		DisplayName:          strings.TrimSpace(envelope.Data.User.DisplayName),
		SessionExchangeToken: envelope.Data.SessionExchangeToken,
		ExpiresAt:            envelope.Data.ExpiresAt,
	}, nil
}

// CheckPermission verifies a permission against Platform Core. Platform Core
// rejects checks without a scope (validScope fails on an empty kind, which the
// Gateway would surface as a 503), so the scope is derived from the permission
// code: the product named by the code's first segment (e.g. "portal.notice.read"
// checks the "portal" product scope), and platform scope for "platform.*" codes.
func (c *Client) CheckPermission(ctx context.Context, exchangeToken, permissionCode string) error {
	scope := map[string]string{"kind": "platform"}
	if product, _, ok := strings.Cut(permissionCode, "."); ok && product != "platform" {
		scope = map[string]string{"kind": "product", "product_code": product}
	}
	body, _ := json.Marshal(map[string]any{
		"session_exchange_token": exchangeToken,
		"permission_code":        permissionCode,
		"scope":                  scope,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/authorization/check", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if err := c.signer.Sign(req); err != nil {
		return fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	default:
		return fmt.Errorf("authorization/check: status %d", resp.StatusCode)
	}
}

// Sentinel errors.
var (
	ErrUnauthorized = fmt.Errorf("unauthorized")
	ErrForbidden    = fmt.Errorf("forbidden")
)
