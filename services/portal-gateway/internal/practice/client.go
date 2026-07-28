package practice

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

//go:generate go run ../../cmd/quizcraftcontractgen -contract ../../../../packages/api-contracts/openapi/quizcraft.yaml -output contract_generated.go

const (
	CatalogReadPermission = "portal.practice.read"
	// AnonymousCatalogActor makes guest catalog reads explicit while preserving
	// Portal Gateway's product-request actor-header invariant.
	AnonymousCatalogActor = "anonymous"
)

var (
	ErrCatalogUnauthorized = errors.New("QuizCraft catalog rejected service authentication")
	ErrCatalogForbidden    = errors.New("QuizCraft catalog denied the requested permission")
	ErrCatalogUnavailable  = errors.New("QuizCraft catalog is unavailable")
	ErrInvalidCatalog      = errors.New("QuizCraft returned an invalid catalog response")
)

// Client is an internal, read-only client for the QuizCraft catalog. It is not
// registered on Portal Gateway's public router until the #166 cutover window.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
}

// NewClient creates an internal-only Catalog client with explicit service
// credentials. Browser clients never receive this configuration.
func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || clientID == "" || len(clientSecret) < 32 || keyID == "" {
		return nil, errors.New("invalid QuizCraft catalog client configuration")
	}
	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}, nil
}

// Banks reads the Core's published catalog. A genuine empty data array is a
// successful empty catalog; absent or null data is an invalid response, never
// a mock fallback.
func (c *Client) Banks(ctx context.Context, actorUserID, requestID string) (BankListEnvelope, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || strings.TrimSpace(requestID) == "" {
		return BankListEnvelope{}, ErrCatalogUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+ListPracticeBanksPath, nil)
	if err != nil {
		return BankListEnvelope{}, ErrCatalogUnavailable
	}
	actorUserID = strings.TrimSpace(actorUserID)
	if actorUserID == "" {
		actorUserID = AnonymousCatalogActor
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Permission-Code", CatalogReadPermission)
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "quizcraft")
	if err := c.signer.Sign(req); err != nil {
		return BankListEnvelope{}, fmt.Errorf("catalog sign: %w", ErrCatalogUnavailable)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return BankListEnvelope{}, fmt.Errorf("catalog request: %w", ErrCatalogUnavailable)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return BankListEnvelope{}, ErrCatalogUnauthorized
	case http.StatusForbidden:
		return BankListEnvelope{}, ErrCatalogForbidden
	default:
		return BankListEnvelope{}, fmt.Errorf("catalog status %d: %w", resp.StatusCode, ErrCatalogUnavailable)
	}

	var raw struct {
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&raw); err != nil {
		return BankListEnvelope{}, fmt.Errorf("catalog decode: %w", ErrInvalidCatalog)
	}
	if strings.TrimSpace(raw.RequestID) == "" || len(raw.Data) == 0 || string(raw.Data) == "null" {
		return BankListEnvelope{}, ErrInvalidCatalog
	}
	var result BankListEnvelope
	if err := json.Unmarshal(raw.Data, &result.Data); err != nil || result.Data == nil {
		return BankListEnvelope{}, ErrInvalidCatalog
	}
	result.RequestID = raw.RequestID
	if err := validateCatalog(result); err != nil {
		return BankListEnvelope{}, err
	}
	return result, nil
}

func validateCatalog(result BankListEnvelope) error {
	for _, bank := range result.Data {
		if bank.BankID == "" || bank.BankVersionID == "" || bank.BankKey == "" || bank.Name == "" || len(bank.ContentSHA256) != 64 || bank.QuestionCount < 0 || bank.Chapters == nil {
			return ErrInvalidCatalog
		}
		for _, chapter := range bank.Chapters {
			if chapter.ID == "" || chapter.Name == "" {
				return ErrInvalidCatalog
			}
		}
	}
	return nil
}
