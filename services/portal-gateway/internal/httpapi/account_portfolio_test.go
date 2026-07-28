package httpapi

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/session"
)

const accountPortfolioSecret = "account-portfolio-gateway-secret-at-least-32-bytes"

func TestAccountSummaryUsesPortalSessionAndReturnsRealOwnerData(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/account/summary" || request.Header.Get("X-Actor-User-Id") != "11111111-1111-4111-8111-111111111111" {
			t.Fatalf("Account Portfolio request = %s actor=%q", request.URL.Path, request.Header.Get("X-Actor-User-Id"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"points_balance":            0,
				"plan":                      "free",
				"lifetime":                  false,
				"unread_notification_count": 0,
				"open_ticket_count":         0,
			},
			"request_id": "req_account_owner",
		})
	}))
	defer owner.Close()

	handler := newAccountPortfolioHandler(t, owner.URL)
	request := authenticatedAccountRequest(t, handler, "/api/v1/account/summary")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("summary status = %d: %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("summary Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
	for _, want := range []string{`"points_balance":0`, `"plan":"free"`, `"lifetime":false`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("summary omitted %s: %s", want, response.Body.String())
		}
	}
}

func TestAccountSummaryDoesNotFallbackWhenOwnerIsUnavailable(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":{"code":"DEPENDENCY_UNAVAILABLE"},"request_id":"req_owner"}`))
	}))
	defer owner.Close()

	handler := newAccountPortfolioHandler(t, owner.URL)
	request := authenticatedAccountRequest(t, handler, "/api/v1/account/summary")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("summary status = %d, want 503: %s", response.Code, response.Body.String())
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "mock") || strings.Contains(response.Body.String(), `"points_balance":0`) {
		t.Fatalf("summary substituted a success fallback: %s", response.Body.String())
	}
}

func TestAccountSummaryRejectsInvalidOwnerResponseInsteadOfShowingAccountData(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data":       []string{"not an account object"},
			"request_id": "req_invalid_owner",
		})
	}))
	defer owner.Close()

	handler := newAccountPortfolioHandler(t, owner.URL)
	request := authenticatedAccountRequest(t, handler, "/api/v1/account/summary")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("summary status = %d, want 502: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "not an account object") || strings.Contains(response.Body.String(), `"points_balance":0`) {
		t.Fatalf("summary returned invalid or fallback account data: %s", response.Body.String())
	}
}

func TestAccountSummaryRejectsMissingPortalSession(t *testing.T) {
	handler := newAccountPortfolioHandler(t, "http://127.0.0.1:1")
	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/account/summary", nil)
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("summary without Portal Session status = %d, want 401", response.Code)
	}
}

func TestEveryAccountReadRouteUsesTheAuthenticatedOwnerBoundary(t *testing.T) {
	paths := []string{
		"/api/v1/account/summary",
		"/api/v1/account/points",
		"/api/v1/account/membership",
		"/api/v1/account/notifications",
		"/api/v1/account/tickets",
		"/api/v1/account/membership-orders",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != path {
					t.Fatalf("owner path = %s, want %s", request.URL.Path, path)
				}
				if request.Header.Get("X-Actor-User-Id") != "11111111-1111-4111-8111-111111111111" {
					t.Fatalf("owner actor = %q", request.Header.Get("X-Actor-User-Id"))
				}
				writer.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"data":       validAccountOwnerData(path),
					"request_id": "req_account_owner",
				})
			}))
			defer owner.Close()

			handler := newAccountPortfolioHandler(t, owner.URL)
			request := authenticatedAccountRequest(t, handler, path)
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data"`) {
				t.Fatalf("%s response = %d: %s", path, response.Code, response.Body.String())
			}
		})
	}
}

