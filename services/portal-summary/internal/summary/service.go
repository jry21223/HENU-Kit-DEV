package summary

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"henukit.dev/portal-summary/internal/contract"
)

const (
	DefaultProbeTimeout = 500 * time.Millisecond
	DefaultTotalTimeout = 1500 * time.Millisecond
	maxFeedbackAge      = 5 * time.Minute
	maxProbes           = 8
)

type Probe struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Config struct {
	Version, CommitSHA  string
	DeployedAt          time.Time
	ReadinessURL        string
	KeyProbes           []Probe
	EntryProbes         []Probe
	FeedbackURL         string
	FeedbackCredentials Credentials
	ProbeTimeout        time.Duration
	TotalTimeout        time.Duration
}

type Credentials struct{ ClientID, ClientSecret, KeyID string }

type Service struct {
	config Config
	client *http.Client
	now    func() time.Time
}

type Feedback struct {
	PendingCount int       `json:"pending_count"`
	RecentCount  int       `json:"recent_count"`
	AsOf         time.Time `json:"as_of"`
}

type probeResult struct {
	name    string
	healthy bool
	detail  string
}

func New(config Config, client *http.Client) (*Service, error) {
	if strings.TrimSpace(config.Version) == "" || len([]rune(config.Version)) > 80 || !validCommit(config.CommitSHA) || config.DeployedAt.IsZero() || client == nil {
		return nil, errors.New("portal deployment metadata and HTTP client are required")
	}
	if err := validEndpoint(config.ReadinessURL, true); err != nil {
		return nil, fmt.Errorf("invalid readiness URL: %w", err)
	}
	if len(config.KeyProbes) > maxProbes || len(config.EntryProbes) > maxProbes {
		return nil, fmt.Errorf("probe groups are limited to %d entries", maxProbes)
	}
	if len(config.KeyProbes) == 0 || len(config.EntryProbes) == 0 {
		return nil, errors.New("at least one key-page probe and one product-entry probe are required")
	}
	for _, probe := range append(append([]Probe{}, config.KeyProbes...), config.EntryProbes...) {
		if strings.TrimSpace(probe.Name) == "" || len([]rune(probe.Name)) > 40 {
			return nil, errors.New("probe name must contain at most 40 characters")
		}
		if err := validEndpoint(probe.URL, true); err != nil {
			return nil, fmt.Errorf("invalid probe %q: %w", probe.Name, err)
		}
	}
	if config.FeedbackURL != "" {
		if err := validEndpoint(config.FeedbackURL, true); err != nil {
			return nil, fmt.Errorf("invalid feedback URL: %w", err)
		}
		if config.FeedbackCredentials.ClientID == "" || len(config.FeedbackCredentials.ClientSecret) < 16 || config.FeedbackCredentials.KeyID == "" {
			return nil, errors.New("feedback summary credentials are required when its URL is configured")
		}
	}
	if config.ProbeTimeout <= 0 || config.ProbeTimeout > time.Second {
		config.ProbeTimeout = DefaultProbeTimeout
	}
	if config.TotalTimeout <= 0 || config.TotalTimeout > 2*time.Second {
		config.TotalTimeout = DefaultTotalTimeout
	}
	probeClient := *client
	probeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Service{config: config, client: &probeClient, now: time.Now}, nil
}

func (s *Service) Build(ctx context.Context) contract.PortalSummary {
	ctx, cancel := context.WithTimeout(ctx, s.config.TotalTimeout)
	defer cancel()
	type groupResult struct {
		kind    string
		results []probeResult
	}
	groups := make(chan groupResult, 3)
	go func() {
		groups <- groupResult{kind: "readiness", results: s.probeAll(ctx, []Probe{{Name: "readiness", URL: s.config.ReadinessURL}})}
	}()
	go func() { groups <- groupResult{kind: "key", results: s.probeAll(ctx, s.config.KeyProbes)} }()
	go func() { groups <- groupResult{kind: "entry", results: s.probeAll(ctx, s.config.EntryProbes)} }()
	collected := map[string][]probeResult{}
	for range 3 {
		item := <-groups
		collected[item.kind] = item.results
	}
	feedback, feedbackState := s.feedback(ctx)
	readinessHealthy, _ := countHealthy(collected["readiness"])
	keyHealthy, keyTotal := countHealthy(collected["key"])
	entryHealthy, entryTotal := countHealthy(collected["entry"])
	failures := (1 - readinessHealthy) + (keyTotal - keyHealthy) + (entryTotal - entryHealthy)
	status := "ok"
	if failures > 0 || feedbackState != "ok" {
		status = "partial"
	}
	feedbackValue, feedbackHint := "未接入", "未配置 Portal 反馈摘要数据源"
	if feedbackState == "ok" {
		feedbackValue = fmt.Sprintf("%d 待处理", feedback.PendingCount)
		feedbackHint = fmt.Sprintf("近期开启 %d · 截至 %s", feedback.RecentCount, feedback.AsOf.UTC().Format(time.RFC3339))
	} else if feedbackState == "unavailable" {
		feedbackValue, feedbackHint = "暂不可用", "已配置的数据源未返回有效摘要"
	}
	message := "Portal 部署与只读探测正常"
	if status == "partial" {
		message = fmt.Sprintf("Portal 摘要部分可用：%d 个当前探测异常，反馈状态为%s", failures, feedbackValue)
	}
	return contract.PortalSummary{ID: "portal", Status: status, AsOf: s.now().UTC(), StatusMessage: message, Metrics: []contract.Metric{
		{Label: "部署版本", Value: s.config.Version},
		{Label: "Commit", Value: shortCommit(s.config.CommitSHA), Hint: s.config.CommitSHA},
		{Label: "部署时间", Value: s.config.DeployedAt.UTC().Format("01-02 15:04"), Hint: s.config.DeployedAt.UTC().Format(time.RFC3339)},
		{Label: "Readiness", Value: healthLabel(readinessHealthy, 1)},
		{Label: "关键探测", Value: fmt.Sprintf("%d/%d", keyHealthy, keyTotal), Hint: failureHint(collected["key"])},
		{Label: "入口健康", Value: fmt.Sprintf("%d/%d", entryHealthy, entryTotal), Hint: failureHint(collected["entry"])},
		{Label: "反馈摘要", Value: feedbackValue, Hint: feedbackHint},
		{Label: "当前异常", Value: fmt.Sprintf("%d", failures), Hint: "仅表示本次有界探测，不冒充历史事故记录"},
	}}
}

