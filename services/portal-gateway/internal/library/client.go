package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/serviceauth"
)

// Client proxies read-only requests to the Library service.
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

// Courses fetches the list of available courses.
func (c *Client) Courses(ctx context.Context, actorUserID, requestID string) (contract.LibraryCoursesResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/courses", nil)
	if err != nil {
		return contract.LibraryCoursesResponse{}, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	req.Header.Set("X-Request-Id", requestID)
	if err := c.signer.Sign(req); err != nil {
		return contract.LibraryCoursesResponse{}, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return contract.LibraryCoursesResponse{}, fmt.Errorf("Do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return contract.LibraryCoursesResponse{}, fmt.Errorf("unauthorized")
	}
	if resp.StatusCode != http.StatusOK {
		return contract.LibraryCoursesResponse{}, fmt.Errorf("library: status %d", resp.StatusCode)
	}

	var result contract.LibraryCoursesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return contract.LibraryCoursesResponse{}, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}
