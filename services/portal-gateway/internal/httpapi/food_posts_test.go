package httpapi

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/session"
)

const foodPostsSecret = "food-posts-gateway-secret-at-least-32-bytes"

const foodSessionUserID = "11111111-1111-4111-8111-111111111111"
const foodSessionDisplayName = "小河同学"

// The Food Post routes shadow the legacy Portal API food wildcard. The fake
// upstreams assert the boundary: session actor binding, independent create vs
// read credentials, envelope unwrap, verbatim error passthrough, and fail
// closed without any wildcard fallback.

func TestCreateFoodPostRequiresPortalSession(t *testing.T) {
	called := false
	food := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer food.Close()

	handler := newFoodPostsHandler(t, food.URL, unreachablePortalAPIURL())
	request := foodPostsRequest(t, handler, false, http.MethodPost, "/api/v1/food/posts", `{"venue_name":"x","campus":"jinming","tier":"hang","review_text":"y"}`, "idem_food_create_1")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("create without session status = %d, want 401: %s", response.Code, response.Body.String())
	}
	if called {
		t.Fatal("create without session contacted the Food upstream")
	}
}

func TestCreateFoodPostBindsPortalSessionActorAndIgnoresBrowserActor(t *testing.T) {
	const body = `{"venue_name":"老干妈拌饭","campus":"jinming","tier":"top","review_text":"真香","images":[{"content_type":"image/jpeg","data":"aGVsbG8="}]}`
	const idempotencyKey = "idem.food.create:2026-08-15_1"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/food/posts" {
			t.Fatalf("upstream request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Actor-User-Id"); got != foodSessionUserID {
			t.Fatalf("upstream X-Actor-User-Id = %q, want the Portal Session actor %q", got, foodSessionUserID)
		}
		if got := r.Header.Get("X-Actor-Display-Name"); got != foodSessionDisplayName {
			t.Fatalf("upstream X-Actor-Display-Name = %q, want the session snapshot %q", got, foodSessionDisplayName)
		}
		if got := r.Header.Get("Idempotency-Key"); got != idempotencyKey {
			t.Fatalf("upstream Idempotency-Key = %q, want %q", got, idempotencyKey)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != body {
			t.Fatalf("upstream body = %q, want the browser body byte-for-byte %q", raw, body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       map[string]any{"post": foodPostWireFixture()},
			"request_id": "req_food_create",
		})
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())
	request := foodPostsRequest(t, handler, true, http.MethodPost, "/api/v1/food/posts", body, idempotencyKey)
	// A browser must never select the actor: Gateway overwrites both headers.
	request.Header.Set("X-Actor-User-Id", "99999999-9999-4999-8999-999999999999")
	request.Header.Set("X-Actor-Display-Name", "浏览器伪造的名字")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"post"`, `"request_id":"req_food_create"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("create response omitted %s: %s", want, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("create response kept the Food envelope: %s", response.Body.String())
	}
}

func TestFoodPostPublicReadsHitFoodUpstreamNeverPortalAPI(t *testing.T) {
	var portalAPICalls int
	portalAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		portalAPICalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer portalAPI.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/food/posts":
			if r.URL.RawQuery != "campus=jinming" {
				t.Fatalf("upstream list query = %q, want campus=jinming", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       map[string]any{"posts": []any{foodPostWireFixture()}},
				"request_id": "req_food_list",
			})
		case "/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       map[string]any{"post": foodPostWireFixture(), "comments": []any{}},
				"request_id": "req_food_detail",
			})
		case "/api/v1/food/venues":
			if r.URL.RawQuery != "campus=minglun" {
				t.Fatalf("upstream venues query = %q, want campus=minglun", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       map[string]any{"campus": "minglun", "venues": []any{}},
				"request_id": "req_food_venues",
			})
		case "/api/v1/food/posts/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb":
			// A non-existent post is a plain 404 passthrough.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"FOOD_POST_NOT_FOUND","message":"内容不存在或已下架"},"request_id":"req_food_404"}`))
		case "/api/v1/food/posts/cccccccc-cccc-4ccc-8ccc-cccccccccccc":
			// A Food server fault is forwarded verbatim, never rewritten.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"FOOD_INTERNAL","message":"Food exploded"},"request_id":"req_food_500"}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, portalAPI.URL)

	tests := []struct {
		name string
		path string
		want []string
	}{
		{name: "list", path: "/api/v1/food/posts?campus=jinming", want: []string{`"posts"`, `"request_id":"req_food_list"`}},
		{name: "detail", path: "/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", want: []string{`"post"`, `"comments":[]`, `"request_id":"req_food_detail"`}},
		{name: "venues", path: "/api/v1/food/venues?campus=minglun", want: []string{`"campus":"minglun"`, `"request_id":"req_food_venues"`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://portal.test"+test.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("%s status = %d: %s", test.name, response.Code, response.Body.String())
			}
			for _, want := range test.want {
				if !strings.Contains(response.Body.String(), want) {
					t.Fatalf("%s response omitted %s: %s", test.name, want, response.Body.String())
				}
			}
			if strings.Contains(response.Body.String(), `"data"`) {
				t.Fatalf("%s response kept the Food envelope: %s", test.name, response.Body.String())
			}
		})
	}

	t.Run("detail 404 passthrough", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/food/posts/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", nil))
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "FOOD_POST_NOT_FOUND") {
			t.Fatalf("detail 404 passthrough = %d: %s", response.Code, response.Body.String())
		}
	})

	t.Run("food 500 passthrough", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/food/posts/cccccccc-cccc-4ccc-8ccc-cccccccccccc", nil))
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "FOOD_INTERNAL") {
			t.Fatalf("food 500 passthrough = %d: %s", response.Code, response.Body.String())
		}
	})

	if portalAPICalls != 0 {
		t.Fatalf("portal-api wildcard proxy received %d calls", portalAPICalls)
	}
}

