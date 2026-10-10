package mcp_test

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
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"henukit.dev/food-mcp/internal/foodclient"
	mcp "henukit.dev/food-mcp/internal/mcp"
	"henukit.dev/food-mcp/internal/sessionclient"
)

const (
	testCreateClientID = "portal-gateway-create"
	testCreateSecret   = "food-post-create-secret-at-least-32-bytes"
	testReadClientID   = "portal-gateway-read"
	testReadSecret     = "food-post-read-secret-at-least-32-bytes"
	testActorID        = "44444444-4444-4444-8444-444444444444"
	testActorName      = "MCP 测试同学"
)

// fakeFood is a minimal Food service: it verifies the five-line signature,
// records the signed request, and answers canned responses.
type fakeFood struct {
	createRequests []*http.Request
	readRequests   []*http.Request
	nextCreate     func(body map[string]any) (int, map[string]any)
}

func (f *fakeFood) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		secret := testReadSecret
		clientID := r.Header.Get("X-Service-Id")
		if strings.HasSuffix(r.URL.Path, "/posts") && r.Method == http.MethodPost {
			secret = testCreateSecret
		}
		if !validSignature(r, body, secret, clientID) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"INVALID_SERVICE_AUTH","message":"bad"},"request_id":"req_fake"}`))
			return
		}
		if r.Method == http.MethodPost {
			f.createRequests = append(f.createRequests, r)
			var input map[string]any
			_ = json.Unmarshal(body, &input)
			status, data := f.nextCreate(input)
			if status != 200 {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"code":"DAILY_POST_CAP_REACHED","message":"daily Food post limit reached"},"request_id":"req_fake"}`))
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":%s,"request_id":"req_fake"}`, jsonOf(data))))
			return
		}
		f.readRequests = append(f.readRequests, r)
		_, _ = w.Write([]byte(`{"data":{"posts":[{"id":"11111111-1111-4111-8111-111111111111","title":"仁和食堂","campus":"minglun"}]},"request_id":"req_fake"}`))
	})
}

func validSignature(r *http.Request, body []byte, secret, clientID string) bool {
	if r.Header.Get("X-Service-Id") != clientID {
		return false
	}
	timestamp := r.Header.Get("X-Timestamp")
	nonce := r.Header.Get("X-Nonce")
	digest := sha256.Sum256(body)
	canonical := r.Method + "\n" + r.URL.RequestURI() + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(digest[:])
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hmac.Equal([]byte(r.Header.Get("X-Signature")), []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil))))
}

