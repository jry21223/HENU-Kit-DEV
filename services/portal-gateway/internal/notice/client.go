package notice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/serviceauth"
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

// List fetches published notices.
func (c *Client) List(ctx context.Context, actorUserID, requestID string) (contract.NoticeListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/notices", nil)
	if err != nil {
		return contract.NoticeListResponse{}, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	req.Header.Set("X-Request-Id", requestID)
	if err := c.signer.Sign(req); err != nil {
		return contract.NoticeListResponse{}, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return contract.NoticeListResponse{}, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return contract.NoticeListResponse{}, fmt.Errorf("unauthorized")
	}
	if resp.StatusCode != http.StatusOK {
		return contract.NoticeListResponse{}, fmt.Errorf("notice: status %d", resp.StatusCode)
	}

	var result contract.NoticeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return contract.NoticeListResponse{}, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}
