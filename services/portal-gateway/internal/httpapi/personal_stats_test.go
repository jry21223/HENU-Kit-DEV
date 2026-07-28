package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/practice"
	"henukit.dev/portal-gateway/internal/session"
)

const personalStatsUserID = "5f03dac8-7f7f-4513-9dcd-e4cc5f592c85"

func TestPersonalPracticeStatsUsesSignedInUserAndNeverFallsBackToPortalAPI(t *testing.T) {
	var platformCalls atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/authorization/check" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		platformCalls.Add(1)
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["permission_code"] != practice.CatalogReadPermission || payload["session_exchange_token"] != strings.Repeat("x", 32) {
			t.Fatalf("Platform authorization payload = %#v", payload)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer platform.Close()

	var coreCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != practice.GetPersonalPracticeStatsPath || request.Header.Get("X-Actor-User-Id") != personalStatsUserID || request.Header.Get("X-Request-Id") != "req_gateway_stats" {
			t.Fatalf("QuizCraft Core request = %s actor=%q request_id=%q", request.URL.Path, request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Request-Id"))
		}
		if request.Header.Get("X-Permission-Code") != practice.CatalogReadPermission || request.Header.Get("X-Service-Id") != "portal-gateway" || request.Header.Get("X-Signature") == "" {
			t.Fatalf("QuizCraft Core request omitted signed read credentials")
		}
		coreCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"request_id":"req_core_stats","data":{"total_answers":4,"correct_answers":3,"accuracy":75,"streak_days":2,"mastery":[{"bank_id":"10ca9b18-c303-4b7a-ab14-1241e41b665a","label":"计算机基础","value":50,"total_questions":4,"correct_questions":2}]}}`))
	}))
	defer core.Close()

	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { portalAPICalls.Add(1) }))
	defer portalAPI.Close()

	handler := newPersonalStatsHandler(t, platform.URL, core.URL, portalAPI.URL)
	first := getPersonalStats(t, handler, sessionCookie(t, handler, personalStatsUserID))
	second := getPersonalStats(t, handler, sessionCookie(t, handler, personalStatsUserID))
	for index, response := range []*httptest.ResponseRecorder{first, second} {
		if response.Code != http.StatusOK {
			t.Fatalf("device %d stats = %d %s", index, response.Code, response.Body.String())
		}
		var stats practice.PersonalPracticeStatsEnvelope
		if err := json.Unmarshal(response.Body.Bytes(), &stats); err != nil {
			t.Fatal(err)
		}
		if stats.RequestID != "req_core_stats" || stats.Data.TotalAnswers != 4 || stats.Data.CorrectAnswers != 3 || stats.Data.Accuracy != 75 || stats.Data.StreakDays != 2 || len(stats.Data.Mastery) != 1 {
			t.Fatalf("device %d stats = %+v", index, stats)
		}
	}
	if platformCalls.Load() != 2 || coreCalls.Load() != 2 || portalAPICalls.Load() != 0 {
		t.Fatalf("stats chain calls = platform:%d core:%d portal-api:%d", platformCalls.Load(), coreCalls.Load(), portalAPICalls.Load())
	}
}

func TestPersonalPracticeStatsFailsHonestlyForNoSessionOrCoreFailure(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/authorization/check" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer platform.Close()

	const upstreamDetail = "core-failure-must-not-reach-browser"
	var coreCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		coreCalls.Add(1)
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"error":"` + upstreamDetail + `"}`))
	}))
	defer core.Close()

	handler := newPersonalStatsHandler(t, platform.URL, core.URL, "http://127.0.0.1:9")

	unauthenticated := httptest.NewRecorder()
	handler.Router().ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/stats", nil))
	if unauthenticated.Code != http.StatusUnauthorized || coreCalls.Load() != 0 {
		t.Fatalf("unauthenticated stats = %d %s core:%d", unauthenticated.Code, unauthenticated.Body.String(), coreCalls.Load())
	}

	failure := getPersonalStats(t, handler, sessionCookie(t, handler, personalStatsUserID))
	if failure.Code != http.StatusServiceUnavailable || strings.Contains(failure.Body.String(), upstreamDetail) || strings.Contains(failure.Body.String(), "catalog-secret") || strings.Contains(failure.Body.String(), `"data"`) {
		t.Fatalf("Core failure response = %d %s", failure.Code, failure.Body.String())
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(failure.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.RequestID != "req_gateway_stats" || envelope.Error != "practice statistics are temporarily unavailable" {
		t.Fatalf("Core failure envelope = %+v", envelope)
	}
}

func newPersonalStatsHandler(t *testing.T, platformURL, coreURL, portalAPIURL string) *Handler {
	t.Helper()
	handler, err := New(config.Config{
		SessionKey:              []byte("0123456789abcdef0123456789abcdef"),
		PlatformCoreURL:         platformURL,
		PlatformClientID:        "portal-gateway",
		PlatformSecret:          "portal-client-secret-with-enough-entropy",
		PlatformKeyID:           "platform-key-1",
		PortalRedirectURI:       "https://portal.test/api/v1/auth/callback",
		PortalOrigin:            "https://portal.test",
		PortalAPIURL:            portalAPIURL,
		QuizCraftV2ReadsEnabled: true,
		QuizCraftCoreURL:        coreURL,
		QuizCraftCoreAuth: config.ServiceAuth{
			ClientID: "portal-gateway", ClientSecret: "catalog-secret-with-enough-entropy", KeyID: "portal-catalog-key-1",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func sessionCookie(t *testing.T, handler *Handler, userID string) *http.Cookie {
	t.Helper()
	encoded, err := handler.sessionCodec.Encode(session.Value{
		UserID: userID, DisplayName: "跨设备测试用户", ExchangeToken: strings.Repeat("x", 32), ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "henukit_portal_session_local", Value: encoded}
}

func getPersonalStats(t *testing.T, handler *Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/stats", nil)
	request.Header.Set("X-Request-Id", "req_gateway_stats")
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(recorder, request)
	return recorder
}
