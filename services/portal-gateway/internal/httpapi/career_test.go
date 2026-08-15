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

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/session"
)

const (
	careerSessionUserID     = "22222222-2222-4222-8222-222222222222"
	careerSessionFreeUserID = "33333333-3333-4333-8333-333333333333"
	careerSessionSecret     = "career-test-secret-at-least-32-bytes"
)

func newCareerHandler(t *testing.T, careerURL, membershipURL string) *Handler {
	t.Helper()
	cfg := config.Config{
		SessionKey: []byte("0123456789abcdef0123456789abcdef"),
		CareerURL:  careerURL,
		CareerAuth: config.ServiceAuth{
			ClientID: "portal-gateway-career", ClientSecret: careerSessionSecret, KeyID: "career-key",
		},
		PortalAPIURL: unreachablePortalAPIURL(),
	}
	if membershipURL != "" {
		cfg.AccountPortfolioURL = membershipURL
		cfg.AccountPortfolioAuth = config.ServiceAuth{
			ClientID: "portal-gateway-acct", ClientSecret: "account-test-secret-at-least-32-bytes", KeyID: "acct-key",
		}
	}
	handler, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func careerRequest(t *testing.T, handler *Handler, withSession bool, actorUserID, method, path, body, idempotencyKey string) *http.Request {
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
			UserID:        actorUserID,
			DisplayName:   "测试同学",
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

// membershipServer responds to /api/v1/account/membership with lifetime=true
// for lifetimeActor and lifetime=false otherwise. A nil handler body simulates
// an unavailable membership service.
func membershipServer(t *testing.T, lifetimeActor string, fail bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"DEP","message":"boom"},"request_id":"req_mem"}`))
			return
		}
		if r.URL.Path != "/api/v1/account/membership" {
			t.Fatalf("membership path = %q", r.URL.Path)
		}
		actor := r.Header.Get("X-Actor-User-Id")
		lifetime := actor == lifetimeActor
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":       map[string]any{"plan": map[bool]string{true: "lifetime", false: "free"}[lifetime], "lifetime": lifetime},
			"request_id": "req_mem",
		})
	}))
}

// careerUpstream responds to the Career routes with a canned search, asserting
// the actor header is bound from the session.
func careerUpstream(t *testing.T, expectedActor string, searchBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Actor-User-Id"); got != expectedActor {
			t.Fatalf("upstream X-Actor-User-Id = %q, want %q", got, expectedActor)
		}
		assertCareerSignature(t, r, expectedActor)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/career/searches":
			if got := r.Header.Get("Idempotency-Key"); got != "idem_career_create" {
				t.Fatalf("upstream Idempotency-Key = %q", got)
			}
			raw, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(raw), `"profile"`) {
				t.Fatalf("upstream create body lost profile: %s", raw)
			}
			_, _ = w.Write([]byte(`{"data":{"search":` + searchBody + `},"request_id":"req_career"}`))
		case "GET /api/v1/career/searches":
			_, _ = w.Write([]byte(`{"data":{"searches":[]},"request_id":"req_career"}`))
		case "GET /api/v1/career/searches/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa":
			_, _ = w.Write([]byte(`{"data":{"search":` + searchBody + `},"request_id":"req_career"}`))
		case "GET /api/v1/career/profile":
			_, _ = w.Write([]byte(`{"data":{"profile":{"user_id":"` + expectedActor + `","target_roles":"","email_notification_enabled":true}},"request_id":"req_career"}`))
		case "PUT /api/v1/career/profile":
			_, _ = w.Write([]byte(`{"data":{"profile":{"user_id":"` + expectedActor + `","target_roles":"后端"}},"request_id":"req_career"}`))
		default:
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
	}))
}

func assertCareerSignature(t *testing.T, request *http.Request, actor string) {
	t.Helper()
	if request.Header.Get("X-Service-Id") != "portal-gateway-career" || request.Header.Get("X-Key-Id") != "career-key" {
		t.Fatalf("career signature service headers missing")
	}
	if request.Header.Get("X-Signature") == "" {
		t.Fatalf("career signature missing")
	}
}

func TestCareerCreateSearchRequiresSessionAndLifetime(t *testing.T) {
	career := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("upstream contacted without auth gate")
	}))
	defer career.Close()
	mem := membershipServer(t, careerSessionUserID, false)
	defer mem.Close()

	handler := newCareerHandler(t, career.URL, mem.URL)

	// Anonymous -> 401
	anon := httptest.NewRecorder()
	handler.Router().ServeHTTP(anon, careerRequest(t, handler, false, careerSessionUserID, http.MethodPost, "/api/v1/career/searches", `{"profile":{"target_roles":"x"}}`, "idem_career_create"))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous create = %d, want 401", anon.Code)
	}

	// Free signed-in -> 403 lifetime_required
	free := httptest.NewRecorder()
	handler.Router().ServeHTTP(free, careerRequest(t, handler, true, careerSessionFreeUserID, http.MethodPost, "/api/v1/career/searches", `{"profile":{"target_roles":"x"}}`, "idem_career_create"))
	if free.Code != http.StatusForbidden {
		t.Fatalf("free create = %d, want 403: %s", free.Code, free.Body.String())
	}
	if !strings.Contains(free.Body.String(), "lifetime_required") {
		t.Fatalf("free create missing lifetime_required code: %s", free.Body.String())
	}
}

func TestCareerCreateSearchFailsClosedWhenMembershipUnavailable(t *testing.T) {
	career := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("upstream contacted when membership dependency failed")
	}))
	defer career.Close()
	mem := membershipServer(t, careerSessionUserID, true)
	defer mem.Close()

	handler := newCareerHandler(t, career.URL, mem.URL)
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, careerRequest(t, handler, true, careerSessionUserID, http.MethodPost, "/api/v1/career/searches", `{"profile":{"target_roles":"x"}}`, "idem_career_create"))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("membership-failure create = %d, want 503 (fail closed): %s", response.Code, response.Body.String())
	}
}

func TestCareerLifetimeCreateSearchBindsSessionActor(t *testing.T) {
	search := `{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","status":"queued","user_id":"` + careerSessionUserID + `","has_email":false,"created_at":"2026-08-15T00:00:00Z"}`
	upstream := careerUpstream(t, careerSessionUserID, search)
	defer upstream.Close()
	mem := membershipServer(t, careerSessionUserID, false)
	defer mem.Close()

	handler := newCareerHandler(t, upstream.URL, mem.URL)
	request := careerRequest(t, handler, true, careerSessionUserID, http.MethodPost, "/api/v1/career/searches", `{"profile":{"target_roles":"后端"}}`, "idem_career_create")
	request.Header.Set("X-Actor-User-Id", "99999999-9999-4999-8999-999999999999")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("lifetime create = %d: %s", response.Code, response.Body.String())
	}
	for _, want := range []string{`"search"`, `"request_id":"req_career"`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("create response omitted %s: %s", want, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("create response kept Career envelope: %s", response.Body.String())
	}
}

func TestCareerReadsBindActorAndForward(t *testing.T) {
	search := `{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","status":"completed","user_id":"` + careerSessionUserID + `","has_email":true,"created_at":"2026-08-15T00:00:00Z"}`
	upstream := careerUpstream(t, careerSessionUserID, search)
	defer upstream.Close()
	mem := membershipServer(t, careerSessionUserID, false)
	defer mem.Close()

	handler := newCareerHandler(t, upstream.URL, mem.URL)
	for _, path := range []string{
		"/api/v1/career/searches",
		"/api/v1/career/searches/aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"/api/v1/career/profile",
	} {
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, careerRequest(t, handler, true, careerSessionUserID, http.MethodGet, path, "", ""))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, response.Code, response.Body.String())
		}
	}
}

func TestCareerUnconfiguredFailsClosed(t *testing.T) {
	mem := membershipServer(t, careerSessionUserID, false)
	defer mem.Close()
	handler := newCareerHandler(t, "", mem.URL)
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, careerRequest(t, handler, true, careerSessionUserID, http.MethodGet, "/api/v1/career/searches", "", ""))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured career read = %d, want 503", response.Code)
	}
}