func (s *Service) probeAll(ctx context.Context, probes []Probe) []probeResult {
	results := make([]probeResult, len(probes))
	var wait sync.WaitGroup
	for index, probe := range probes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			probeCtx, cancel := context.WithTimeout(ctx, s.config.ProbeTimeout)
			defer cancel()
			request, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, probe.URL, nil)
			request.Header.Set("Accept", "text/html,application/json;q=0.9")
			response, err := s.client.Do(request)
			if err != nil {
				results[index] = probeResult{name: probe.Name, detail: "request_failed"}
				return
			}
			defer response.Body.Close()
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
			healthy := response.StatusCode >= 200 && response.StatusCode < 300
			results[index] = probeResult{name: probe.Name, healthy: healthy, detail: fmt.Sprintf("http_%d", response.StatusCode)}
		}()
	}
	wait.Wait()
	return results
}

func (s *Service) feedback(ctx context.Context) (Feedback, string) {
	if s.config.FeedbackURL == "" {
		return Feedback{}, "not_configured"
	}
	probeCtx, cancel := context.WithTimeout(ctx, s.config.ProbeTimeout)
	defer cancel()
	request, _ := http.NewRequestWithContext(probeCtx, http.MethodGet, s.config.FeedbackURL, nil)
	request.Header.Set("Accept", "application/json")
	if err := sign(request, s.config.FeedbackCredentials, s.now()); err != nil {
		return Feedback{}, "unavailable"
	}
	response, err := s.client.Do(request)
	if err != nil {
		return Feedback{}, "unavailable"
	}
	defer response.Body.Close()
	var value Feedback
	decoder := json.NewDecoder(io.LimitReader(response.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if response.StatusCode != http.StatusOK || decoder.Decode(&value) != nil {
		return Feedback{}, "unavailable"
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Feedback{}, "unavailable"
	}
	feedbackAge := s.now().Sub(value.AsOf)
	if value.PendingCount < 0 || value.RecentCount < 0 || value.AsOf.IsZero() || feedbackAge < -time.Minute || feedbackAge > maxFeedbackAge {
		return Feedback{}, "unavailable"
	}
	return value, "ok"
}

func sign(request *http.Request, credentials Credentials, now time.Time) error {
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	nonceValue := base64.RawURLEncoding.EncodeToString(nonce)
	timestamp := fmt.Sprintf("%d", now.Unix())
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, nonceValue, hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(credentials.ClientSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(credentials.ClientID, credentials.ClientSecret)
	request.Header.Set("X-Service-Id", credentials.ClientID)
	request.Header.Set("X-Key-Id", credentials.KeyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonceValue)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Request-Id", "req_portal_feedback_"+base64.RawURLEncoding.EncodeToString(nonce[:9]))
	return nil
}

func countHealthy(results []probeResult) (int, int) {
	healthy := 0
	for _, result := range results {
		if result.healthy {
			healthy++
		}
	}
	return healthy, len(results)
}

func failureHint(results []probeResult) string {
	failed := []string{}
	for _, result := range results {
		if !result.healthy {
			failed = append(failed, result.name+"("+result.detail+")")
		}
	}
	if len(failed) == 0 {
		return "全部配置探测成功"
	}
	return truncate("失败："+strings.Join(failed, "、"), 120)
}

func healthLabel(healthy, total int) string {
	if healthy == total {
		return "ready"
	}
	return "not ready"
}
func shortCommit(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
func truncate(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum-1]) + "…"
}
func validCommit(value string) bool {
	if len(value) < 7 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

func validEndpoint(raw string, required bool) error {
	if raw == "" && !required {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("absolute URL is required")
	}
	loopback := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
	if parsed.Scheme != "https" && !loopback {
		return errors.New("HTTPS is required outside loopback")
	}
	return nil
}
