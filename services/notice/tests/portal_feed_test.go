package tests

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	notice "henukit.dev/notice"
)

const portalFeedSecret = "portal-notice-read-secret-at-least-32-bytes"

func TestPortalFeedIsActorBoundAndFiltersBeforeLimit(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
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
	sourceID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '学校办公室', 'https://example.edu/notices', $3)`, sourceID, "portal-feed-"+uuid.NewString(), actor); err != nil {
		t.Fatal(err)
	}

	// This eligible version is deliberately older than more than the snapshot
	// limit of ineligible versions: filtering must happen in Owner SQL before
	// the final order and LIMIT.
	eligible := insertPortalFeedVersion(t, pool, sourceID, actor, 1, "全校 in-app 通知", "全校学生可以查看的正文", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(-2*time.Hour))
	excluded := make(map[string]struct{}, 54)
	for index := 0; index < 51; index++ {
		versionID := insertPortalFeedVersion(t, pool, sourceID, actor, index+2, fmt.Sprintf("仅邮件 %d", index), "这条不能进入 Portal", "distributed", "email", "all_students", nil, time.Now().UTC().Add(time.Duration(index)*time.Minute))
		excluded[versionID.String()] = struct{}{}
	}
	college := "信息工程学院"
	collegeVersion := insertPortalFeedVersion(t, pool, sourceID, actor, 60, "学院定向", "不能进入 Portal", "distributed", "in_app", "college", &college, time.Now().UTC())
	excluded[collegeVersion.String()] = struct{}{}
	role := "counselor"
	roleVersion := insertPortalFeedVersion(t, pool, sourceID, actor, 61, "角色定向", "不能进入 Portal", "distributed", "in_app", "role", &role, time.Now().UTC())
	excluded[roleVersion.String()] = struct{}{}
	undistributedVersion := insertPortalFeedVersion(t, pool, sourceID, actor, 62, "尚未分发", "不能进入 Portal", "approved", "in_app", "all_students", nil, time.Now().UTC())
	excluded[undistributedVersion.String()] = struct{}{}

	// Legacy rows may predate the source-origin write policy. More than the
	// final feed cap are deliberately newer than the eligible item: the Owner
	// must discard them before it applies the cap, or a malformed/mismatched
	// legacy URL could both hide the valid item and poison Gateway's strict
	// whole-feed validation.
	legacySourceID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧来源', 'https://example.edu/notices', $3)`, legacySourceID, "portal-legacy-origin-"+uuid.NewString(), actor); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 51; index++ {
		legacyID := insertPortalFeedVersionWithURL(t, pool, legacySourceID, actor, index+1, "旧跨来源 "+fmt.Sprint(index), "这条旧记录不能进入 Portal", fmt.Sprintf("https://other.example.edu/notices/%d", index), "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(2*time.Hour+time.Duration(index)*time.Minute))
		excluded[legacyID.String()] = struct{}{}
	}
	malformedID := insertPortalFeedVersionWithURL(t, pool, legacySourceID, actor, 52, "旧格式错误", "这条旧记录不能进入 Portal", "https:///notices", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(4*time.Hour))
	excluded[malformedID.String()] = struct{}{}
	privateSourceID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧内网来源', 'https://127.0.0.1/notices', $3)`, privateSourceID, "portal-legacy-private-"+uuid.NewString(), actor); err != nil {
		t.Fatal(err)
	}
	privateID := insertPortalFeedVersionWithURL(t, pool, privateSourceID, actor, 1, "旧内网来源", "这条旧记录不能进入 Portal", "https://127.0.0.1/notices/1", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(5*time.Hour))
	excluded[privateID.String()] = struct{}{}
	ambiguousIPSourceID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧歧义地址来源', 'https://127.1/notices', $3)`, ambiguousIPSourceID, "portal-legacy-ambiguous-ip-"+uuid.NewString(), actor); err != nil {
		t.Fatal(err)
	}
	ambiguousIPID := insertPortalFeedVersionWithURL(t, pool, ambiguousIPSourceID, actor, 1, "旧歧义地址来源", "这条旧记录不能进入 Portal", "https://127.1/notices/1", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(6*time.Hour))
	excluded[ambiguousIPID.String()] = struct{}{}
	unicodeDotSourceID := uuid.New()
	unicodeDotCanonical := "https://127\uFF0E0.0.1/notices"
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧 Unicode 地址来源', $3, $4)`, unicodeDotSourceID, "portal-legacy-unicode-ip-"+uuid.NewString(), unicodeDotCanonical, actor); err != nil {
		t.Fatal(err)
	}
	unicodeDotID := insertPortalFeedVersionWithURL(t, pool, unicodeDotSourceID, actor, 1, "旧 Unicode 地址来源", "这条旧记录不能进入 Portal", unicodeDotCanonical+"/1", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(7*time.Hour))
	excluded[unicodeDotID.String()] = struct{}{}
	invalidDNSSourceID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧无效 DNS 来源', 'https://foo_.example.edu/notices', $3)`, invalidDNSSourceID, "portal-dns-"+uuid.NewString(), actor); err != nil {
		t.Fatal(err)
	}
	invalidDNSID := insertPortalFeedVersionWithURL(t, pool, invalidDNSSourceID, actor, 1, "旧无效 DNS 来源", "这条旧记录不能进入 Portal", "https://foo_.example.edu/notices/1", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(8*time.Hour))
	excluded[invalidDNSID.String()] = struct{}{}
	kelvinSourceID := uuid.New()
	kelvinCanonical := "https://\u212A.example.edu/notices"
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧 Unicode 折叠来源', $3, $4)`, kelvinSourceID, "portal-kelvin-"+uuid.NewString(), kelvinCanonical, actor); err != nil {
		t.Fatal(err)
	}
	kelvinID := insertPortalFeedVersionWithURL(t, pool, kelvinSourceID, actor, 1, "旧 Unicode 折叠来源", "这条旧记录不能进入 Portal", kelvinCanonical+"/1", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(9*time.Hour))
	excluded[kelvinID.String()] = struct{}{}
	for index, legacyNumericOrigin := range []string{"https://foo.0x7f/notices", "https://example.127/notices", "https://example.0x/notices", "https://xn--a.example/notices", "https://xn--0.example/notices"} {
		legacyNumericSourceID := uuid.New()
		if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '旧数字 DNS 尾部来源', $3, $4)`, legacyNumericSourceID, "portal-numeric-tail-"+uuid.NewString(), legacyNumericOrigin, actor); err != nil {
			t.Fatal(err)
		}
		legacyNumericID := insertPortalFeedVersionWithURL(t, pool, legacyNumericSourceID, actor, 1, "旧数字 DNS 尾部来源", "这条旧记录不能进入 Portal", legacyNumericOrigin+"/1", "distributed", "in_app", "all_students", nil, time.Now().UTC().Add(time.Duration(10+index)*time.Hour))
		excluded[legacyNumericID.String()] = struct{}{}
	}

	response, err := http.DefaultClient.Do(portalFeedRequest(t, server.URL, actor))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("portal feed = %d: %s", response.StatusCode, payload)
	}
	var envelope struct {
		Data struct {
			Notices []struct {
				ID     string `json:"id"`
				Title  string `json:"title"`
				Body   string `json:"body"`
				Source struct {
					Name string `json:"name"`
					URL  string `json:"url"`
				} `json:"source"`
				CreatedAt time.Time `json:"created_at"`
			} `json:"notices"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Data.Notices) > 50 {
		t.Fatalf("Portal feed returned %d notices, want at most 50", len(envelope.Data.Notices))
	}
	var item *struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		Source struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"source"`
		CreatedAt time.Time `json:"created_at"`
	}
	for index := range envelope.Data.Notices {
		notice := &envelope.Data.Notices[index]
		if _, forbidden := excluded[notice.ID]; forbidden {
			t.Fatalf("Portal feed exposed excluded version %s: %#v", notice.ID, envelope.Data.Notices)
		}
		if notice.ID == eligible.String() {
			item = notice
		}
	}
	if item == nil {
		t.Fatalf("Portal feed omitted eligible all-students in-app version %s: %#v", eligible, envelope.Data.Notices)
	}
	if item.Title == "" || item.Body == "" || item.Source.Name == "" || !strings.HasPrefix(item.Source.URL, "https://example.edu/notices/1-") || item.CreatedAt.IsZero() {
		t.Fatalf("Portal feed omitted a public field: %#v", item)
	}

	// A header swap after signing must not turn this request into another actor.
	mutated := portalFeedRequest(t, server.URL, actor)
	mutated.Header.Set("X-Actor-User-Id", uuid.NewString())
	mutatedResponse, err := http.DefaultClient.Do(mutated)
	if err != nil {
		t.Fatal(err)
	}
	mutatedResponse.Body.Close()
	if mutatedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("actor-swapped Portal signature = %d, want 401", mutatedResponse.StatusCode)
	}

	console := signedRequest(t, server.URL, actor, "notice.read", "product", http.MethodGet, "/api/v1/portal/notices", "", "")
	console.Body.Close()
	if console.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Console credential reached Portal feed: %d", console.StatusCode)
	}
	portalManagement := portalFeedRequest(t, server.URL, actor)
	portalManagement.URL.Path = "/api/v1/console-notices"
	portalManagement.URL.RawPath = ""
	portalManagement.RequestURI = ""
	managementResponse, err := http.DefaultClient.Do(portalManagement)
	if err != nil {
		t.Fatal(err)
	}
	managementResponse.Body.Close()
	if managementResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Portal credential reached Console snapshot: %d", managementResponse.StatusCode)
	}
}

func TestPortalFeedRejectsQueryAndBodyOnItsFixedReadRoute(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
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

	for _, testCase := range []struct {
		name  string
		query string
		body  string
	}{
		{name: "query", query: "unexpected=1"},
		{name: "body", body: `{"unexpected":true}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response, err := http.DefaultClient.Do(portalFeedRequestWith(t, server.URL, uuid.NewString(), testCase.query, testCase.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				payload, _ := io.ReadAll(response.Body)
				t.Fatalf("Portal feed %s request = %d: %s", testCase.name, response.StatusCode, payload)
			}
		})
	}
}

