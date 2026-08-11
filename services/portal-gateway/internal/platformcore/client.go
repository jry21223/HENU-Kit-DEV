package platformcore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

// Client communicates with Platform Core for auth and authorization.
type Client struct {
	baseURL                 string
	redirectURI             string
	clientID                string
	httpClient              *http.Client
	authorizationHTTPClient *http.Client
	signer                  *serviceauth.Signer
}

// NewClient creates a Platform Core client.
func NewClient(baseURL, redirectURI, clientID, clientSecret, keyID string) *Client {
	return &Client{
		baseURL:                 baseURL,
		redirectURI:             redirectURI,
		clientID:                clientID,
		httpClient:              &http.Client{Timeout: 10 * time.Second},
		authorizationHTTPClient: directAuthorizationClient(),
		signer:                  serviceauth.NewSigner(clientID, clientSecret, keyID),
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

// CheckPermission verifies a permission against Platform Core.
func (c *Client) CheckPermission(ctx context.Context, exchangeToken, permissionCode string) error {
	body, _ := json.Marshal(map[string]string{
		"session_exchange_token": exchangeToken,
		"permission_code":        permissionCode,
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

// ProductPermissionDecision is the verified Core subject for a product-scoped
// Portal read. Gateway compares ActorUserID with its decoded Portal Session
// before it sends that actor to a downstream Owner.
type ProductPermissionDecision struct {
	ActorUserID string
}

// CheckProductPermission uses Platform Core's required structured scope
// rather than the historical incomplete CheckPermission request. It is kept
// separate so unrelated callers retain their existing contracts.
func (c *Client) CheckProductPermission(ctx context.Context, exchangeToken, permissionCode, productCode string) (ProductPermissionDecision, error) {
	return c.checkProductPermission(ctx, exchangeToken, permissionCode, productCode, "")
}

// CheckProductPermissionWithRequestID preserves the Gateway's inbound request
// ID across the live authorization check. The existing method remains for
// callers whose established contract does not provide a request ID.
func (c *Client) CheckProductPermissionWithRequestID(ctx context.Context, exchangeToken, permissionCode, productCode, requestID string) (ProductPermissionDecision, error) {
	if !authorizationRequestIDPattern.MatchString(requestID) {
		return ProductPermissionDecision{}, fmt.Errorf("invalid authorization/check request id")
	}
	return c.checkProductPermission(ctx, exchangeToken, permissionCode, productCode, requestID)
}

func (c *Client) checkProductPermission(ctx context.Context, exchangeToken, permissionCode, productCode, requestID string) (ProductPermissionDecision, error) {
	body, err := json.Marshal(struct {
		SessionExchangeToken string `json:"session_exchange_token"`
		PermissionCode       string `json:"permission_code"`
		Scope                struct {
			Kind        string `json:"kind"`
			ProductCode string `json:"product_code"`
		} `json:"scope"`
	}{
		SessionExchangeToken: exchangeToken,
		PermissionCode:       permissionCode,
		Scope: struct {
			Kind        string `json:"kind"`
			ProductCode string `json:"product_code"`
		}{Kind: "product", ProductCode: productCode},
	})
	if err != nil {
		return ProductPermissionDecision{}, fmt.Errorf("marshal authorization/check request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/authorization/check", bytes.NewReader(body))
	if err != nil {
		return ProductPermissionDecision{}, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
	}
	if err := c.signer.Sign(req); err != nil {
		return ProductPermissionDecision{}, fmt.Errorf("sign: %w", err)
	}
	if c.authorizationHTTPClient == nil {
		return ProductPermissionDecision{}, fmt.Errorf("authorization client is unavailable")
	}
	resp, err := c.authorizationHTTPClient.Do(req)
	if err != nil {
		return ProductPermissionDecision{}, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ProductPermissionDecision{}, ErrUnauthorized
	case http.StatusForbidden:
		return ProductPermissionDecision{}, ErrForbidden
	default:
		return ProductPermissionDecision{}, fmt.Errorf("authorization/check: status %d", resp.StatusCode)
	}
	var envelope struct {
		RequestID string `json:"request_id"`
		Data      struct {
			Allowed        bool   `json:"allowed"`
			ActorUserID    string `json:"actor_user_id"`
			PermissionCode string `json:"permission_code"`
			Scope          struct {
				Kind        string `json:"kind"`
				ProductCode string `json:"product_code"`
			} `json:"scope"`
			GrantID               string    `json:"grant_id"`
			AuthorizationRevision int64     `json:"authorization_revision"`
			CheckedAt             time.Time `json:"checked_at"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return ProductPermissionDecision{}, fmt.Errorf("decode authorization/check response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return ProductPermissionDecision{}, fmt.Errorf("invalid authorization/check response")
	}
	if strings.TrimSpace(envelope.RequestID) == "" || !envelope.Data.Allowed || envelope.Data.PermissionCode != permissionCode || envelope.Data.Scope.Kind != "product" || envelope.Data.Scope.ProductCode != productCode || envelope.Data.AuthorizationRevision < 1 || envelope.Data.CheckedAt.IsZero() {
		return ProductPermissionDecision{}, fmt.Errorf("invalid authorization/check decision")
	}
	if !authorizationUUIDPattern.MatchString(envelope.Data.ActorUserID) {
		return ProductPermissionDecision{}, fmt.Errorf("invalid authorization/check actor")
	}
	if !authorizationUUIDPattern.MatchString(envelope.Data.GrantID) {
		return ProductPermissionDecision{}, fmt.Errorf("invalid authorization/check grant")
	}
	return ProductPermissionDecision{ActorUserID: envelope.Data.ActorUserID}, nil
}

// Sentinel errors.
var (
	ErrUnauthorized = fmt.Errorf("unauthorized")
	ErrForbidden    = fmt.Errorf("forbidden")
)

var authorizationUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var authorizationRequestIDPattern = regexp.MustCompile(`^req_[A-Za-z0-9_-]{1,116}$`)

// directAuthorizationClient keeps the fresh session exchange token and HMAC
// credential on the configured Core origin. Existing generic Core callers
// retain their established client behavior; only this new actor-bearing
// authorization step gets the stricter transport.
func directAuthorizationClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