func TestFoodPostNetworkFailureIsHonest502WithoutFallback(t *testing.T) {
	called := false
	portalAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer portalAPI.Close()

	handler := newFoodPostsHandler(t, "http://127.0.0.1:1", portalAPI.URL)
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/food/posts", nil))
	if response.Code != http.StatusBadGateway {
		t.Fatalf("network failure status = %d, want 502: %s", response.Code, response.Body.String())
	}
	if called {
		t.Fatal("network failure fell back to the portal-api wildcard proxy")
	}
	if !strings.Contains(response.Body.String(), "food_service_unavailable") {
		t.Fatalf("network failure body is not the honest error: %s", response.Body.String())
	}
}

func TestMyFoodPostsRequiresSessionAndBindsActor(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/v1/food/posts/mine" || r.Method != http.MethodGet {
			t.Fatalf("upstream request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Actor-User-Id"); got != foodSessionUserID {
			t.Fatalf("upstream X-Actor-User-Id = %q, want %q", got, foodSessionUserID)
		}
		if got := r.Header.Get("X-Actor-Display-Name"); got != "" {
			t.Fatalf("mine must bind only the actor user ID, got display name %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       map[string]any{"posts": []any{foodPostWireFixture()}},
			"request_id": "req_food_mine",
		})
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())

	request := foodPostsRequest(t, handler, false, http.MethodGet, "/api/v1/food/posts/mine", "", "")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("mine without session status = %d, want 401: %s", response.Code, response.Body.String())
	}
	if called {
		t.Fatal("mine without session contacted the Food upstream")
	}

	request = foodPostsRequest(t, handler, true, http.MethodGet, "/api/v1/food/posts/mine", "", "")
	response = httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"request_id":"req_food_mine"`) {
		t.Fatalf("mine with session response = %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("mine Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
}

func TestCreateFoodPostRejectsInvalidIdempotencyKeyBeforeUpstream(t *testing.T) {
	for _, key := range []string{"short", "bad key!", "only-7c", strings.Repeat("k", 201)} {
		t.Run(key, func(t *testing.T) {
			called := false
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer upstream.Close()

			handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())
			request := foodPostsRequest(t, handler, true, http.MethodPost, "/api/v1/food/posts", `{"venue_name":"x","campus":"jinming","tier":"hang","review_text":"y"}`, key)
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || called {
				t.Fatalf("invalid key %q status/called = %d/%t, want 400/false: %s", key, response.Code, called, response.Body.String())
			}
		})
	}
}

func TestFoodPostRoutesFailClosedWhenUnconfigured(t *testing.T) {
	portalAPICalls := 0
	portalAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		portalAPICalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer portalAPI.Close()

	handler := newFoodPostsUnconfiguredHandler(t, portalAPI.URL)

	routes := []struct {
		name        string
		method      string
		path        string
		withSession bool
	}{
		{name: "create", method: http.MethodPost, path: "/api/v1/food/posts", withSession: true},
		{name: "list", method: http.MethodGet, path: "/api/v1/food/posts"},
		{name: "mine", method: http.MethodGet, path: "/api/v1/food/posts/mine", withSession: true},
		{name: "detail", method: http.MethodGet, path: "/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"},
		{name: "image", method: http.MethodGet, path: "/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/images/0"},
		{name: "venues", method: http.MethodGet, path: "/api/v1/food/venues?campus=jinming"},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			body := ""
			key := ""
			if route.method == http.MethodPost {
				body = `{"venue_name":"x","campus":"jinming","tier":"hang","review_text":"y"}`
				key = "idem_food_unconfigured"
			}
			request := foodPostsRequest(t, handler, route.withSession, route.method, route.path, body, key)
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "food_posts_unavailable") {
				t.Fatalf("%s unconfigured status/body = %d: %s", route.name, response.Code, response.Body.String())
			}
		})
	}
	if portalAPICalls != 0 {
		t.Fatalf("unconfigured Food Post routes fell back to portal-api %d times", portalAPICalls)
	}
}

func TestFoodPostDailyCapErrorPassesThroughVerbatim(t *testing.T) {
	const upstreamBody = `{"error":{"code":"DAILY_POST_CAP_REACHED","message":"今天已经投满 3 条，明天再来吧"},"request_id":"req_food_cap"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(upstreamBody))
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())
	request := foodPostsRequest(t, handler, true, http.MethodPost, "/api/v1/food/posts", `{"venue_name":"x","campus":"jinming","tier":"hang","review_text":"y"}`, "idem_food_cap_1")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("daily cap status = %d, want 429: %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"code":"DAILY_POST_CAP_REACHED"`, "今天已经投满 3 条", `"request_id":"req_food_cap"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("daily cap response lost %s: %s", want, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), `"error":"`) && !strings.Contains(response.Body.String(), `"code"`) {
		t.Fatalf("daily cap body was rewritten by the Gateway envelope: %s", response.Body.String())
	}
}

func TestFoodPostEnvelopeUnwrapForListDetailCreate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/food/posts":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       map[string]any{"post": foodPostWireFixture()},
				"request_id": "req_food_create_unwrap",
			})
		case r.URL.Path == "/api/v1/food/posts":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       map[string]any{"posts": []any{foodPostWireFixture()}},
				"request_id": "req_food_list_unwrap",
			})
		case strings.HasPrefix(r.URL.Path, "/api/v1/food/posts/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":       map[string]any{"post": foodPostWireFixture(), "comments": []any{}},
				"request_id": "req_food_detail_unwrap",
			})
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())
	tests := []struct {
		name   string
		method string
		path   string
		want   []string
	}{
		{name: "create", method: http.MethodPost, path: "/api/v1/food/posts", want: []string{`"post"`, `"request_id":"req_food_create_unwrap"`}},
		{name: "list", method: http.MethodGet, path: "/api/v1/food/posts", want: []string{`"posts":[`, `"request_id":"req_food_list_unwrap"`}},
		{name: "detail", method: http.MethodGet, path: "/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", want: []string{`"post"`, `"comments":[]`, `"request_id":"req_food_detail_unwrap"`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := ""
			key := ""
			if test.method == http.MethodPost {
				body = `{"venue_name":"x","campus":"jinming","tier":"hang","review_text":"y"}`
				key = "idem_food_unwrap_1"
			}
			request := foodPostsRequest(t, handler, true, test.method, test.path, body, key)
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("%s status = %d: %s", test.name, response.Code, response.Body.String())
			}
			for _, want := range test.want {
				if !strings.Contains(response.Body.String(), want) {
					t.Fatalf("%s response omitted %s: %s", test.name, want, response.Body.String())
				}
			}
			if strings.Contains(response.Body.String(), `"data"`) {
				t.Fatalf("%s response kept the Food envelope: %s", test.name, response.Body.String())
			}
		})
	}
}

func TestFoodPostCreateAndReadUseIndependentCredentials(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.Header.Get("X-Service-Id")+" "+r.Header.Get("X-Key-Id"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"post": foodPostWireFixture()}, "request_id": "req_food_cred_create"})
		case r.URL.Path == "/api/v1/food/posts/mine":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"posts": []any{}}, "request_id": "req_food_cred_mine"})
		case r.URL.Path == "/api/v1/food/venues":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"campus": "jinming", "venues": []any{}}, "request_id": "req_food_cred_venues"})
		case strings.HasSuffix(r.URL.Path, "/images/0"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("photo-bytes"))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"post": foodPostWireFixture(), "comments": []any{}}, "request_id": "req_food_cred_detail"})
		}
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())
	router := handler.Router()

	create := foodPostsRequest(t, handler, true, http.MethodPost, "/api/v1/food/posts", `{"venue_name":"x","campus":"jinming","tier":"hang","review_text":"y"}`, "idem_food_cred_1")
	createResponse := httptest.NewRecorder()
	router.ServeHTTP(createResponse, create)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d: %s", createResponse.Code, createResponse.Body.String())
	}

	for _, path := range []string{
		"/api/v1/food/posts?campus=jinming",
		"/api/v1/food/posts/mine",
		"/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/images/0",
		"/api/v1/food/venues?campus=jinming",
	} {
		request := foodPostsRequest(t, handler, true, http.MethodGet, path, "", "")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("read %s status = %d: %s", path, response.Code, response.Body.String())
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 6 {
		t.Fatalf("upstream saw %d requests, want 6: %v", len(seen), seen)
	}
	for _, entry := range seen {
		parts := strings.Split(entry, " ")
		method, path, serviceID, keyID := parts[0], parts[1], parts[2], parts[3]
		if method == http.MethodPost {
			if serviceID != "food-post-create" || keyID != "create-key" {
				t.Fatalf("create used wrong credential: %s", entry)
			}
		} else {
			if serviceID != "food-post-read" || keyID != "read-key" {
				t.Fatalf("read %s used wrong credential: %s", path, entry)
			}
		}
	}
}

func TestFoodPostImagePassesThroughBytesAndCacheHeaders(t *testing.T) {
	const body = "\x89PNG\r\n\x1a\nphoto-bytes"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/images/1" {
			t.Fatalf("upstream path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("ETag", `"sha256abc"`)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/images/1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("image status = %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("image Content-Type = %q, want image/png", got)
	}
	if got := response.Header().Get("ETag"); got != `"sha256abc"` {
		t.Fatalf("image ETag = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("image Cache-Control = %q", got)
	}
	if response.Body.String() != body {
		t.Fatalf("image body = %q, want upstream bytes unchanged", response.Body.String())
	}
}

func TestFoodPostImage404PassesThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"FOOD_POST_IMAGE_NOT_FOUND","message":"图片不存在"},"request_id":"req_food_image_404"}`))
	}))
	defer upstream.Close()

	handler := newFoodPostsHandler(t, upstream.URL, unreachablePortalAPIURL())
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/images/0", nil))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "FOOD_POST_IMAGE_NOT_FOUND") {
		t.Fatalf("image 404 passthrough = %d: %s", response.Code, response.Body.String())
	}
}

