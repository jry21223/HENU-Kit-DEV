package overview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/contract"
)

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

	aggregator, err := New(endpoints, &http.Client{}, redisClient, Options{ModuleTimeout: 50 * time.Millisecond, OverviewTimeout: 90 * time.Millisecond, CacheTTL: DefaultCacheTTL, RetryDelay: func(int) time.Duration { return time.Millisecond }})
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
}

func TestAggregatorOverallDeadlineCancelsEveryModule(t *testing.T) {
	redisClient := integrationRedis(t)
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
	defer server.Close()
	endpoints := map[string]string{}
	for _, id := range moduleIDs {
		endpoints[id] = server.URL
	}
	aggregator, _ := New(endpoints, &http.Client{}, redisClient, Options{ModuleTimeout: 500 * time.Millisecond, OverviewTimeout: 60 * time.Millisecond, RetryDelay: func(int) time.Duration { return 0 }})
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
}

func successfulSummary(id string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
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
