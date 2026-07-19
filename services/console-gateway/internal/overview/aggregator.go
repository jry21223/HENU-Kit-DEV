package overview

import (
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
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

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/contract"
)

const (
	DefaultModuleTimeout   = 2 * time.Second
	DefaultOverviewTimeout = 3 * time.Second
	DefaultCacheTTL        = 5 * time.Minute
)

var moduleIDs = []string{"portal", "platform", "notice", "library", "quizcraft", "food"}

type Aggregator struct {
	endpoints       map[string]string
	httpClient      *http.Client
	redis           *redis.Client
	moduleTimeout   time.Duration
	overviewTimeout time.Duration
	cacheTTL        time.Duration
	retryDelay      func(int) time.Duration
	now             func() time.Time
	credentials     map[string]Credentials
}

type Credentials struct{ ClientID, ClientSecret, KeyID string }

type Options struct {
	ModuleTimeout, OverviewTimeout, CacheTTL time.Duration
	RetryDelay                               func(int) time.Duration
	Now                                      func() time.Time
}

type cacheEntry struct {
	Summary       contract.ConsoleModuleSummary `json:"summary"`
	LastSuccessAt time.Time                     `json:"last_success_at"`
}

func New(endpoints map[string]string, client *http.Client, redisClient *redis.Client, credentials map[string]Credentials, options Options) (*Aggregator, error) {
	if client == nil || redisClient == nil {
		return nil, errors.New("overview dependencies are required")
	}
	validated := make(map[string]string, len(moduleIDs))
	usedSecrets := map[string]string{}
	for _, id := range moduleIDs {
		endpoint := strings.TrimSpace(endpoints[id])
		if endpoint != "" {
			parsed, err := url.Parse(endpoint)
			loopback := err == nil && parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || net.ParseIP(parsed.Hostname()).IsLoopback())
			if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !loopback) || parsed.User != nil || parsed.Fragment != "" {
				return nil, fmt.Errorf("invalid %s summary endpoint", id)
			}
			credential := credentials[id]
			if credential.ClientID == "" || len(credential.ClientSecret) < 16 || credential.KeyID == "" {
				return nil, fmt.Errorf("%s summary credentials are required", id)
			}
			if owner, duplicate := usedSecrets[credential.ClientSecret]; duplicate {
				return nil, fmt.Errorf("%s and %s summary credentials must use distinct secrets", owner, id)
			}
			usedSecrets[credential.ClientSecret] = id
		}
		validated[id] = endpoint
	}
	if options.ModuleTimeout <= 0 {
		options.ModuleTimeout = DefaultModuleTimeout
	}
	if options.OverviewTimeout <= 0 {
		options.OverviewTimeout = DefaultOverviewTimeout
	}
	if options.CacheTTL <= 0 || options.CacheTTL > DefaultCacheTTL {
		options.CacheTTL = DefaultCacheTTL
	}
	if options.RetryDelay == nil {
		options.RetryDelay = func(attempt int) time.Duration { return jitteredRetryDelay(cryptorand.Reader, attempt) }
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Aggregator{endpoints: validated, httpClient: client, redis: redisClient, moduleTimeout: options.ModuleTimeout, overviewTimeout: options.OverviewTimeout, cacheTTL: options.CacheTTL, retryDelay: options.RetryDelay, now: options.Now, credentials: credentials}, nil
}

func (a *Aggregator) Fetch(ctx context.Context, requestID string) contract.ConsoleOverview {
	ctx, cancel := context.WithTimeout(ctx, a.overviewTimeout)
	defer cancel()
	type result struct {
		index   int
		summary contract.ConsoleModuleSummary
	}
	results := make(chan result, len(moduleIDs))
	var wait sync.WaitGroup
	for index, id := range moduleIDs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- result{index: index, summary: a.fetchModule(ctx, id, requestID+"_"+id)}
		}()
	}
	go func() { wait.Wait(); close(results) }()
	modules := make([]contract.ConsoleModuleSummary, len(moduleIDs))
	for item := range results {
		modules[item.index] = item.summary
	}
	return contract.ConsoleOverview{Modules: modules, GeneratedAt: a.now().UTC()}
}