func TestPortalFeedBoundsEncodedResponseBeforeFinalItemCap(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), os.Getenv("NOTICE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("NOTICE_TEST_REDIS_ADDR")})
	defer redisClient.Close()
	if err := redisClient.FlushDB(context.Background()).Err(); err != nil {
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
	sourceID := uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '学校办公室', 'https://example.edu/notices', $3)`, sourceID, "portal-feed-byte-budget-"+uuid.NewString(), actor); err != nil {
		t.Fatal(err)
	}

	// Each body is valid under the 100,000-rune public contract but occupies
	// roughly 300 KB in UTF-8. Twenty-one newer candidates exceed Gateway's
	// 6 MiB Owner-response read bound unless Owner applies its own byte budget.
	now := time.Now().UTC()
	largeBody := strings.Repeat("学", 100000)
	for index := 0; index < 21; index++ {
		insertPortalFeedVersion(t, pool, sourceID, actor, index+1, fmt.Sprintf("大正文 %d", index), largeBody, "distributed", "in_app", "all_students", nil, now.Add(time.Duration(21-index)*time.Minute))
	}
	// This long, persisted same-origin URL stays within PostgreSQL's existing
	// unique-index boundary. It proves the HTTP path continues to a later valid
	// source URL after it skips the oversized newer bodies.
	longSourceURL := "https://example.edu/notices/" + strings.Repeat("u", 1800)
	olderSmall := insertPortalFeedVersionWithURL(t, pool, sourceID, actor, 22, "较早的小通知", "这条较早但仍合格的通知证明 Owner 会在跳过超预算候选后继续扫描。", longSourceURL, "distributed", "in_app", "all_students", nil, now.Add(-time.Minute))

	response, err := http.DefaultClient.Do(portalFeedRequest(t, server.URL, actor))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("portal feed = %d: %s", response.StatusCode, payload)
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 5<<20 {
		t.Fatalf("Owner response = %d bytes, want at most the 5 MiB Owner budget", len(payload))
	}
	if len(payload) > 6<<20 {
		t.Fatalf("Owner response = %d bytes, exceeds Gateway's 6 MiB read bound", len(payload))
	}
	var envelope struct {
		Data struct {
			Notices []struct {
				ID string `json:"id"`
			} `json:"notices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("bounded Owner response is not consumable JSON: %v", err)
	}
	if len(envelope.Data.Notices) > 50 {
		t.Fatalf("Portal feed returned %d notices, want at most 50", len(envelope.Data.Notices))
	}
	for _, item := range envelope.Data.Notices {
		if item.ID == olderSmall.String() {
			return
		}
	}
	t.Fatalf("Portal feed did not continue past over-budget candidates to older valid notice %s", olderSmall)
}

