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

	"golang.org/x/text/unicode/norm"

	"henukit.dev/portal-gateway/internal/serviceauth"
)

//go:generate go run ../../cmd/quizcraftcontractgen -contract ../../../../packages/api-contracts/openapi/quizcraft.yaml -output contract_generated.go

const (
	PortalReadPermission  = "portal.practice.read"
	CatalogReadPermission = PortalReadPermission
	// AnonymousCatalogActor makes guest catalog reads explicit while preserving
	// Portal Gateway's product-request actor-header invariant.
	AnonymousCatalogActor = "anonymous"
)

var (
	ErrPortalReadUnauthorized = errors.New("QuizCraft Portal read rejected service authentication")
	ErrPortalReadForbidden    = errors.New("QuizCraft Portal read denied the requested permission")
	ErrPortalReadUnavailable  = errors.New("QuizCraft Portal read is unavailable")
	ErrInvalidPortalRead      = errors.New("QuizCraft returned an invalid Portal read response")

	ErrCatalogUnauthorized = ErrPortalReadUnauthorized
	ErrCatalogForbidden    = ErrPortalReadForbidden
	ErrCatalogUnavailable  = ErrPortalReadUnavailable
	ErrInvalidCatalog      = ErrInvalidPortalRead

	ErrRankingUnauthorized = ErrPortalReadUnauthorized
	ErrRankingForbidden    = ErrPortalReadForbidden
	ErrRankingUnavailable  = ErrPortalReadUnavailable
	ErrInvalidRanking      = ErrInvalidPortalRead

	ErrStatsUnauthorized = ErrPortalReadUnauthorized
	ErrStatsForbidden    = ErrPortalReadForbidden
	ErrStatsUnavailable  = ErrPortalReadUnavailable
	ErrInvalidStats      = ErrInvalidPortalRead
)

// Client is an internal, read-only client for QuizCraft catalog and ranking
// facts. It is not registered on Portal Gateway's public router until #166.
type Client struct {
	baseURL    string
	httpClient *http.Client
	signer     *serviceauth.Signer
}

// NewClient creates an internal-only Portal read client with explicit service
// credentials. Browser clients never receive this configuration.
func NewClient(baseURL, clientID, clientSecret, keyID string) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || clientID == "" || len(clientSecret) < 32 || keyID == "" {
		return nil, errors.New("invalid QuizCraft Portal read client configuration")
	}
	return &Client{
		baseURL:    strings.TrimRight(parsed.String(), "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		signer:     serviceauth.NewSigner(clientID, clientSecret, keyID),
	}, nil
}

// Banks reads the Core's public published catalog. A catalog has no
// user-specific result, and the legacy read signature does not bind an actor
// header, so its API deliberately accepts no actor argument. This makes every
// catalog read explicitly anonymous instead of depending on each caller to
// avoid attaching an unauthenticated browser identity.
//
// A genuine empty data array is successful; absent or null data is invalid,
// never a mock fallback.
func (c *Client) Banks(ctx context.Context, requestID string) (BankListEnvelope, error) {
	resp, err := c.portalRead(ctx, AnonymousCatalogActor, requestID, ListPracticeBanksPath)
	if err != nil {
		return BankListEnvelope{}, err
	}
	defer resp.Body.Close()

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

// OverallRanking returns a true empty page when Core has no scored attempts;
// it never turns that state into browser-owned example rankings.
func (c *Client) OverallRanking(ctx context.Context, actorUserID, requestID string, period RankingPeriod) (RankingEnvelope, error) {
	normalized, err := normalizeRankingPeriod(period)
	if err != nil {
		return RankingEnvelope{}, err
	}
	values := url.Values{"period": []string{string(normalized)}}
	return c.ranking(ctx, actorUserID, requestID, OverallRankingPath+"?"+values.Encode(), "overall", "", normalized)
}

// BankRanking returns one bank's public ranking with the same service-auth
// boundary as the overall view.
func (c *Client) BankRanking(ctx context.Context, actorUserID, requestID, bankID string, period RankingPeriod) (RankingEnvelope, error) {
	if strings.TrimSpace(bankID) == "" {
		return RankingEnvelope{}, ErrInvalidRanking
	}
	normalized, err := normalizeRankingPeriod(period)
	if err != nil {
		return RankingEnvelope{}, err
	}
	path := strings.Replace(BankRankingPath, "{bank_id}", url.PathEscape(bankID), 1)
	values := url.Values{"period": []string{string(normalized)}}
	return c.ranking(ctx, actorUserID, requestID, path+"?"+values.Encode(), "bank", bankID, normalized)
}

func (c *Client) ranking(ctx context.Context, actorUserID, requestID, path, scope, bankID string, period RankingPeriod) (RankingEnvelope, error) {
	resp, err := c.portalRead(ctx, actorUserID, requestID, path)
	if err != nil {
		return RankingEnvelope{}, err
	}
	defer resp.Body.Close()
	var result RankingEnvelope
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 2<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return RankingEnvelope{}, fmt.Errorf("ranking decode: %w", ErrInvalidRanking)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RankingEnvelope{}, fmt.Errorf("ranking decode: %w", ErrInvalidRanking)
	}
	if err := validateRanking(result, scope, bankID, period); err != nil {
		return RankingEnvelope{}, err
	}
	return result, nil
}