func (a *Aggregator) fetchModule(parent context.Context, id, requestID string) contract.ConsoleModuleSummary {
	ctx, cancel := context.WithTimeout(parent, a.moduleTimeout)
	defer cancel()
	endpoint := a.endpoints[id]
	if endpoint != "" {
		for attempt := 0; attempt < 2; attempt++ {
			summary, retry, err := a.read(ctx, id, endpoint, requestID)
			if err == nil {
				a.store(ctx, id, summary)
				return summary
			}
			if !retry || attempt == 1 || !waitFor(ctx, a.retryDelay(attempt)) {
				break
			}
		}
	}
	if cached, ok := a.cached(parent, id); ok {
		cached.Summary.Status = "stale"
		cached.Summary.LastSuccessAt = &cached.LastSuccessAt
		cached.Summary.RequestID = requestID
		cached.Summary.StatusMessage = "当前摘要不可用，展示五分钟内最近一次成功结果"
		return cached.Summary
	}
	return contract.ConsoleModuleSummary{ID: id, Status: "unavailable", Metrics: []contract.ConsoleModuleMetric{}, StatusMessage: "摘要暂不可用", RequestID: requestID}
}

func (a *Aggregator) read(ctx context.Context, id, endpoint, requestID string) (contract.ConsoleModuleSummary, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return contract.ConsoleModuleSummary{}, false, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Request-Id", requestID)
	if err := a.sign(request, a.credentials[id]); err != nil {
		return contract.ConsoleModuleSummary{}, false, err
	}
	response, err := a.httpClient.Do(request)
	if err != nil {
		return contract.ConsoleModuleSummary{}, !errors.Is(err, context.Canceled), err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32<<10))
		return contract.ConsoleModuleSummary{}, response.StatusCode >= 500, fmt.Errorf("module returned %d", response.StatusCode)
	}
	var envelope struct {
		Data      contract.ConsoleModuleSummary `json:"data"`
		RequestID string                        `json:"request_id"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || envelope.Data.ID != id || !liveStatus(envelope.Data.Status) || !validSummary(envelope.Data) || envelope.RequestID != requestID {
		return contract.ConsoleModuleSummary{}, false, errors.New("module summary violates contract")
	}
	envelope.Data.RequestID = envelope.RequestID
	return envelope.Data, false, nil
}

func (a *Aggregator) store(ctx context.Context, id string, summary contract.ConsoleModuleSummary) {
	encoded, err := json.Marshal(cacheEntry{Summary: summary, LastSuccessAt: a.now().UTC()})
	if err == nil {
		_ = a.redis.Set(ctx, "console:overview:"+id, encoded, a.cacheTTL).Err()
	}
}

func (a *Aggregator) cached(ctx context.Context, id string) (cacheEntry, bool) {
	ttl, err := a.redis.TTL(ctx, "console:overview:"+id).Result()
	if err != nil || ttl <= 0 || ttl > a.cacheTTL {
		return cacheEntry{}, false
	}
	encoded, err := a.redis.Get(ctx, "console:overview:"+id).Bytes()
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if json.Unmarshal(encoded, &entry) != nil {
		return cacheEntry{}, false
	}
	age := a.now().Sub(entry.LastSuccessAt)
	if entry.Summary.ID != id || !liveStatus(entry.Summary.Status) || !validSummary(entry.Summary) || entry.LastSuccessAt.IsZero() || age < 0 || age > a.cacheTTL {
		return cacheEntry{}, false
	}
	return entry, true
}

func (a *Aggregator) sign(request *http.Request, credentials Credentials) error {
	nonce := make([]byte, 24)
	if _, err := cryptorand.Read(nonce); err != nil {
		return err
	}
	timestamp := fmt.Sprintf("%d", a.now().Unix())
	digest := sha256.Sum256(nil)
	canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), timestamp, base64.RawURLEncoding.EncodeToString(nonce), hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, []byte(credentials.ClientSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth(credentials.ClientID, credentials.ClientSecret)
	request.Header.Set("X-Service-Id", credentials.ClientID)
	request.Header.Set("X-Key-Id", credentials.KeyID)
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", base64.RawURLEncoding.EncodeToString(nonce))
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	return nil
}

func liveStatus(status string) bool {
	return status == "ok" || status == "empty" || status == "partial"
}

func validSummary(summary contract.ConsoleModuleSummary) bool {
	if summary.AsOf == nil || summary.AsOf.IsZero() || len(summary.Metrics) > 8 || summary.StatusMessage == "" || len([]rune(summary.StatusMessage)) > 240 {
		return false
	}
	for _, metric := range summary.Metrics {
		if metric.Label == "" || len([]rune(metric.Label)) > 40 || metric.Value == "" || len([]rune(metric.Value)) > 80 || (metric.Hint != nil && len([]rune(*metric.Hint)) > 120) {
			return false
		}
	}
	return true
}

func waitFor(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func jitteredRetryDelay(source io.Reader, attempt int) time.Duration {
	var random [1]byte
	if _, err := io.ReadFull(source, random[:]); err != nil {
		random[0] = byte(time.Now().UnixNano())
	}
	return time.Duration(25+int(random[0])%51+attempt*17) * time.Millisecond
}