func TestPortalFeedBoundsCandidateScanBeforeFinalItemCap(t *testing.T) {
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
	sourceID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO notice_sources (id, code, name, canonical_url, created_by) VALUES ($1, $2, '学校办公室', 'https://example.edu/notices', $3)`, sourceID, "portal-candidate-"+uuid.NewString(), actor); err != nil {
		t.Fatal(err)
	}
	// The candidate window is intentionally larger than the final 50 items, so
	// old malformed rows do not crowd out valid later items. The item after the
	// documented window must not make the Owner scan unbounded history. Keep
	// this globally newest-first fixture independent of other integration tests
	// sharing the database, so it owns the first 201 candidates.
	now := time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 199; index++ {
		insertPortalFeedVersionWithURL(t, pool, sourceID, actor, index+1, "旧格式错误", "这条旧记录不会进入 Portal。", fmt.Sprintf("https:///notices/%d", index), "distributed", "in_app", "all_students", nil, now.Add(time.Duration(201-index)*time.Minute))
	}
	insideWindow := insertPortalFeedVersion(t, pool, sourceID, actor, 200, "窗口内有效通知", "这条通知位于候选窗口内，应在旧坏记录之后继续被考虑。", "distributed", "in_app", "all_students", nil, now.Add(2*time.Minute))
	beyondWindow := insertPortalFeedVersion(t, pool, sourceID, actor, 201, "窗口外有效通知", "这条通知位于候选窗口之外，不应让请求无限扫描历史。", "distributed", "in_app", "all_students", nil, now.Add(time.Minute))

	response, err := http.DefaultClient.Do(portalFeedRequest(t, server.URL, actor))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("portal feed = %d: %s", response.StatusCode, payload)
	}
	var envelope struct {
		Data struct {
			Notices []struct {
				ID string `json:"id"`
			} `json:"notices"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, item := range envelope.Data.Notices {
		seen[item.ID] = true
	}
	if !seen[insideWindow.String()] {
		t.Fatalf("candidate window did not consider valid item %s inside its boundary", insideWindow)
	}
	if seen[beyondWindow.String()] {
		t.Fatalf("candidate window scanned valid item %s beyond its boundary", beyondWindow)
	}
}

