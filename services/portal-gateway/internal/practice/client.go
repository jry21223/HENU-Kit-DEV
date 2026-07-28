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
	ErrStatsUnauthorized   = errors.New("QuizCraft statistics rejected service authentication")
	ErrStatsForbidden      = errors.New("QuizCraft statistics denied the requested permission")
	ErrStatsUnavailable    = errors.New("QuizCraft statistics are unavailable")
	ErrInvalidStats        = errors.New("QuizCraft returned invalid personal statistics")
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

// PersonalStats reads one signed-in Portal user's fact-derived Practice
// statistics. Empty totals and an empty mastery list are valid success states;
// no legacy or mock-shaped response is ever accepted as a fallback.
func (c *Client) PersonalStats(ctx context.Context, actorUserID, requestID string) (PersonalPracticeStatsEnvelope, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || strings.TrimSpace(requestID) == "" {
		return PersonalPracticeStatsEnvelope{}, ErrStatsUnavailable
	}
	actorUserID = strings.TrimSpace(actorUserID)
	if !validUUID(actorUserID) {
		return PersonalPracticeStatsEnvelope{}, ErrStatsUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+GetPersonalPracticeStatsPath, nil)
	if err != nil {
		return PersonalPracticeStatsEnvelope{}, ErrStatsUnavailable
	}
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Permission-Code", CatalogReadPermission)
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "quizcraft")
	if err := c.signer.SignWithActor(req, actorUserID); err != nil {
		return PersonalPracticeStatsEnvelope{}, fmt.Errorf("stats sign: %w", ErrStatsUnavailable)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PersonalPracticeStatsEnvelope{}, fmt.Errorf("stats request: %w", ErrStatsUnavailable)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return PersonalPracticeStatsEnvelope{}, ErrStatsUnauthorized
	case http.StatusForbidden:
		return PersonalPracticeStatsEnvelope{}, ErrStatsForbidden
	default:
		return PersonalPracticeStatsEnvelope{}, fmt.Errorf("stats status %d: %w", resp.StatusCode, ErrStatsUnavailable)
	}

	var raw struct {
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&raw); err != nil {
		return PersonalPracticeStatsEnvelope{}, fmt.Errorf("stats decode: %w", ErrInvalidStats)
	}
	if strings.TrimSpace(raw.RequestID) == "" || len(raw.Data) == 0 || string(raw.Data) == "null" {
		return PersonalPracticeStatsEnvelope{}, ErrInvalidStats
	}
	var result PersonalPracticeStatsEnvelope
	if err := json.Unmarshal(raw.Data, &result.Data); err != nil {
		return PersonalPracticeStatsEnvelope{}, ErrInvalidStats
	}
	result.RequestID = raw.RequestID
	if err := validatePersonalStats(result); err != nil {
		return PersonalPracticeStatsEnvelope{}, err
	}
	return result, nil
}

func validatePersonalStats(result PersonalPracticeStatsEnvelope) error {
	stats := result.Data
	if strings.TrimSpace(result.RequestID) == "" || stats.TotalAnswers < 0 || stats.CorrectAnswers < 0 || stats.CorrectAnswers > stats.TotalAnswers || stats.Accuracy < 0 || stats.Accuracy > 100 || stats.Accuracy != roundedPercent(stats.CorrectAnswers, stats.TotalAnswers) || stats.StreakDays < 0 || stats.Mastery == nil {
		return ErrInvalidStats
	}
	seenBanks := make(map[string]struct{}, len(stats.Mastery))
	for _, subject := range stats.Mastery {
		if !validUUID(subject.BankID) || strings.TrimSpace(subject.Label) == "" || subject.Value < 0 || subject.Value > 100 || subject.TotalQuestions < 0 || subject.CorrectQuestions < 0 || subject.CorrectQuestions > subject.TotalQuestions || subject.Value != roundedPercent(subject.CorrectQuestions, subject.TotalQuestions) {
			return ErrInvalidStats
		}
		if _, exists := seenBanks[subject.BankID]; exists {
			return ErrInvalidStats
		}
		seenBanks[subject.BankID] = struct{}{}
	}
	return nil
}

func roundedPercent(numerator, denominator int64) int {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	value := (numerator*100 + denominator/2) / denominator
	if value > 100 {
		return 100
	}
	return int(value)
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') && !(char >= 'A' && char <= 'F') {
			return false
		}
	}
	return true
}
