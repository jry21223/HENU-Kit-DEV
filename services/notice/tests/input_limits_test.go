package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	notice "henukit.dev/notice"
)

func TestNoticeVersionWriteHonorsUnicodeBodyLimitOverHTTP(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	handler, err := notice.New(notice.Config{
		Database:       pool,
		Redis:          redisClient,
		ClientID:       "console-gateway",
		Keys:           map[string]string{"active": testSecret},
		PortalClientID: "portal-gateway-notice-read",
		PortalKeys:     map[string]string{"portal-active": portalFeedSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	actor := uuid.NewString()
	source := send(t, server.URL, actor, "notice.manage", http.MethodPost, "/api/v1/sources", `{"code":"unicode-body-`+uuid.NewString()+`","name":"学校办公室","canonical_url":"https://example.edu/notices"}`, "idem_unicode_body_source")
	sourceID := dataString(t, source, "id")
	path := "/api/v1/sources/" + sourceID + "/versions"

	maximum, err := json.Marshal(map[string]string{
		"title":      "100000 个 CJK 字符",
		"body":       strings.Repeat("学", 100000),
		"source_url": "https://example.edu/notices/maximum-cjk",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := signedRequest(t, server.URL, actor, "notice.manage", "product", http.MethodPost, path, string(maximum), "idem_unicode_body_maximum")
	maximumPayload, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("100000-rune CJK version write = %d: %s", response.StatusCode, maximumPayload)
	}

	overMaximum, err := json.Marshal(map[string]string{
		"title":      "100001 个 CJK 字符",
		"body":       strings.Repeat("学", 100001),
		"source_url": "https://example.edu/notices/over-maximum-cjk",
	})
	if err != nil {
		t.Fatal(err)
	}
	response = signedRequest(t, server.URL, actor, "notice.manage", "product", http.MethodPost, path, string(overMaximum), "idem_unicode_body_over_maximum")
	overMaximumPayload, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(overMaximumPayload), `"code":"INVALID_REQUEST"`) {
		t.Fatalf("100001-rune CJK version write = %d: %s", response.StatusCode, overMaximumPayload)
	}

	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notice_versions WHERE source_id=$1`, sourceID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 {
		t.Fatalf("CJK body boundary persisted %d versions, want exactly one", versions)
	}
}
