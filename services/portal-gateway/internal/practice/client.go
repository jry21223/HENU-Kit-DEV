package practice

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/serviceauth"
)

// Client proxies read-only requests to QuizCraft's practice endpoints.
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

// Banks fetches available question banks.
func (c *Client) Banks(ctx context.Context, actorUserID, requestID string) (contract.PracticeBanksResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/banks", nil)
	if err != nil {
		return contract.PracticeBanksResponse{}, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	req.Header.Set("X-Request-Id", requestID)
	if err := c.signer.Sign(req); err != nil {
		return contract.PracticeBanksResponse{}, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return contract.PracticeBanksResponse{}, fmt.Errorf("Do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return contract.PracticeBanksResponse{}, fmt.Errorf("unauthorized")
	}
	if resp.StatusCode != http.StatusOK {
		return contract.PracticeBanksResponse{}, fmt.Errorf("practice: status %d", resp.StatusCode)
	}

	var result contract.PracticeBanksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return contract.PracticeBanksResponse{}, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}