func insertPortalFeedVersion(t *testing.T, pool *pgxpool.Pool, sourceID uuid.UUID, actor string, version int, title, body, state, channel, audienceKind string, audienceValue *string, createdAt time.Time) uuid.UUID {
	t.Helper()
	versionID := uuid.New()
	sourceURL := fmt.Sprintf("https://example.edu/notices/%d-%s", version, versionID)
	return insertPortalFeedVersionWithURL(t, pool, sourceID, actor, version, title, body, sourceURL, state, channel, audienceKind, audienceValue, createdAt)
}

func insertPortalFeedVersionWithURL(t *testing.T, pool *pgxpool.Pool, sourceID uuid.UUID, actor string, version int, title, body, sourceURL, state, channel, audienceKind string, audienceValue *string, createdAt time.Time) uuid.UUID {
	t.Helper()
	versionID := uuid.New()
	hash := sha256.Sum256([]byte(body + sourceURL))
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_versions (id, source_id, version, title, body, source_url, content_hash, created_by, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, versionID, sourceID, version, title, body, sourceURL, hex.EncodeToString(hash[:]), actor, createdAt); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_lifecycles (notice_version_id, state) VALUES ($1,$2)`, versionID, state); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO notice_distributions (id, notice_version_id, channel, audience_kind, audience_value, actor_user_id, request_id, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, uuid.New(), versionID, channel, audienceKind, audienceValue, actor, "req_portal_feed_"+uuid.NewString(), createdAt); err != nil {
		t.Fatal(err)
	}
	return versionID
}

func portalFeedRequest(t *testing.T, baseURL, actor string) *http.Request {
	return portalFeedRequestWith(t, baseURL, actor, "", "")
}

func portalFeedRequestWith(t *testing.T, baseURL, actor, query, body string) *http.Request {
	t.Helper()
	target := baseURL + "/api/v1/portal/notices"
	if query != "" {
		target += "?" + query
	}
	request, err := http.NewRequest(http.MethodGet, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	nonce := base64.RawURLEncoding.EncodeToString([]byte(uuid.NewString()[:24]))
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	digest := sha256.Sum256([]byte(body))
	canonical := strings.Join([]string{http.MethodGet, request.URL.RequestURI(), timestamp, nonce, hex.EncodeToString(digest[:]), actor}, "\n")
	mac := hmac.New(sha256.New, []byte(portalFeedSecret))
	_, _ = mac.Write([]byte(canonical))
	request.SetBasicAuth("portal-gateway-notice-read", portalFeedSecret)
	request.Header.Set("X-Service-Id", "portal-gateway-notice-read")
	request.Header.Set("X-Key-Id", "portal-active")
	request.Header.Set("X-Timestamp", timestamp)
	request.Header.Set("X-Nonce", nonce)
	request.Header.Set("X-Signature", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-Actor-User-Id", actor)
	request.Header.Set("X-Request-Id", "req_"+uuid.NewString())
	return request
}
