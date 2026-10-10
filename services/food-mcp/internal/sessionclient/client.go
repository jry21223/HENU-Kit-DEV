// Package sessionclient resolves the existing Portal Session through its
// owning Gateway. Credentials remain transport metadata, never MCP arguments.
package sessionclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const secureCookieName = "__Host-henukit_portal_session"

var (
	ErrNoSession   = errors.New("no usable Kit session")
	ErrUnavailable = errors.New("kit session lookup is unavailable")
)

type Account struct {
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Client struct {
	endpoint, cookieName string
	httpClient           *http.Client
}

// NewClient permits an empty URL for installations without Kit integration.
// The configured endpoint is trusted operator configuration; redirects are
// disabled so the selected session cookie cannot follow a different endpoint.
func NewClient(endpoint, cookieName string) (*Client, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "/api/v1/session" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("invalid Kit session endpoint")
	}
	if cookieName == "" {
		cookieName = secureCookieName
	}
	if err := (&http.Cookie{Name: cookieName, Value: "validation"}).Valid(); err != nil {
		return nil, errors.New("invalid Kit session cookie name")
	}
	return &Client{
		endpoint: parsed.String(), cookieName: cookieName,
		httpClient: &http.Client{
			Timeout:       3 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// Resolve re-reads the cookie on every call, rather than retaining an account
// from MCP initialization. Only the exact Kit cookie goes to the Gateway;
// unrelated cookies and the MCP Bearer credential never leave this service.
func (c *Client) Resolve(ctx context.Context, headers http.Header) (Account, error) {
	if c == nil {
		return Account{}, ErrNoSession
	}
	cookie, err := (&http.Request{Header: headers}).Cookie(c.cookieName)
	if err != nil || cookie.Value == "" {
		return Account{}, ErrNoSession
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return Account{}, ErrUnavailable
	}
	request.AddCookie(cookie)
	if c.cookieName == secureCookieName {
		// Compose's Gateway trusts its private network. HTTPS installations
		// select the same secure cookie profile even over the internal hop.
		request.Header.Set("X-Forwarded-Proto", "https")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return Account{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return Account{}, ErrNoSession
	}
	if response.StatusCode != http.StatusOK {
		return Account{}, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<10)+1))
	if err != nil || len(raw) > 4<<10 {
		return Account{}, ErrUnavailable
	}
	var account Account
	if err := json.Unmarshal(raw, &account); err != nil {
		return Account{}, ErrUnavailable
	}
	id, err := uuid.Parse(account.UserID)
	account.DisplayName = strings.TrimSpace(account.DisplayName)
	if err != nil || id == uuid.Nil || !account.ExpiresAt.After(time.Now()) || len([]rune(account.DisplayName)) > 120 || strings.ContainsAny(account.DisplayName, "\r\n") {
		return Account{}, ErrNoSession
	}
	account.UserID = id.String()
	if account.DisplayName == "" {
		account.DisplayName = "Kit 用户"
	}
	return account, nil
}
