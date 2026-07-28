package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"henukit.dev/portal-gateway/internal/config"
)

func TestRouterKeepsQuizCraftCatalogDarkBeforeCutover(t *testing.T) {
	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		portalAPICalls.Add(1)
		if request.URL.Path != "/api/v1/practice/banks" {
			t.Fatalf("Portal API path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"banks":[],"request_id":"req_legacy_portal"}`))
	}))
	defer portalAPI.Close()

	var quizCraftCalls atomic.Int32
	quizCraft := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		quizCraftCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer quizCraft.Close()

	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL: portalAPI.URL,
		PracticeURL:  quizCraft.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	banks := httptest.NewRecorder()
	handler.Router().ServeHTTP(banks, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/banks", nil))
	if banks.Code != http.StatusOK || banks.Body.String() != `{"banks":[],"request_id":"req_legacy_portal"}` {
		t.Fatalf("dark bank response = %d %s", banks.Code, banks.Body.String())
	}

	stats := httptest.NewRecorder()
	handler.Router().ServeHTTP(stats, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/practice/stats", nil))
	if stats.Code != http.StatusServiceUnavailable || stats.Header().Get("Cache-Control") != "no-store" || stats.Body.String() == `{"banks":[],"request_id":"req_legacy_portal"}` {
		t.Fatalf("dark stats response = %d headers=%v body=%s", stats.Code, stats.Header(), stats.Body.String())
	}
	if portalAPICalls.Load() != 1 || quizCraftCalls.Load() != 0 {
		t.Fatalf("before #166 public route calls = portal-api:%d QuizCraft:%d", portalAPICalls.Load(), quizCraftCalls.Load())
	}
}
