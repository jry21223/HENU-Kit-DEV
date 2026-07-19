package overview

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/contract"
)

func testCredentials() map[string]Credentials {
	result := make(map[string]Credentials, len(moduleIDs))
	for _, id := range moduleIDs {
		result[id] = Credentials{ClientID: "console-gateway-" + id, ClientSecret: "test-" + id + "-summary-secret-with-entropy", KeyID: id + "-active-key"}
	}
	return result
}

func TestAggregatorReturnsPartialResultsRetriesOnceAndUsesStaleCache(t *testing.T) {
	redisClient := integrationRedis(t)
	var retryCalls, failedCalls atomic.Int32
	var libraryFails atomic.Bool
	endpoints := map[string]string{}
	servers := []*httptest.Server{}
	add := func(id string, handler http.HandlerFunc) {
		server := httptest.NewServer(handler)
		servers = append(servers, server)
		endpoints[id] = server.URL
	}
	add("portal", successfulSummary("portal"))
	add("platform", func(writer http.ResponseWriter, request *http.Request) {
		if retryCalls.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		successfulSummary("platform").ServeHTTP(writer, request)
	})
	add("notice", func(writer http.ResponseWriter, _ *http.Request) {
		failedCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	})
	add("library", func(writer http.ResponseWriter, request *http.Request) {
		if libraryFails.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		successfulSummary("library").ServeHTTP(writer, request)
	})
	add("quizcraft", func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-time.After(120 * time.Millisecond):
			successfulSummary("quizcraft").ServeHTTP(writer, request)
		case <-request.Context().Done():
		}
	})
	add("food", successfulSummary("food"))
	for _, server := range servers {
		defer server.Close()
	}

	aggregator, err := New(endpoints, &http.Client{}, redisClient, testCredentials(), Options{ModuleTimeout: 50 * time.Millisecond, OverviewTimeout: 90 * time.Millisecond, CacheTTL: DefaultCacheTTL, RetryDelay: func(int) time.Duration { return time.Millisecond }})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	first := aggregator.Fetch(t.Context(), "req_overview_first")
	if elapsed := time.Since(started); elapsed > 110*time.Millisecond {
		t.Fatalf("overview exceeded deadline: %v", elapsed)
	}
	if len(first.Modules) != 6 || first.Modules[1].Status != "ok" || retryCalls.Load() != 2 || first.Modules[2].Status != "unavailable" || failedCalls.Load() != 2 || first.Modules[4].Status != "unavailable" {
		t.Fatalf("unexpected partial result: %+v retry=%d failed=%d", first.Modules, retryCalls.Load(), failedCalls.Load())
	}
	ttl, err := redisClient.TTL(t.Context(), "console:overview:library").Result()
	if err != nil || ttl <= 0 || ttl > DefaultCacheTTL {
		t.Fatalf("cache TTL = %v, err=%v", ttl, err)
	}

	libraryFails.Store(true)
	second := aggregator.Fetch(t.Context(), "req_overview_second")
	library := second.Modules[3]
	if library.Status != "stale" || library.AsOf == nil || library.LastSuccessAt == nil || library.RequestID != "req_overview_second_library" {
		t.Fatalf("stale library summary = %+v", library)
	}
	now := time.Now().UTC()
	oversized := strings.Repeat("x", 241)
	corrupt, _ := json.Marshal(cacheEntry{Summary: contract.ConsoleModuleSummary{ID: "library", Status: "ok", Metrics: []contract.ConsoleModuleMetric{}, StatusMessage: oversized, AsOf: &now, RequestID: "req_corrupt"}, LastSuccessAt: now})
	if err := redisClient.Set(t.Context(), "console:overview:library", corrupt, DefaultCacheTTL).Err(); err != nil {
		t.Fatal(err)
	}
	if _, ok := aggregator.cached(t.Context(), "library"); ok {
		t.Fatal("corrupted cache bypassed summary bounds")
	}
	old, _ := json.Marshal(cacheEntry{Summary: contract.ConsoleModuleSummary{ID: "library", Status: "ok", Metrics: []contract.ConsoleModuleMetric{}, StatusMessage: "摘要可用", AsOf: &now, RequestID: "req_old"}, LastSuccessAt: now.Add(-DefaultCacheTTL - time.Second)})
	_ = redisClient.Set(t.Context(), "console:overview:library", old, DefaultCacheTTL).Err()
	if _, ok := aggregator.cached(t.Context(), "library"); ok {
		t.Fatal("expired cache entry was accepted")
	}
}

