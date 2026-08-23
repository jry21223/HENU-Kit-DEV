package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	career "henukit.dev/career"
)

const suificationsPath = "/api/v1/career/profile/suifications"

func newCareerServerWithSuify(t *testing.T, suify career.SuifyFunc, rateLimit int) (*httptest.Server, *pgxpool.Pool, *redis.Client) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), os.Getenv("CAREER_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("CAREER_TEST_REDIS_ADDR")})
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `TRUNCATE career_digest_deliveries, career_search_operations, career_search_results, career_searches, career_profiles, career_resume_extractions`); err != nil {
		t.Fatal(err)
	}
	service, err := career.New(career.Config{
		Database:       pool,
		Redis:          redisClient,
		ClientID:       careerClientID,
		Keys:           map[string]string{"active": careerSecret},
		Suify:          suify,
		SuifyRateLimit: rateLimit,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisClient.Close() })
	return httptest.NewServer(service), pool, redisClient
}

func TestCreateSuificationReturnsTransientDraft(t *testing.T) {
	const original = "负责校园资料检索网站开发"
	const draft = "负责校园资料检索网站开发，聚焦校园资料检索场景"
	var received string
	calls := 0
	server, pool, _ := newCareerServerWithSuify(t, func(_ context.Context, value string) (string, error) {
		calls++
		received = value
		return draft, nil
	}, 5)
	defer server.Close()
	defer pool.Close()

	response := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"`+original+`"}`), "idem_suify_transient")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("create suification status = %d: %s", response.StatusCode, readBody(t, response))
	}
	data := decodeData(t, readBody(t, response))
	draftValue := data["draft"].(map[string]any)
	if draftValue["resume_text"] != draft {
		t.Fatalf("suification draft = %v, want %q", draftValue, draft)
	}
	if received != original {
		t.Fatalf("suifier received %q, want exact current Resume Text %q", received, original)
	}
	replay := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"`+original+`"}`), "idem_suify_transient")
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("suification replay status = %d: %s", replay.StatusCode, readBody(t, replay))
	}
	if calls != 1 {
		t.Fatalf("idempotent replay called the Career LLM %d times, want 1", calls)
	}
	var storedProfiles int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM career_profiles`).Scan(&storedProfiles); err != nil {
		t.Fatal(err)
	}
	if storedProfiles != 0 {
		t.Fatalf("transient suification stored %d profiles", storedProfiles)
	}
}

func TestCreateSuificationRejectsEmptyAndUnconfiguredRequests(t *testing.T) {
	server, pool, _ := newCareerServerWithSuify(t, nil, 5)
	defer server.Close()
	defer pool.Close()

	unconfigured := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"校内项目"}`), "idem_suify_unconfigured")
	if unconfigured.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured suification status = %d: %s", unconfigured.StatusCode, readBody(t, unconfigured))
	}

	configured, configuredPool, _ := newCareerServerWithSuify(t, func(_ context.Context, value string) (string, error) {
		return value, nil
	}, 5)
	defer configured.Close()
	defer configuredPool.Close()
	empty := send(t, configured.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"   "}`), "idem_suify_empty")
	if empty.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty suification status = %d: %s", empty.StatusCode, readBody(t, empty))
	}
}

func TestCreateSuificationRateLimitsPerActor(t *testing.T) {
	server, pool, _ := newCareerServerWithSuify(t, func(_ context.Context, value string) (string, error) {
		return value + " 已酥化", nil
	}, 1)
	defer server.Close()
	defer pool.Close()

	first := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"校内项目"}`), "idem_suify_rate_1")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first suification status = %d: %s", first.StatusCode, readBody(t, first))
	}
	second := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"校内项目"}`), "idem_suify_rate_2")
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second suification status = %d: %s", second.StatusCode, readBody(t, second))
	}
}

func TestCreateSuificationRejectsIdempotencyKeyReuseWithDifferentText(t *testing.T) {
	server, pool, _ := newCareerServerWithSuify(t, func(_ context.Context, value string) (string, error) {
		return value + " 已酥化", nil
	}, 5)
	defer server.Close()
	defer pool.Close()

	first := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"项目一"}`), "idem_suify_conflict")
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first suification status = %d: %s", first.StatusCode, readBody(t, first))
	}
	conflict := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"项目二"}`), "idem_suify_conflict")
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("conflicting suification status = %d: %s", conflict.StatusCode, readBody(t, conflict))
	}
}

func TestCreateSuificationAllowsOnlyOneActiveProviderCallPerKey(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server, pool, _ := newCareerServerWithSuify(t, func(_ context.Context, value string) (string, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return value, nil
	}, 5)
	defer server.Close()
	defer pool.Close()

	firstRequest := newSignedRequest(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"校内项目"}`), "application/json", "idem_suify_active")
	type requestResult struct {
		response *http.Response
		err      error
	}
	firstResponse := make(chan requestResult, 1)
	go func() {
		response, err := http.DefaultClient.Do(firstRequest)
		firstResponse <- requestResult{response: response, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first Suification did not reach the provider")
	}
	second := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"校内项目"}`), "idem_suify_active")
	secondPayload := readBody(t, second)
	close(release)
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("active replay status = %d, want 409: %s", second.StatusCode, secondPayload)
	}
	if code := decodeErrorCode(t, secondPayload); code != "SUIFY_ALREADY_ACTIVE" {
		t.Fatalf("active replay code = %q, want SUIFY_ALREADY_ACTIVE", code)
	}
	select {
	case result := <-firstResponse:
		if result.err != nil {
			t.Fatalf("first Suification request failed: %v", result.err)
		}
		first := result.response
		firstPayload := readBody(t, first)
		if first.StatusCode != http.StatusOK {
			t.Fatalf("first Suification status = %d: %s", first.StatusCode, firstPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first Suification did not finish")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent replay called the provider %d times, want 1", got)
	}
}

func TestCreateSuificationReportsRedisFailureAsDependencyUnavailable(t *testing.T) {
	server, pool, redisClient := newCareerServerWithSuify(t, func(_ context.Context, value string) (string, error) {
		return value, nil
	}, 5)
	defer server.Close()
	defer pool.Close()
	if err := redisClient.Close(); err != nil {
		t.Fatal(err)
	}

	response := send(t, server.URL, actorA, http.MethodPost, suificationsPath, []byte(`{"resume_text":"校内项目"}`), "idem_suify_redis_down")
	payload := readBody(t, response)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("Redis failure status = %d, want 503: %s", response.StatusCode, payload)
	}
	if code := decodeErrorCode(t, payload); code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("Redis failure code = %q, want DEPENDENCY_UNAVAILABLE", code)
	}
}