func (c *Client) portalRead(ctx context.Context, actorUserID, requestID, path string) (*http.Response, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || strings.TrimSpace(requestID) == "" {
		return nil, ErrPortalReadUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, ErrPortalReadUnavailable
	}
	actorUserID = strings.TrimSpace(actorUserID)
	if actorUserID == "" {
		actorUserID = AnonymousCatalogActor
	}
	req.Header.Set("X-Actor-User-Id", actorUserID)
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Permission-Code", PortalReadPermission)
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "quizcraft")
	if err := c.signer.Sign(req); err != nil {
		return nil, fmt.Errorf("Portal read sign: %w", ErrPortalReadUnavailable)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Portal read request: %w", ErrPortalReadUnavailable)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized:
		_ = resp.Body.Close()
		return nil, ErrPortalReadUnauthorized
	case http.StatusForbidden:
		_ = resp.Body.Close()
		return nil, ErrPortalReadForbidden
	default:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("Portal read status %d: %w", resp.StatusCode, ErrPortalReadUnavailable)
	}
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
	actorUserID = strings.TrimSpace(actorUserID)
	if !validUUID(actorUserID) {
		return PersonalPracticeStatsEnvelope{}, ErrStatsUnauthorized
	}
	resp, err := c.actorBoundPersonalStats(ctx, actorUserID, requestID)
	if err != nil {
		return PersonalPracticeStatsEnvelope{}, err
	}
	defer resp.Body.Close()

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

// actorBoundPersonalStats is intentionally separate from portalRead. Catalog
// and ranking are public/read-shared facts; personal statistics are one
// account's data, so the Core rejects a request unless the actor UUID is the
// sixth HMAC canonical line.
func (c *Client) actorBoundPersonalStats(ctx context.Context, actorUserID, requestID string) (*http.Response, error) {
	if c == nil || c.signer == nil || c.httpClient == nil || !validUUID(actorUserID) || strings.TrimSpace(requestID) == "" {
		return nil, ErrStatsUnauthorized
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+GetPersonalPracticeStatsPath, nil)
	if err != nil {
		return nil, ErrStatsUnavailable
	}
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Permission-Code", PortalReadPermission)
	req.Header.Set("X-Scope-Kind", "product")
	req.Header.Set("X-Product-Code", "quizcraft")
	if err := c.signer.SignWithActor(req, actorUserID); err != nil {
		return nil, fmt.Errorf("Portal personal stats sign: %w", ErrStatsUnavailable)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Portal personal stats request: %w", ErrStatsUnavailable)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp, nil
	case http.StatusUnauthorized:
		_ = resp.Body.Close()
		return nil, ErrStatsUnauthorized
	case http.StatusForbidden:
		_ = resp.Body.Close()
		return nil, ErrStatsForbidden
	default:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("Portal personal stats status %d: %w", resp.StatusCode, ErrStatsUnavailable)
	}
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

func normalizeRankingPeriod(period RankingPeriod) (RankingPeriod, error) {
	if period == "" {
		return RankingPeriodWeekly, nil
	}
	if period != RankingPeriodWeekly && period != RankingPeriodLifetime {
		return "", ErrInvalidRanking
	}
	return period, nil
}

func validateRanking(result RankingEnvelope, expectedScope, expectedBankID string, expectedPeriod RankingPeriod) error {
	if strings.TrimSpace(result.RequestID) == "" ||
		result.Data.Scope != expectedScope ||
		result.Data.Period != expectedPeriod ||
		result.Data.Metric != "correct_answer_count" ||
		result.Data.Entries == nil {
		return ErrInvalidRanking
	}
	if expectedScope == "bank" && result.Data.BankID != expectedBankID {
		return ErrInvalidRanking
	}
	if expectedScope == "overall" && result.Data.BankID != "" {
		return ErrInvalidRanking
	}
	previousRank := int64(0)
	previousCount := int64(-1)
	for _, entry := range result.Data.Entries {
		if entry.Rank < 1 || entry.Rank < previousRank || strings.TrimSpace(entry.Nickname) == "" || looksLikeRankingIdentifier(entry.Nickname) || !validRankingAvatar(entry.SystemAvatar) || entry.CorrectAnswerCount < 0 || previousCount >= 0 && entry.CorrectAnswerCount > previousCount {
			return ErrInvalidRanking
		}
		previousRank = entry.Rank
		previousCount = entry.CorrectAnswerCount
	}
	return nil
}

func looksLikeRankingIdentifier(value string) bool {
	value = strings.TrimSpace(norm.NFKC.String(value))
	if strings.Contains(value, "@") {
		return true
	}
	compact := strings.Map(func(r rune) rune {
		if strings.ContainsRune(" _-.", r) {
			return -1
		}
		return r
	}, value)
	if len(compact) != 32 {
		return false
	}
	for _, r := range compact {
		if !('0' <= r && r <= '9') && !('a' <= r && r <= 'f') && !('A' <= r && r <= 'F') {
			return false
		}
	}
	return true
}

func validRankingAvatar(value string) bool {
	switch value {
	case "scholar-blue", "coder-green", "reader-amber", "owl-purple":
		return true
	default:
		return false
	}
}
