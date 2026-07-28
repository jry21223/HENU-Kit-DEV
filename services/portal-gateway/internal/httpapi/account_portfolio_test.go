package httpapi

import (
	"crypto/tls"
	"encoding/json"
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
	request := httptest.NewRequest(http.MethodGet, "https://portal.test"+path, nil)
	request.TLS = &tls.ConnectionState{}
	request.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_session", Value: encoded})
	return request
}
