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

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/platformcore"
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

// readPermissionCode is the Notice owner's permission code for the signed
// snapshot read. Its first segment names the owner's product scope, which the
// Gateway derives via platformcore.ScopeOf.
const readPermissionCode = "notice.read"

// Header names the Notice owner requires on the signed actor-bound read.
const (
	actorUserIDHeader    = "X-Actor-User-Id"
	requestIDHeader      = "X-Request-Id"
	permissionCodeHeader = "X-Permission-Code"
	scopeKindHeader      = "X-Scope-Kind"
	productCodeHeader    = "X-Product-Code"
)

// scopeKindProduct is the scope kind of the Notice owner's product scope:
// the read is always scoped to the notice product named by
// readPermissionCode.
const scopeKindProduct = "product"

// Client proxies read-only requests to the Notice service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
}

// NewClient validates the Notice service configuration and returns an error
// immediately when any credential is missing or malformed, so a
// misconfigured client can never make signed requests with empty
// credentials.
func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || strings.TrimSpace(clientID) == "" || len(clientSecret) < 32 || strings.TrimSpace(keyID) == "" {
		return nil, errors.New("invalid Notice client configuration: baseURL must be an absolute http(s) URL without credentials and clientID, clientSecret, and keyID must be set")
	}
	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}, nil
}

// List fetches the Notice owner's bounded snapshot for the Portal Session
// actor and returns the filtered snapshot data ({"items": [...],
// "generated_at"}). Only distributed (published) items are returned;
// pending and approved notices are internal working state and never leave
// the Gateway. A genuine empty items array is successful; absent or null
// items are invalid.
func (c *Client) List(ctx context.Context, actorUserID, requestID string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+SnapshotPath, nil)
	if err != nil {
		return nil, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set(actorUserIDHeader, actorUserID)
	req.Header.Set(requestIDHeader, requestID)
	req.Header.Set(permissionCodeHeader, readPermissionCode)
	req.Header.Set(scopeKindHeader, scopeKindProduct)
	req.Header.Set(productCodeHeader, platformcore.ScopeOf(readPermissionCode))
	if err := c.signer.Sign(req); err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	// Every failed owner read surfaces as a 503 for the Portal user, so the
	// owner's 401/403 rejections fold into the same wrapped error with the
	// status code retained for operator logs.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("notice: status %d", resp.StatusCode)
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	if err := decoder.Decode(&envelope); err != nil || len(envelope.Data) == 0 {
		return nil, errors.New("invalid Notice response")
	}
	var snapshot contract.NoticeFeed
	if err := json.Unmarshal(envelope.Data, &snapshot); err != nil || snapshot.Items == nil {
		return nil, errors.New("invalid Notice snapshot")
	}

	distributed := make([]json.RawMessage, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		var lifecycle contract.NoticeItemLifecycle
		// Items whose lifecycle cannot be parsed are skipped: only notices
		// known to be distributed may leave the Gateway.
		if err := json.Unmarshal(item, &lifecycle); err == nil && lifecycle.State == distributedState {
			distributed = append(distributed, item)
		}
	}
	filtered, err := json.Marshal(contract.NoticeFeed{
		Items:       distributed,
		GeneratedAt: snapshot.GeneratedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("encode Notice snapshot: %w", err)
	}
	return filtered, nil
}
