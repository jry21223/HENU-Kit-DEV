package notice

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

// SnapshotPath is the Notice service's read route. The Notice owner exposes a
// single bounded lifecycle snapshot; there is no separate public list or
// detail endpoint.
const SnapshotPath = "/api/v1/console-notices"

// distributedState is the lifecycle state that makes a notice a published
// announcement. Pending or approved items are internal working state and
// must never be exposed to Portal users.
const distributedState = "distributed"

// Sentinel errors map Notice owner rejections to honest Gateway responses.
var (
	ErrUnauthorized = errors.New("notice rejected service authentication")
	ErrForbidden    = errors.New("notice denied permission or scope")
)

// Client proxies read-only requests to the Notice service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
	configErr  error
}

// NewClient validates the Notice service configuration. A misconfigured
// client is still returned so existing call sites keep compiling, but every
// operation fails with the explicit configuration error instead of silently
// making signed requests with empty credentials.
func NewClient(baseURL, clientID, clientSecret, keyID string) *Client {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(clientID) == "" || len(clientSecret) < 32 || strings.TrimSpace(keyID) == "" {
		return &Client{configErr: errors.New("invalid Notice client configuration: baseURL must be an absolute http(s) URL without credentials and clientID, clientSecret, and keyID must be set")}
	}
	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}
}

// List fetches the Notice owner's bounded snapshot for the Portal Session
// actor and returns the filtered snapshot data ({"items": [...],
// "generated_at"}). Only distributed (published) items are returned;
// pending and approved notices are internal working state and never leave
// the Gateway. A genuine empty items array is successful; absent or null
// items are invalid.
func (c *Client) List(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	if c.configErr != nil {
		return nil, c.configErr
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+SnapshotPath, nil)
	if err != nil {
		return nil, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Permission-Code", "notice.read")
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "notice")
	if err := c.signer.Sign(req); err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusForbidden:
		return nil, ErrForbidden
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("notice: status %d", resp.StatusCode)
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return nil, errors.New("invalid Notice response")
	}
	var snapshot struct {
		Items       []json.RawMessage `json:"items"`
		GeneratedAt json.RawMessage   `json:"generated_at"`
	}
	if err := json.Unmarshal(envelope.Data, &snapshot); err != nil || snapshot.Items == nil {
		return nil, errors.New("invalid Notice snapshot")
	}

	distributed := make([]json.RawMessage, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		var lifecycle struct {
			State string `json:"state"`
		}
		// Items whose lifecycle cannot be parsed are skipped: only notices
		// known to be distributed may leave the Gateway.
		if err := json.Unmarshal(item, &lifecycle); err == nil && lifecycle.State == distributedState {
			distributed = append(distributed, item)
		}
	}
	filtered, err := json.Marshal(struct {
		Items       []json.RawMessage `json:"items"`
		GeneratedAt json.RawMessage   `json:"generated_at"`
	}{Items: distributed, GeneratedAt: snapshot.GeneratedAt})
	if err != nil {
		return nil, fmt.Errorf("encode Notice snapshot: %w", err)
	}
	return filtered, nil
}