func TestAggregatorOverallDeadlineCancelsEveryModule(t *testing.T) {
	redisClient := integrationRedis(t)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
	defer server.Close()
	endpoints := map[string]string{}
	for _, id := range moduleIDs {
		endpoints[id] = server.URL
	}
	aggregator, _ := New(endpoints, &http.Client{}, redisClient, testCredentials(), Options{ModuleTimeout: 500 * time.Millisecond, OverviewTimeout: 60 * time.Millisecond, RetryDelay: func(int) time.Duration { return 0 }})
	started := time.Now()
	result := aggregator.Fetch(t.Context(), "req_overall_deadline")
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("overall deadline exceeded: %v", elapsed)
	}
	for _, summary := range result.Modules {
		if summary.Status != "unavailable" {
			t.Fatalf("deadline summary = %+v", summary)
		}
	}
}

func TestDefaultOverviewLimitsMatchContract(t *testing.T) {
	if DefaultModuleTimeout != 2*time.Second || DefaultOverviewTimeout != 3*time.Second || DefaultCacheTTL != 5*time.Minute {
		t.Fatalf("unexpected defaults: %v %v %v", DefaultModuleTimeout, DefaultOverviewTimeout, DefaultCacheTTL)
	}
	first := jitteredRetryDelay(bytes.NewReader([]byte{1}), 0)
	second := jitteredRetryDelay(bytes.NewReader([]byte{49}), 0)
	if first == second || first < 25*time.Millisecond || second > 75*time.Millisecond {
		t.Fatalf("retry jitter is not bounded and variable: %v %v", first, second)
	}
}

func TestAggregatorRequiresServiceCredentials(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = client.Close() })
	if _, err := New(map[string]string{"portal": "https://portal.internal/summary"}, &http.Client{}, client, nil, Options{}); err == nil {
		t.Fatal("aggregator accepted missing service credentials")
	}
	credentials := testCredentials()
	credentials["platform"] = credentials["portal"]
	if _, err := New(map[string]string{"portal": "https://portal.internal/summary", "platform": "https://platform.internal/summary"}, &http.Client{}, client, credentials, Options{}); err == nil {
		t.Fatal("aggregator accepted a summary secret shared across modules")
	}
}

func TestRetryUsesDistinctNonces(t *testing.T) {
	redisClient := integrationRedis(t)
	var calls atomic.Int32
	nonces := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		nonces <- request.Header.Get("X-Nonce")
		if calls.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		successfulSummary("portal").ServeHTTP(writer, request)
	}))
	defer server.Close()
	aggregator, err := New(map[string]string{"portal": server.URL}, &http.Client{}, redisClient, testCredentials(), Options{RetryDelay: func(int) time.Duration { return 0 }})
	if err != nil {
		t.Fatal(err)
	}
	_ = aggregator.Fetch(t.Context(), "req_nonce")
	first, second := <-nonces, <-nonces
	if first == "" || second == "" || first == second {
		t.Fatalf("retry nonces must be non-empty and distinct: %q %q", first, second)
	}
}

func successfulSummary(id string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		credentials := testCredentials()[id]
		clientID, secret, basic := request.BasicAuth()
		digest := sha256.Sum256(nil)
		canonical := strings.Join([]string{request.Method, request.URL.RequestURI(), request.Header.Get("X-Timestamp"), request.Header.Get("X-Nonce"), hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, []byte(credentials.ClientSecret))
		_, _ = mac.Write([]byte(canonical))
		if !basic || clientID != credentials.ClientID || secret != credentials.ClientSecret || request.Header.Get("X-Service-Id") != clientID || request.Header.Get("X-Key-Id") != credentials.KeyID || !hmac.Equal([]byte(request.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))) {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		now := time.Now().UTC()
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": contract.ConsoleModuleSummary{
			ID: id, Status: "ok", Metrics: []contract.ConsoleModuleMetric{{Label: "状态", Value: "正常"}}, StatusMessage: "摘要可用", AsOf: &now,
		}, "request_id": request.Header.Get("X-Request-Id")})
	}
}

func integrationRedis(t *testing.T) *redis.Client {
	t.Helper()
	address := os.Getenv("CONSOLE_GATEWAY_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("CONSOLE_GATEWAY_TEST_REDIS_ADDR is required")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
