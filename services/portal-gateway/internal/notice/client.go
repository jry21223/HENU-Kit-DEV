package notice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
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

const (
	maxNoticeItems         = 50
	maxNoticeResponseBytes = 2 << 20
)

var requiredNoticeItemFields = []string{
	"id",
	"source",
	"version",
	"title",
	"body",
	"source_url",
	"content_hash",
	"state",
	"revision",
	"created_at",
	"distribution_count",
}

var requiredNoticeSourceFields = []string{"id", "code", "name"}

var (
	noticeUUIDPattern        = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	noticeContentHashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	noticeRequestIDPattern   = regexp.MustCompile(`^req_[A-Za-z0-9_-]+$`)
	validNoticeStates        = map[string]bool{"pending_review": true, "approved": true, "rejected": true, "distributed": true}
	validDistributionStatus  = map[string]bool{"queued": true, "processing": true, "delivered": true, "failed": true}
)

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
func (c *Client) List(ctx context.Context, actorUserID, requestID string) (contract.NoticeFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+SnapshotPath, nil)
	if err != nil {
		return contract.NoticeFeed{}, fmt.Errorf("NewRequest: %w", err)
	}
	req.Header.Set(actorUserIDHeader, actorUserID)
	req.Header.Set(requestIDHeader, requestID)
	req.Header.Set(permissionCodeHeader, readPermissionCode)
	req.Header.Set(scopeKindHeader, scopeKindProduct)
	req.Header.Set(productCodeHeader, platformcore.ScopeOf(readPermissionCode))
	if err := c.signer.Sign(req); err != nil {
		return contract.NoticeFeed{}, fmt.Errorf("sign: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return contract.NoticeFeed{}, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	// Every failed owner read surfaces as a 503 for the Portal user, so the
	// owner's 401/403 rejections fold into the same wrapped error with the
	// status code retained for operator logs.
	if resp.StatusCode != http.StatusOK {
		return contract.NoticeFeed{}, fmt.Errorf("notice: status %d", resp.StatusCode)
	}

	var envelope struct {
		Data      json.RawMessage `json:"data"`
		RequestID string          `json:"request_id"`
	}
	rawResponse, err := io.ReadAll(io.LimitReader(resp.Body, maxNoticeResponseBytes+1))
	if err != nil || len(rawResponse) > maxNoticeResponseBytes || decodeStrict(rawResponse, &envelope) != nil || len(envelope.Data) == 0 || !noticeRequestIDPattern.MatchString(envelope.RequestID) || len(envelope.RequestID) > 120 {
		return contract.NoticeFeed{}, errors.New("invalid Notice response")
	}
	var snapshot struct {
		Items       []json.RawMessage `json:"items"`
		GeneratedAt json.RawMessage   `json:"generated_at"`
	}
	if err := decodeStrict(envelope.Data, &snapshot); err != nil || snapshot.Items == nil || len(snapshot.Items) > maxNoticeItems || len(snapshot.GeneratedAt) == 0 || string(snapshot.GeneratedAt) == "null" {
		return contract.NoticeFeed{}, errors.New("invalid Notice snapshot")
	}
	var generatedAt time.Time
	if err := json.Unmarshal(snapshot.GeneratedAt, &generatedAt); err != nil {
		return contract.NoticeFeed{}, errors.New("invalid Notice snapshot generated_at")
	}

	distributed := make([]contract.NoticeFeedItem, 0, len(snapshot.Items))
	for index, item := range snapshot.Items {
		validated, err := validateNoticeItem(item)
		if err != nil {
			slog.Warn("portal-gateway notice snapshot rejected: invalid item", "request_id", requestID, "item_index", index, "error", err)
			return contract.NoticeFeed{}, errors.New("invalid Notice snapshot item")
		}
		if validated.State == distributedState {
			distributed = append(distributed, validated)
		}
	}
	return contract.NoticeFeed{
		Items:       distributed,
		GeneratedAt: generatedAt,
	}, nil
}

func validateNoticeItem(raw json.RawMessage) (contract.NoticeFeedItem, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return contract.NoticeFeedItem{}, errors.New("notice item must be an object")
	}
	for _, name := range requiredNoticeItemFields {
		value, ok := fields[name]
		if !ok || string(value) == "null" {
			return contract.NoticeFeedItem{}, fmt.Errorf("notice item.%s is required", name)
		}
	}

	var source map[string]json.RawMessage
	if err := json.Unmarshal(fields["source"], &source); err != nil || source == nil {
		return contract.NoticeFeedItem{}, errors.New("notice item.source must be an object")
	}
	for _, name := range requiredNoticeSourceFields {
		value, ok := source[name]
		if !ok || string(value) == "null" {
			return contract.NoticeFeedItem{}, fmt.Errorf("notice item.source.%s is required", name)
		}
	}

	var item contract.NoticeFeedItem
	if err := decodeStrict(raw, &item); err != nil {
		return contract.NoticeFeedItem{}, fmt.Errorf("decode notice item: %w", err)
	}
	if !noticeUUIDPattern.MatchString(item.ID) || !noticeUUIDPattern.MatchString(item.Source.ID) {
		return contract.NoticeFeedItem{}, errors.New("notice item and source ids must be UUIDs")
	}
	if item.Version < 1 || item.Revision < 1 || item.DistributionCount < 0 {
		return contract.NoticeFeedItem{}, errors.New("notice item numeric fields are outside the contract")
	}
	if !noticeContentHashPattern.MatchString(item.ContentHash) {
		return contract.NoticeFeedItem{}, errors.New("notice item content_hash is invalid")
	}
	sourceURL, err := url.ParseRequestURI(item.SourceURL)
	if err != nil || !sourceURL.IsAbs() {
		return contract.NoticeFeedItem{}, errors.New("notice item source_url is invalid")
	}
	if !validNoticeStates[item.State] {
		return contract.NoticeFeedItem{}, errors.New("notice item state is invalid")
	}
	if item.DistributionStatus != nil && !validDistributionStatus[*item.DistributionStatus] {
		return contract.NoticeFeedItem{}, errors.New("notice item distribution_status is invalid")
	}
	return item, nil
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