func jsonOf(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type harness struct {
	mcpServer *httptest.Server
	food      *fakeFood
}

func newHarness(t *testing.T, kitHandlers ...http.Handler) *harness {
	t.Helper()
	food := &fakeFood{
		nextCreate: func(input map[string]any) (int, map[string]any) {
			return 200, map[string]any{"post": map[string]any{"id": "22222222-2222-4222-8222-222222222222", "title": input["venue_name"], "hidden": false}}
		},
	}
	foodServer := httptest.NewServer(food.handler())
	t.Cleanup(foodServer.Close)
	client, err := foodclient.NewClient(foodServer.URL,
		testCreateClientID, testCreateSecret, "active",
		testReadClientID, testReadSecret, "active")
	if err != nil {
		t.Fatal(err)
	}
	options := mcp.Options{Client: client, AccessToken: "test-token"}
	if len(kitHandlers) > 0 {
		kit := httptest.NewServer(kitHandlers[0])
		t.Cleanup(kit.Close)
		options.KitSession, err = sessionclient.NewClient(kit.URL+"/api/v1/session", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	handler, err := mcp.NewHandler(options)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler.Handler())
	t.Cleanup(server.Close)
	return &harness{mcpServer: server, food: food}
}

func (h *harness) connect(t *testing.T) *mcpsdk.ClientSession {
	return h.connectWithHeaders(t, nil)
}

func (h *harness) connectWithHeaders(t *testing.T, headers http.Header) *mcpsdk.ClientSession {
	t.Helper()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client"}, nil)
	session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{
		Endpoint:             h.mcpServer.URL + "/mcp",
		DisableStandaloneSSE: true,
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{next: h.mcpServer.Client().Transport, headers: headers}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// bearerRoundTripper injects the MCP access token on every request so the
// SDK client (which has no plain-header option) can authenticate.
type bearerRoundTripper struct {
	next    http.RoundTripper
	headers http.Header
}

func (b bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer test-token")
	for key, values := range b.headers {
		request.Header[key] = values
	}
	return b.next.RoundTrip(request)
}

func callTool(t *testing.T, session *mcpsdk.ClientSession, name string, args map[string]any) (string, error) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, content := range result.Content {
		if item, ok := content.(*mcpsdk.TextContent); ok {
			text.WriteString(item.Text)
		}
	}
	if result.IsError {
		return text.String(), fmt.Errorf("%s", strings.TrimSpace(text.String()))
	}
	return text.String(), nil
}

func TestCreateFoodPostSignsActorAndReturnsSuccess(t *testing.T) {
	h := newHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/session" || r.Method != http.MethodGet {
			t.Errorf("Kit request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Cookie") != "__Host-henukit_portal_session=test-kit-session" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Forwarded-Proto") != "https" {
			t.Error("Kit resolver did not forward only the selected session cookie")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id": testActorID, "display_name": testActorName, "expires_at": time.Now().Add(time.Hour),
		})
	}))
	session := h.connectWithHeaders(t, http.Header{"Cookie": {"__Host-henukit_portal_session=test-kit-session; unrelated=private"}})

	text, err := callTool(t, session, "create_food_post", map[string]any{
		"venue_name": "仁和食堂", "campus": "minglun", "tier": "hang",
		"review_text": "胡辣汤很顶，早餐必去。",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "投稿已发布") || !strings.Contains(text, "仁和食堂") {
		t.Fatalf("create result = %q", text)
	}
	if strings.Contains(text, "当前未读取到") {
		t.Fatalf("bound account was treated as a guest: %q", text)
	}
	if len(h.food.createRequests) != 1 {
		t.Fatalf("create requests = %d", len(h.food.createRequests))
	}
	request := h.food.createRequests[0]
	if request.Header.Get("X-Service-Id") != testCreateClientID {
		t.Fatalf("create signed with %q", request.Header.Get("X-Service-Id"))
	}
	if request.Header.Get("X-Actor-User-Id") != testActorID || request.Header.Get("X-Actor-Display-Name") != testActorName {
		t.Fatalf("actor headers = %q / %q", request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Actor-Display-Name"))
	}
	if key := request.Header.Get("Idempotency-Key"); !strings.HasPrefix(key, "foodmcp:") || len(key) < 20 {
		t.Fatalf("idempotency key = %q", key)
	}
	if _, err := callTool(t, session, "list_my_food_posts", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if h.food.readRequests[0].Header.Get("X-Actor-User-Id") != testActorID {
		t.Fatal("mine did not automatically resolve the bound account")
	}
}

func TestCreateFoodPostWithoutAccountPublishesAndPromptsBinding(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)
	args := map[string]any{
		"venue_name": "仁和食堂", "campus": "minglun", "tier": "hang",
		"review_text": "胡辣汤很顶，早餐必去。",
	}
	for range 2 {
		text, err := callTool(t, session, "create_food_post", args)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "投稿已发布") || !strings.Contains(text, "绑定") {
			t.Fatalf("unbound result = %q", text)
		}
	}
	if len(h.food.createRequests) != 2 {
		t.Fatalf("create requests = %d", len(h.food.createRequests))
	}
	actor := h.food.createRequests[0].Header.Get("X-Actor-User-Id")
	if actor == "" || actor != h.food.createRequests[1].Header.Get("X-Actor-User-Id") {
		t.Fatalf("guest identity changed within session: %q", actor)
	}
	if _, err := callTool(t, session, "list_my_food_posts", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if h.food.readRequests[0].Header.Get("X-Actor-User-Id") != actor {
		t.Fatal("mine did not use the current guest identity")
	}
	other := h.connect(t)
	if _, err := callTool(t, other, "create_food_post", args); err != nil {
		t.Fatal(err)
	}
	if h.food.createRequests[2].Header.Get("X-Actor-User-Id") == actor {
		t.Fatal("different MCP sessions share a guest identity")
	}
}

func TestKitLookupFailureDoesNotBlockSubmission(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"expired login", http.StatusUnauthorized, `{}`},
		{"unavailable gateway", http.StatusServiceUnavailable, `{}`},
		{"invalid response", http.StatusOK, `{"user_id":"not-an-account"}`},
		{"expired snapshot", http.StatusOK, `{"user_id":"44444444-4444-4444-8444-444444444444","display_name":"旧账号","expires_at":"2000-01-01T00:00:00Z"}`},
		{"missing expiry", http.StatusOK, `{"user_id":"44444444-4444-4444-8444-444444444444","display_name":"旧账号"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			session := h.connectWithHeaders(t, http.Header{
				"Cookie":          {"__Host-henukit_portal_session=test-kit-session"},
				"X-Actor-User-Id": {testActorID}, "X-Actor-Display-Name": {testActorName},
			})
			text, err := callTool(t, session, "create_food_post", map[string]any{
				"venue_name": "仁和食堂", "campus": "minglun", "tier": "hang", "review_text": "胡辣汤很好喝。",
			})
			if err != nil || !strings.Contains(text, "投稿已发布") || !strings.Contains(text, "Kit 账号") {
				t.Fatalf("optional account lookup blocked submission: %q, %v", text, err)
			}
			request := h.food.createRequests[0]
			if request.Header.Get("X-Actor-User-Id") == testActorID || request.Header.Get("X-Actor-Display-Name") != "游客" {
				t.Fatal("invalid session or caller actor headers supplied account identity")
			}
		})
	}
}

func TestToolSchemasDoNotAskForAccountIdentity(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if tool.Name != "create_food_post" && tool.Name != "list_my_food_posts" {
			continue
		}
		schema := jsonOf(tool.InputSchema)
		if strings.Contains(schema, "actor_user_id") || strings.Contains(schema, "actor_display_name") {
			t.Fatalf("%s exposes manual account identity: %s", tool.Name, schema)
		}
	}
}

func TestCreateFoodPostRejectsInvalidInputBeforeUpstream(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)

	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"caller supplied actor", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "hang", "review_text": "好吃好吃。", "actor_user_id": testActorID, "actor_display_name": "名"}},
		{"bad tier", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "ssr", "review_text": "好吃好吃。"}},
		{"short review", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "hang", "review_text": "好"}},
		{"too many dishes", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "hang", "review_text": "好吃好吃。", "dishes": make([]any, 7)}},
		{"oversize price reference", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "hang", "review_text": "好吃好吃。", "price_reference": strings.Repeat("贵", 201)}},
		{"oversize hours reference", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "hang", "review_text": "好吃好吃。", "hours_reference": strings.Repeat("时", 201)}},
		{"oversize image", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "hang", "review_text": "好吃好吃。", "images": []any{map[string]any{"content_type": "image/png", "data": base64.StdEncoding.EncodeToString(make([]byte, 2<<20+1))}}}},
		{"bad image type", map[string]any{"venue_name": "店", "campus": "minglun", "tier": "hang", "review_text": "好吃好吃。", "images": []any{map[string]any{"content_type": "image/gif", "data": base64.StdEncoding.EncodeToString([]byte("x"))}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := callTool(t, session, "create_food_post", tc.args); err == nil {
				t.Fatal("invalid input accepted")
			}
			if len(h.food.createRequests) != 0 {
				t.Fatalf("invalid input reached upstream (%d calls)", len(h.food.createRequests))
			}
		})
	}
}

func TestCreateFoodPostSurfacesDailyCap(t *testing.T) {
	h := newHarness(t)
	h.food.nextCreate = func(input map[string]any) (int, map[string]any) {
		return 429, nil
	}
	session := h.connect(t)

	_, err := callTool(t, session, "create_food_post", map[string]any{
		"venue_name": "仁和食堂", "campus": "minglun", "tier": "hang",
		"review_text": "胡辣汤很顶，早餐必去。",
	})
	if err == nil || !strings.Contains(err.Error(), "DAILY_POST_CAP_REACHED") {
		t.Fatalf("daily cap error = %v", err)
	}
}

func TestReadToolsUseReadCredentialAndPassThrough(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)

	list, err := callTool(t, session, "list_food_posts", map[string]any{"campus": "minglun"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, "仁和食堂") {
		t.Fatalf("list result = %q", list)
	}
	if len(h.food.readRequests) != 1 || h.food.readRequests[0].Header.Get("X-Service-Id") != testReadClientID {
		t.Fatalf("list did not use read credential: %d requests", len(h.food.readRequests))
	}
	if h.food.readRequests[0].URL.RawQuery != "campus=minglun" {
		t.Fatalf("campus query = %q", h.food.readRequests[0].URL.RawQuery)
	}

	if _, err := callTool(t, session, "get_food_post", map[string]any{"post_id": "11111111-1111-4111-8111-111111111111"}); err != nil {
		t.Fatal(err)
	}
	if _, err := callTool(t, session, "list_food_venues", map[string]any{"campus": "jinming"}); err != nil {
		t.Fatal(err)
	}
	if _, err := callTool(t, session, "list_my_food_posts", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if len(h.food.readRequests) != 4 {
		t.Fatalf("read requests = %d, want 4", len(h.food.readRequests))
	}
	mine := h.food.readRequests[3]
	if mine.URL.Path != "/api/v1/food/posts/mine" || mine.Header.Get("X-Actor-User-Id") == "" {
		t.Fatalf("mine request = %s actor=%q", mine.URL.Path, mine.Header.Get("X-Actor-User-Id"))
	}
}

func TestVenuesRequiresCampusAndAuthRejectsBadToken(t *testing.T) {
	h := newHarness(t)
	session := h.connect(t)

	if _, err := callTool(t, session, "list_food_venues", map[string]any{}); err == nil {
		t.Fatal("venues without campus accepted")
	}

	response, err := http.Get(h.mcpServer.URL + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.StatusCode)
	}
}

func TestUnauthorizedCreateRejectsBeforeMCPHandshake(t *testing.T) {
	food := &fakeFood{nextCreate: func(input map[string]any) (int, map[string]any) { return 200, map[string]any{} }}
	foodServer := httptest.NewServer(food.handler())
	t.Cleanup(foodServer.Close)
	client, err := foodclient.NewClient(foodServer.URL, testCreateClientID, testCreateSecret, "active", testReadClientID, testReadSecret, "active")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := mcp.NewHandler(mcp.Options{Client: client, AccessToken: ""})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler.Handler())
	t.Cleanup(server.Close)
	response, err := http.Post(server.URL+"/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured server status = %d", response.StatusCode)
	}
}
