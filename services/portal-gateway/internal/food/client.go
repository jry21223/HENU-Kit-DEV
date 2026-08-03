package food

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/serviceauth"
)

// Client proxies read-only requests to the Food service.
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

// Venues fetches food venues for a campus.
func (c *Client) Venues(ctx context.Context, campus, actorUserID, requestID string) (contract.FoodVenuesResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/venues?campus="+campus, nil)
	if err != nil {
		return contract.FoodVenuesResponse{}, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	req.Header.Set("X-Request-Id", requestID)
	if err := c.signer.Sign(req); err != nil {
		return contract.FoodVenuesResponse{}, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return contract.FoodVenuesResponse{}, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return contract.FoodVenuesResponse{}, fmt.Errorf("unauthorized")
	}
	if resp.StatusCode != http.StatusOK {
		return contract.FoodVenuesResponse{}, fmt.Errorf("food: status %d", resp.StatusCode)
	}

	var result contract.FoodVenuesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return contract.FoodVenuesResponse{}, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}
