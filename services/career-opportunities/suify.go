package career

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed skills/resume-suify.md
var resumeSuifySkill string

// SuifyFunc produces one transient entertainment rewrite of Resume Text.
// The caller decides whether to discard or apply the returned draft.
type SuifyFunc func(ctx context.Context, original string) (string, error)

var (
	ErrSuifyFailed         = errors.New("career resume suification failed")
	errSuifyProviderFailed = fmt.Errorf("%w: suification provider failed", ErrSuifyFailed)
)

// SuifyConfig builds the Career LLM-backed resume suifier.
type SuifyConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewMockSuifier keeps the local mock mode usable without calling a provider.
func NewMockSuifier() SuifyFunc {
	return func(_ context.Context, original string) (string, error) {
		return strings.TrimSpace(original), nil
	}
}

// NewOpenAICompatibleSuifier sends the embedded suification skill and the
// exact current Resume Text to an OpenAI-compatible chat-completions endpoint.
func NewOpenAICompatibleSuifier(cfg SuifyConfig) (SuifyFunc, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, ErrAIUnconfigured
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("%w: provider URL is invalid", ErrAIUnconfigured)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	endpoint := baseURL + "/chat/completions"
	apiKey := strings.TrimSpace(cfg.APIKey)
	model := strings.TrimSpace(cfg.Model)

	return func(ctx context.Context, original string) (string, error) {
		requestBody, err := json.Marshal(map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": resumeSuifySkill},
				{"role": "user", "content": original},
			},
			"temperature": 0.8,
		})
		if err != nil {
			return "", fmt.Errorf("%w: encode provider request: %w", ErrSuifyFailed, err)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
		if err != nil {
			return "", fmt.Errorf("%w: create provider request: %w", ErrSuifyFailed, err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+apiKey)
		response, err := clientCopy.Do(request)
		if err != nil {
			return "", fmt.Errorf("%w: provider call failed", errSuifyProviderFailed)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			providerCode := readProviderErrorCode(response.Body)
			if providerCode != "" {
				return "", fmt.Errorf("%w: provider returned %d (%s)", errSuifyProviderFailed, response.StatusCode, providerCode)
			}
			return "", fmt.Errorf("%w: provider returned %d", errSuifyProviderFailed, response.StatusCode)
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
		if err != nil {
			return "", fmt.Errorf("%w: provider response unreadable", errSuifyProviderFailed)
		}
		var envelope struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Choices) == 0 {
			return "", fmt.Errorf("%w: provider response is invalid", errSuifyProviderFailed)
		}
		draft := strings.TrimSpace(envelope.Choices[0].Message.Content)
		if draft == "" {
			return "", fmt.Errorf("%w: provider response is empty", ErrSuifyFailed)
		}
		return truncateSuificationText(draft, profileResumeTextLimit), nil
	}, nil
}

func truncateSuificationText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