func TestUnrelatedPortalAPIRoutesStillProxyAsBefore(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/food/posts/survey-01/comments":
			_ = json.NewEncoder(w).Encode(map[string]any{"comments": []any{}, "request_id": "req_legacy_comments"})
		case "/api/v1/campus/items":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "request_id": "req_legacy_campus"})
		default:
			t.Fatalf("unexpected portal-api path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := newFoodPostsUnconfiguredHandler(t, upstream.URL)
	for _, path := range []string{"/api/v1/food/posts/survey-01/comments", "/api/v1/campus/items"} {
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://portal.test"+path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("legacy proxy %s status = %d: %s", path, response.Code, response.Body.String())
		}
	}
}

// --- Helpers ---

func foodPostWireFixture() map[string]any {
	return map[string]any{
		"id":      "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"campus":  "jinming",
		"title":   "老干妈拌饭",
		"excerpt": "真香",
		"author":  "小河同学",
		"likes":   0,
		"stars":   0,
		"tags":    []any{"顶级"},
		"shop":    map[string]any{"name": "老干妈拌饭"},
		"time":    "2026-08-15T08:00:00Z",
		"hidden":  false,
		"images":  []any{"/api/v1/food/posts/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa/images/0"},
		"blocks":  []any{},
	}
}

func newFoodPostsHandler(t *testing.T, foodURL, portalAPIURL string) *Handler {
	t.Helper()
	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		FoodPostsURL: foodURL,
		FoodPostCreateAuth: config.ServiceAuth{
			ClientID: "food-post-create", ClientSecret: foodPostsSecret, KeyID: "create-key",
		},
		FoodPostReadAuth: config.ServiceAuth{
			ClientID: "food-post-read", ClientSecret: foodPostsSecret, KeyID: "read-key",
		},
		PortalAPIURL: portalAPIURL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func newFoodPostsUnconfiguredHandler(t *testing.T, portalAPIURL string) *Handler {
	t.Helper()
	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL: portalAPIURL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func unreachablePortalAPIURL() string {
	return "http://127.0.0.1:1"
}

func foodPostsRequest(t *testing.T, handler *Handler, withSession bool, method, path, body, idempotencyKey string) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, "https://portal.test"+path, reader)
	request.TLS = &tls.ConnectionState{}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if withSession {
		encoded, err := handler.sessionCodec.Encode(session.Value{
			UserID:        foodSessionUserID,
			DisplayName:   foodSessionDisplayName,
			ExchangeToken: strings.Repeat("x", 32),
			ExpiresAt:     time.Now().Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		request.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_session", Value: encoded})
	}
	return request
}