func TestAccountTicketAndNotificationCommandsUsePortalSessionActor(t *testing.T) {
	const ticketID = "22222222-2222-4222-8222-222222222222"
	const notificationID = "33333333-3333-4333-8333-333333333333"
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Actor-User-Id") != "11111111-1111-4111-8111-111111111111" {
			t.Fatalf("owner actor = %q, want Portal Session actor", request.Header.Get("X-Actor-User-Id"))
		}
		if request.Method == http.MethodPost && request.Header.Get("Idempotency-Key") == "" {
			t.Fatal("owner command omitted Idempotency-Key")
		}
		var data map[string]any
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch request.URL.Path {
		case "/api/v1/account/tickets":
			if request.Method != http.MethodPost {
				t.Fatalf("ticket create method = %s", request.Method)
			}
			if string(body) != `{"title":"Need help","category":"account","body":"Initial message."}` {
				t.Fatalf("ticket create body = %q", body)
			}
			data = map[string]any{"ticket": accountTicketFixture(ticketID)}
			writer.WriteHeader(http.StatusCreated)
		case "/api/v1/account/tickets/" + ticketID:
			if request.Method != http.MethodGet {
				t.Fatalf("ticket detail method = %s", request.Method)
			}
			data = map[string]any{
				"ticket":   accountTicketFixture(ticketID),
				"messages": []any{map[string]any{"id": "44444444-4444-4444-8444-444444444444", "author_kind": "user", "body": "Initial message.", "created_at": "2026-07-28T00:00:00Z"}},
				"events":   []any{},
			}
		case "/api/v1/account/tickets/" + ticketID + "/follow-ups":
			if request.Method != http.MethodPost {
				t.Fatalf("ticket follow-up method = %s", request.Method)
			}
			if string(body) != `{"body":"Please update me.","expected_version":1}` {
				t.Fatalf("ticket follow-up body = %q", body)
			}
			data = map[string]any{"ticket": accountTicketFixture(ticketID)}
		case "/api/v1/account/notifications/" + notificationID + "/read":
			if request.Method != http.MethodPost {
				t.Fatalf("notification read method = %s", request.Method)
			}
			if len(body) != 0 {
				t.Fatalf("notification read body = %q, want empty", body)
			}
			data = map[string]any{"notification": accountNotificationFixture(notificationID)}
		default:
			t.Fatalf("unexpected owner route %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": data, "request_id": "req_owner"})
	}))
	defer owner.Close()

	handler := newAccountPortfolioHandler(t, owner.URL)
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		key    string
		status int
	}{
		{name: "create", method: http.MethodPost, path: "/api/v1/account/tickets", body: `{"title":"Need help","category":"account","body":"Initial message."}`, key: "idem_gateway_create", status: http.StatusCreated},
		{name: "detail", method: http.MethodGet, path: "/api/v1/account/tickets/" + ticketID, status: http.StatusOK},
		{name: "follow-up", method: http.MethodPost, path: "/api/v1/account/tickets/" + ticketID + "/follow-ups", body: `{"body":"Please update me.","expected_version":1}`, key: "idem_gateway_follow_up", status: http.StatusOK},
		{name: "notification-read", method: http.MethodPost, path: "/api/v1/account/notifications/" + notificationID + "/read", key: "idem_gateway_notification", status: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := authenticatedAccountRequestWithMethod(t, handler, test.method, test.path, test.body, test.key)
			request.Header.Set("X-Actor-User-Id", "99999999-9999-4999-8999-999999999999")
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("%s status = %d, want %d: %s", test.name, response.Code, test.status, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"data"`) {
				t.Fatalf("%s response headers/body = %#v %s", test.name, response.Header(), response.Body.String())
			}
		})
	}
}

func TestAccountTicketCommandRejectsInvalidIdempotencyBeforeContactingOwner(t *testing.T) {
	called := false
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		called = true
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer owner.Close()

	handler := newAccountPortfolioHandler(t, owner.URL)
	request := authenticatedAccountRequestWithMethod(t, handler, http.MethodPost, "/api/v1/account/tickets", `{"title":"Need help","category":"account","body":"Initial message."}`, "short")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || called {
		t.Fatalf("invalid idempotency status/called = %d/%t, want 400/false: %s", response.Code, called, response.Body.String())
	}
}

func TestAccountTicketCommandMapsOwnerConflictWithoutMockSuccess(t *testing.T) {
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusConflict)
		_, _ = writer.Write([]byte(`{"error":{"code":"VERSION_CONFLICT"},"request_id":"req_owner"}`))
	}))
	defer owner.Close()

	handler := newAccountPortfolioHandler(t, owner.URL)
	request := authenticatedAccountRequestWithMethod(t, handler, http.MethodPost, "/api/v1/account/tickets", `{"title":"Need help","category":"account","body":"Initial message."}`, "idem_gateway_conflict")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("conflicting ticket command status = %d, want 409: %s", response.Code, response.Body.String())
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "mock") || strings.Contains(response.Body.String(), `"ticket"`) {
		t.Fatalf("conflicting ticket command returned success fallback: %s", response.Body.String())
	}
}

func validAccountOwnerData(path string) map[string]any {
	switch path {
	case "/api/v1/account/summary":
		return map[string]any{
			"points_balance":            0,
			"plan":                      "free",
			"lifetime":                  false,
			"unread_notification_count": 0,
			"open_ticket_count":         0,
		}
	case "/api/v1/account/points":
		return map[string]any{"balance": 0, "entries": []any{}}
	case "/api/v1/account/membership":
		return map[string]any{"plan": "free", "lifetime": false}
	case "/api/v1/account/notifications":
		return map[string]any{"notifications": []any{}}
	case "/api/v1/account/tickets":
		return map[string]any{"tickets": []any{}}
	case "/api/v1/account/membership-orders":
		return map[string]any{"orders": []any{}}
	default:
		panic("unknown Account Portfolio route " + path)
	}
}

func accountTicketFixture(id string) map[string]any {
	return map[string]any{
		"id":         id,
		"reference":  "HKT-" + id,
		"title":      "Need help",
		"category":   "account",
		"status":     "open",
		"version":    1,
		"created_at": "2026-07-28T00:00:00Z",
		"updated_at": "2026-07-28T00:00:00Z",
	}
}

func accountNotificationFixture(id string) map[string]any {
	return map[string]any{
		"id":         id,
		"title":      "客服工单状态已更新",
		"body":       "工单状态已更新。",
		"kind":       "ticket_status",
		"created_at": "2026-07-28T00:00:00Z",
	}
}

func newAccountPortfolioHandler(t *testing.T, ownerURL string) *Handler {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	handler, err := New(config.Config{
		SessionKey:             []byte("0123456789abcdef0123456789abcdef"),
		AccountPortfolioURL:    ownerURL,
		AccountPortfolioAuth:   config.ServiceAuth{ClientID: "portal-gateway", ClientSecret: accountPortfolioSecret, KeyID: "account-key"},
		LocalOAuthCookieName:   "portal_oauth_local",
		LocalSessionCookieName: "portal_session_local",
	}, redisClient)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func authenticatedAccountRequest(t *testing.T, handler *Handler, path string) *http.Request {
	return authenticatedAccountRequestWithMethod(t, handler, http.MethodGet, path, "", "")
}

func authenticatedAccountRequestWithMethod(t *testing.T, handler *Handler, method, path, body, idempotencyKey string) *http.Request {
	t.Helper()
	encoded, err := handler.sessionCodec.Encode(session.Value{
		UserID:        "11111111-1111-4111-8111-111111111111",
		DisplayName:   "小河同学",
		ExchangeToken: strings.Repeat("x", 32),
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, "https://portal.test"+path, strings.NewReader(body))
	request.TLS = &tls.ConnectionState{}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_session", Value: encoded})
	return request
}
