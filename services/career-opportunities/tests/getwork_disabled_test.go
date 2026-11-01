package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/redis/go-redis/v9"

	career "henukit.dev/career"
)

func TestDisabledJobSourceRetainsHistoryAndFailsQueuedScanWithoutDigest(t *testing.T) {
	ctx := context.Background()
	digests := &digestRecorder{}
	enabled, pool := newCareerServerWithDigest(t, digestWork(), digests, "https://henukit.cn/career")
	defer enabled.Close()
	defer pool.Close()
	seedProfileNotifications(t, pool, actorA, true)
	created := send(t, enabled.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_before_disabled")
	if created.StatusCode != http.StatusOK {
		t.Fatalf("initial create=%d %s", created.StatusCode, readBody(t, created))
	}
	id := decodeData(t, readBody(t, created))["search"].(map[string]any)["id"].(string)
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("CAREER_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	var invoked atomic.Bool
	service, err := career.New(career.Config{
		Database: pool, Redis: redisClient, ClientID: careerClientID, Keys: map[string]string{"active": careerSecret},
		JobSourceDisabled: true, DigestSender: digests, DigestResultURL: "https://henukit.cn/career",
		Work: func(context.Context, any) (career.WorkResult, error) {
			invoked.Store(true)
			return career.WorkResult{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	disabled := httptest.NewServer(service)
	defer disabled.Close()
	create := send(t, disabled.URL, actorA, http.MethodPost, "/api/v1/career/searches", profileBody(actorA), "idem_after_disabled")
	body := readBody(t, create)
	if create.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), `"code":"JOB_SOURCE_DISABLED"`) {
		t.Fatalf("disabled create=%d %s", create.StatusCode, body)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM career_searches`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("disabled source created a new task; searches=%d", count)
	}
	history := send(t, disabled.URL, actorA, http.MethodGet, "/api/v1/career/searches", nil, "")
	if history.StatusCode != http.StatusOK || len(decodeData(t, readBody(t, history))["searches"].([]any)) != 1 {
		t.Fatal("disabled source made existing history unavailable")
	}
	profile := send(t, disabled.URL, actorA, http.MethodGet, "/api/v1/career/profile", nil, "")
	if profile.StatusCode != http.StatusOK {
		t.Fatalf("disabled source made profile unavailable: %d %s", profile.StatusCode, readBody(t, profile))
	}
	_ = readBody(t, profile)
	claimed, err := service.Claims().Step(ctx)
	if err != nil || !claimed {
		t.Fatalf("queued scan claim=%t err=%v", claimed, err)
	}
	status := send(t, disabled.URL, actorA, http.MethodGet, "/api/v1/career/searches/"+id, nil, "")
	search := decodeData(t, readBody(t, status))["search"].(map[string]any)
	if status.StatusCode != http.StatusOK || search["status"] != "failed" || search["error_code"] != "JOB_SOURCE_DISABLED" || search["result"] != nil {
		t.Fatalf("retained task did not fail honestly: %+v", search)
	}
	if invoked.Load() || digests.count() != 0 {
		t.Fatalf("disabled source ran work=%t or emitted %d digests", invoked.Load(), digests.count())
	}
	for _, table := range []string{"career_search_results", "career_digest_deliveries"} {
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE search_id=$1", id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("disabled source wrote %d %s rows", count, table)
		}
	}
}
