package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/practice"
)

const learningStateUserID = "7d2d1f0e-0b4a-4f7e-9b3d-6a8c5f1e2a33"

func TestLearningStateReadsSignedInUserActorBoundFacts(t *testing.T) {
	var platformCalls atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/authorization/check" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		platformCalls.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer platform.Close()

	var coreCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != practice.GetPortalLearningStatePath || request.Header.Get("X-Actor-User-Id") != learningStateUserID || request.Header.Get("X-Request-Id") != "req_gateway_learning_state" {
			t.Fatalf("QuizCraft Core request = %s actor=%q request_id=%q", request.URL.Path, request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Request-Id"))
		}
		assertActorBoundStatsSignature(t, request, learningStateUserID, "catalog-secret-with-enough-entropy")
		coreCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", request.Header.Get("X-Request-Id"))
		_, _ = writer.Write([]byte(`{"request_id":"req_gateway_learning_state","data":[{"bank_id":"10ca9b18-c303-4b7a-ab14-1241e41b665a","question_id":"2f6d5a1e-7f3b-4c8e-9d0a-5e7b9c1d2e3f","question_version_id":"3a4b5c6d-1e2f-4a3b-8c7d-9e0f1a2b3c4d","wrong":true,"attempt_count":3,"correct_count":1,"updated_at":"2026-08-06T08:00:00.123Z"}]}`))
	}))
	defer core.Close()

	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { portalAPICalls.Add(1) }))
	defer portalAPI.Close()

	handler := newPersonalStatsHandler(t, platform.URL, core.URL, portalAPI.URL)
	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/learning-state", nil)
	request.Header.Set("X-Request-Id", "req_gateway_learning_state")
	request.AddCookie(sessionCookie(t, handler, learningStateUserID))
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("learning state = %d %s", response.Code, response.Body.String())
	}
	var envelope practice.LearningStateEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.RequestID != "req_gateway_learning_state" || len(envelope.Data) != 1 || envelope.Data[0].Wrong != true || envelope.Data[0].AttemptCount != 3 || envelope.Data[0].CorrectCount != 1 {
		t.Fatalf("learning state envelope = %+v", envelope)
	}
	if platformCalls.Load() != 1 || coreCalls.Load() != 1 || portalAPICalls.Load() != 0 {
		t.Fatalf("learning state chain calls = platform:%d core:%d portal-api:%d", platformCalls.Load(), coreCalls.Load(), portalAPICalls.Load())
	}
}

func TestLearningStateRejectsUnauthenticatedWithoutTouchingCore(t *testing.T) {
	var coreCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		coreCalls.Add(1)
		t.Fatalf("Core must not be reached for an unauthenticated learning-state read: %s", request.URL.Path)
	}))
	defer core.Close()

	handler := newPersonalStatsHandler(t, "http://127.0.0.1:9", core.URL, "http://127.0.0.1:9")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/learning-state", nil))

	if response.Code != http.StatusUnauthorized || coreCalls.Load() != 0 {
		t.Fatalf("unauthenticated learning state = %d core:%d %s", response.Code, coreCalls.Load(), response.Body.String())
	}
	if strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("unauthenticated learning state fabricated data: %s", response.Body.String())
	}
}

func TestLearningStateStaysDarkUntilTheV2ReadGate(t *testing.T) {
	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		portalAPICalls.Add(1)
		t.Fatalf("legacy Portal API must not serve learning state: %s", request.URL.Path)
	}))
	defer portalAPI.Close()
	var coreCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		coreCalls.Add(1)
		t.Fatalf("QuizCraft Core must not be reached while the V2 read gate is closed: %s", request.URL.Path)
	}))
	defer core.Close()

	handler, err := New(config.Config{
		SessionKey:   []byte("0123456789abcdef0123456789abcdef"),
		PortalAPIURL: portalAPI.URL,
		PracticeURL:  core.URL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/learning-state", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("dark learning state = %d %s", response.Code, response.Body.String())
	}
	if portalAPICalls.Load() != 0 || coreCalls.Load() != 0 {
		t.Fatalf("dark learning state touched upstreams = portal-api:%d core:%d", portalAPICalls.Load(), coreCalls.Load())
	}
	if strings.Contains(response.Body.String(), `"data"`) || !strings.Contains(response.Body.String(), "错题本暂时不可用") {
		t.Fatalf("dark learning state response is not an honest unavailable envelope: %s", response.Body.String())
	}
}
