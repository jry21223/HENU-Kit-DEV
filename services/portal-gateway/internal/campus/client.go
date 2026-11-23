package campus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

const maxBodyBytes = 512 << 10

type Client struct {
	baseURL    string
	signer     *serviceauth.Signer
	httpClient *http.Client
}

type UpstreamResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.TrimSpace(clientID) == "" || len(clientSecret) < 16 || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("campus portal client credentials are incomplete")
	}
	return &Client{
		baseURL:    baseURL,
		signer:     serviceauth.NewSigner(strings.TrimSpace(clientID), clientSecret, strings.TrimSpace(keyID)),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *Client) Do(ctx context.Context, method, path, actorUserID, displayName, requestID string, body []byte) (UpstreamResponse, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return UpstreamResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.signer.Sign(req); err != nil {
		return UpstreamResponse{}, fmt.Errorf("sign campus request: %w", err)
	}
	req.Header.Set("X-Actor-User-Id", strings.TrimSpace(actorUserID))
	if strings.TrimSpace(displayName) != "" {
		req.Header.Set("X-Actor-Display-Name", strings.TrimSpace(displayName))
	}
	if strings.TrimSpace(requestID) != "" {
		req.Header.Set("X-Request-Id", strings.TrimSpace(requestID))
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return UpstreamResponse{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return UpstreamResponse{}, err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	return UpstreamResponse{StatusCode: resp.StatusCode, ContentType: ct, Body: respBody}, nil
}
