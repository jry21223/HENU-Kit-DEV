package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

func TestOperationsDirectoryAndAccountReadRoutes(t *testing.T) {
	redisClient := testRedis(t)
	codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
	token := "exchange_token_with_at_least_32_characters"
	userID := "22222222-2222-4222-8222-222222222222"
	itemID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"

	platform := &fakePlatform{
		exchange: platformcore.Exchange{ExchangeToken: token},
	}
	notice := &fakeNotice{
		result: json.RawMessage(`{"sources":[{"id":"b1111111-1111-4111-8111-111111111101","code":"jwc-henu","name":"河南大学教务处","canonical_url":"https://jwc.henu.edu.cn/","created_at":"2026-11-20T00:00:00Z"}]}`),
	}
	account := &fakeAccountPortfolio{
		points: json.RawMessage(`{"balance":320,"entries":[{"id":"c1111111-1111-4111-8111-111111111101","amount":200,"reason":"资料奖励","created_at":"2026-11-20T00:00:00Z"}]}`),
		order:  json.RawMessage(`{"orders":[{"id":"c2222222-2222-4222-8222-222222222201","user_id":"22222222-2222-4222-8222-222222222222","plan":"lifetime","amount_cents":990,"status":"paid","version":2,"created_at":"2026-11-10T00:00:00Z","updated_at":"2026-11-10T00:00:00Z"}]}`),
	}
	handler, err := New(
		"https://account.henukit.test",
		"console-gateway",
		"https://console.henukit.test/api/v1/auth/callback",
		platform, notice, fakeOverview{}, redisClient, codec,
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		nil, nil, account,
	)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(handler)
	defer server.Close()

	encoded, _ := codec.Encode(session.Value{
		UserID:        "11111111-1111-4111-8111-111111111111",
		ExchangeToken: token,
		ExpiresAt:     time.Now().Add(time.Minute),
	})

	do := func(method, path string, withCookie bool, origin, idempotencyKey string, body []byte) *http.Response {
		t.Helper()
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequest(method, server.URL+path, reader)
		if err != nil {
			t.Fatal(err)
		}
		if withCookie {
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// 1. Unauthenticated returns 401 across all 5 routes
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/notices/sources"},
		{http.MethodGet, "/api/v1/operations/roles"},
		{http.MethodPost, "/api/v1/operations/inbox/" + itemID + "/updates"},
		{http.MethodGet, "/api/v1/account/points/" + userID},
		{http.MethodGet, "/api/v1/account/membership-orders"},
	} {
		res := do(tc.method, tc.path, false, "", "", nil)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s unauthenticated = %d, want 401", tc.method, tc.path, res.StatusCode)
		}
	}

	// 2. GET /api/v1/notices/sources
	res := do(http.MethodGet, "/api/v1/notices/sources", true, "", "", nil)
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(raw), "jwc-henu") {
		t.Fatalf("GET notices/sources = %d %s", res.StatusCode, raw)
	}
	res = do(http.MethodGet, "/api/v1/notices/sources?unexpected=1", true, "", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET notices/sources with query = %d, want 400", res.StatusCode)
	}

	// 3. GET /api/v1/operations/roles
	res = do(http.MethodGet, "/api/v1/operations/roles", true, "", "", nil)
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(raw), "platform-operator") {
		t.Fatalf("GET operations/roles = %d %s", res.StatusCode, raw)
	}

	// 4. POST /api/v1/operations/inbox/{item_id}/updates (Origin + Idempotency + Happy path)
	res = do(http.MethodPost, "/api/v1/operations/inbox/"+itemID+"/updates", true, "https://evil.example", "idem-inbox-12345", []byte(`{"expected_version":1,"status":"in_progress"}`))
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("POST inbox update cross-origin = %d, want 403", res.StatusCode)
	}
	res = do(http.MethodPost, "/api/v1/operations/inbox/"+itemID+"/updates", true, server.URL, "idem-inbox-12345", []byte(`{"expected_version":1,"status":"in_progress"}`))
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"in_progress"`) {
		t.Fatalf("POST inbox update valid = %d %s", res.StatusCode, raw)
	}
	for _, archiveStatus := range []string{"cancelled", "archived"} {
		res = do(http.MethodPost, "/api/v1/operations/inbox/"+itemID+"/updates", true, server.URL, "idem-inbox-archive-"+archiveStatus, []byte(`{"expected_version":2,"status":"`+archiveStatus+`"}`))
		raw, _ = io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("POST inbox archive status=%s = %d %s", archiveStatus, res.StatusCode, raw)
		}
	}
	res = do(http.MethodPost, "/api/v1/operations/inbox/not-a-uuid/updates", true, server.URL, "idem-inbox-12345", []byte(`{"expected_version":1,"status":"resolved"}`))
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST inbox update bad uuid = %d, want 400", res.StatusCode)
	}

	// 5. GET /api/v1/account/points/{user_id}
	res = do(http.MethodGet, "/api/v1/account/points/"+userID+"?limit=20", true, "", "", nil)
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"balance":320`) {
		t.Fatalf("GET account/points = %d %s", res.StatusCode, raw)
	}
	res = do(http.MethodGet, "/api/v1/account/points/not-a-uuid", true, "", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET account/points invalid uuid = %d, want 400", res.StatusCode)
	}

	// 6. GET /api/v1/account/membership-orders (including identity enrichment & validation)
	res = do(http.MethodGet, "/api/v1/account/membership-orders?status=paid&limit=50", true, "", "", nil)
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"c2222222-2222-4222-8222-222222222201"`) || !strings.Contains(string(raw), `"email"`) {
		t.Fatalf("GET account/membership-orders = %d %s", res.StatusCode, raw)
	}
	res = do(http.MethodGet, "/api/v1/account/membership-orders?status=bogus", true, "", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("GET account/membership-orders invalid status = %d, want 400", res.StatusCode)
	}

	// 7. Forbidden checks (403)
	platform.accountErrors = map[string]error{"account.orders.read": platformcore.ErrForbidden, "account.points.adjust": platformcore.ErrForbidden}
	res = do(http.MethodGet, "/api/v1/account/membership-orders", true, "", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("GET account/membership-orders forbidden = %d, want 403", res.StatusCode)
	}
	res = do(http.MethodGet, "/api/v1/account/points/"+userID, true, "", "", nil)
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("GET account/points forbidden = %d, want 403", res.StatusCode)
	}
}

func TestSameOriginAllowedStrictProductionAndLoopbackHTTP(t *testing.T) {
	cases := []struct {
		name           string
		host           string
		origin         string
		forwardedProto string
		want           bool
	}{
		{"prod https exact", "console.henukit.cn", "https://console.henukit.cn", "https", true},
		{"prod http denied", "console.henukit.cn", "http://console.henukit.cn", "", false},
		{"prod cross origin denied", "console.henukit.cn", "https://evil.example", "https", false},
		{"localhost http allowed", "localhost:8089", "http://localhost:8089", "http", true},
		{"127.0.0.1 http allowed", "127.0.0.1:8089", "http://127.0.0.1:8089", "", true},
		{"localhost http denied if forwarded https", "localhost:8089", "http://localhost:8089", "https", false},
		{"localhost cross port denied", "localhost:8089", "http://localhost:8088", "", false},
		{"missing origin denied", "localhost:8089", "", "", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "http://"+tc.host+"/api/v1/test", nil)
		req.Host = tc.host
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.forwardedProto != "" {
			req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
		}
		if got := sameOriginAllowed(req); got != tc.want {
			t.Errorf("%s: sameOriginAllowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}
