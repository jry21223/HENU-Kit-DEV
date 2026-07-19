package platformcore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
)

var (
	ErrUnauthorized = errors.New("platform core rejected session")
	ErrForbidden    = errors.New("platform core denied access")
	ErrConflict     = errors.New("platform core conflict")
	ErrNotFound     = errors.New("platform core resource not found")
	ErrInvalid      = errors.New("platform core rejected request")
)

type Client struct {
	baseURL, clientID, clientSecret, keyID string
	httpClient                             *http.Client
}

func (c *Client) PlatformOperations(ctx context.Context, exchangeToken string) (json.RawMessage, error) {
	return c.operationRequest(ctx, http.MethodGet, "/api/v1/platform-operations", exchangeToken, "", nil)
}

func (c *Client) RevokeSession(ctx context.Context, exchangeToken, sessionID, idempotencyKey string, body []byte) (json.RawMessage, error) {
	if _, err := uuid.Parse(sessionID); err != nil {
		return nil, ErrInvalid
	}
	return c.operationRequest(ctx, http.MethodPost, "/api/v1/platform-operations/sessions/"+sessionID+"/revocations", exchangeToken, idempotencyKey, body)
}

func (c *Client) UpdateAccess(ctx context.Context, exchangeToken, userID, idempotencyKey string, body []byte) (json.RawMessage, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrInvalid
	}
	return c.operationRequest(ctx, http.MethodPost, "/api/v1/platform-operations/users/"+userID+"/access-updates", exchangeToken, idempotencyKey, body)
}

func (c *Client) OperationStatus(ctx context.Context, exchangeToken, operation, idempotencyKey string) (json.RawMessage, error) {
	if operation != "session_revoke" && operation != "access_update" {
		return nil, ErrInvalid
	}
	return c.operationRequest(ctx, http.MethodGet, "/api/v1/platform-operations/operations/"+operation, exchangeToken, idempotencyKey, nil)
}

func (c *Client) operationRequest(ctx context.Context, method, path, exchangeToken, idempotencyKey string, body []byte) (json.RawMessage, error) {
	request, err := c.signedRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("X-Session-Exchange-Token", exchangeToken)
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if err := responseError(response); err != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		return nil, err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return nil, errors.New("invalid platform core operations response")
	}
	return envelope.Data, nil
}

type Exchange struct {
	UserID        string
	ExchangeToken string
	ExpiresAt     time.Time
}

func New(baseURL, clientID, clientSecret, keyID string, client *http.Client) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || clientID == "" || clientSecret == "" || keyID == "" {
		return nil, errors.New("invalid platform core client configuration")
	}
	loopback := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
	if parsed.Scheme != "https" && !loopback {
		return nil, errors.New("platform core URL must use HTTPS outside loopback development")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), clientID: clientID, clientSecret: clientSecret, keyID: keyID, httpClient: client}, nil
}

func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI, verifier, idempotencyKey string) (Exchange, error) {
	body, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "code": code, "redirect_uri": redirectURI, "client_id": c.clientID, "code_verifier": verifier})
	request, err := c.signedRequest(ctx, http.MethodPost, "/api/v1/oauth/token", body)
	if err != nil {
		return Exchange{}, err
	}
	request.Header.Set("Idempotency-Key", idempotencyKey)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return Exchange{}, err
	}
	defer response.Body.Close()
	if err := responseError(response); err != nil {
		return Exchange{}, err
	}
	var envelope struct {
		Data struct {
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			SessionExchangeToken string    `json:"session_exchange_token"`
			ExpiresAt            time.Time `json:"expires_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&envelope); err != nil || len(envelope.Data.SessionExchangeToken) < 32 || envelope.Data.ExpiresAt.IsZero() {
		return Exchange{}, errors.New("invalid platform core exchange response")
	}
	if _, userErr := uuid.Parse(envelope.Data.User.ID); userErr != nil {
		return Exchange{}, errors.New("invalid platform core exchange response")
	}
	return Exchange{UserID: envelope.Data.User.ID, ExchangeToken: envelope.Data.SessionExchangeToken, ExpiresAt: envelope.Data.ExpiresAt}, nil
}

func (c *Client) CheckOverview(ctx context.Context, exchangeToken string) error {
	return c.checkPermission(ctx, exchangeToken, "console.overview.read")
}

func (c *Client) CheckPlatformOperations(ctx context.Context, exchangeToken string) error {
	return c.checkPermission(ctx, exchangeToken, "platform.operations.read")
}

func (c *Client) CheckPlatformOperationsWrite(ctx context.Context, exchangeToken string) error {
	return c.checkPermission(ctx, exchangeToken, "platform.operations.write")
}

func (c *Client) checkPermission(ctx context.Context, exchangeToken, permissionCode string) error {
	body, _ := json.Marshal(map[string]any{"session_exchange_token": exchangeToken, "permission_code": permissionCode, "scope": map[string]string{"kind": "platform"}})
	request, err := c.signedRequest(ctx, http.MethodPost, "/api/v1/authorization/check", body)
	if err != nil {
		return err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		_ = response.Body.Close()
	}()
	return responseError(response)
}

func (c *Client) signedRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(c.clientID, c.clientSecret)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	digest := sha256.Sum256(body)
	canonical := strings.Join([]string{method, path, timestamp, nonce, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(c.clientSecret))
	_, _ = mac.Write([]byte(canonical))
	request.Header.Set("X-Service-Id", c.clientID)
	request.Header.Set("X-Key-Id", c.keyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	return request, nil
}

func responseError(response *http.Response) error {
	switch response.StatusCode {
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
		return fmt.Errorf("platform core returned %d", response.StatusCode)
	}
}
