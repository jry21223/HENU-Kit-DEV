package notice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

// SnapshotPath is the Notice service's read route. The Notice owner exposes a
// single bounded lifecycle snapshot; there is no separate public list or
// detail endpoint.
const SnapshotPath = "/api/v1/console-notices"

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
}

func NewClient(baseURL, clientID, clientSecret, keyID string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}
}

// List fetches the Notice owner's bounded snapshot for the Portal Session
// actor and returns the raw snapshot data ({"items": [...], "generated_at"}).
// A genuine empty items array is successful; absent or null items are invalid.
func (c *Client) List(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
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
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(envelope.Data, &snapshot); err != nil || snapshot.Items == nil {
		return nil, errors.New("invalid Notice snapshot")
	}
	return envelope.Data, nil
}
