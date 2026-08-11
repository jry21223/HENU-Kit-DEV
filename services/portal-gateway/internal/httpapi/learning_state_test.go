package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"henukit.dev/portal-gateway/internal/contract"
	"henukit.dev/portal-gateway/internal/practice"
)

const learningStateUserID = "7d2d1f0e-0b4a-4f7e-9b3d-6a8c5f1e2a33"

func TestLearningStateReadsOnlyTheAuthorizedPortalSessionActor(t *testing.T) {
	var platformCalls atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		platformCalls.Add(1)
		if request.URL.Path != "/api/v1/authorization/check" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		if got := request.Header.Get("X-Request-Id"); got != "req_gateway_learning_state" {
			t.Fatalf("Platform Core X-Request-Id = %q, want inbound request id", got)
		}
		var payload struct {
			SessionExchangeToken string `json:"session_exchange_token"`
			PermissionCode       string `json:"permission_code"`
			Scope                struct {
				Kind        string `json:"kind"`
				ProductCode string `json:"product_code"`
			} `json:"scope"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.SessionExchangeToken != strings.Repeat("x", 32) || payload.PermissionCode != practice.PortalReadPermission || payload.Scope.Kind != "product" || payload.Scope.ProductCode != "quizcraft" {
			t.Fatalf("Platform authorization payload = %+v", payload)
		}
		writeProductPermissionDecision(t, writer, learningStateUserID, practice.PortalReadPermission)
	}))
	defer platform.Close()

	var coreCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		coreCalls.Add(1)
		if request.URL.Path != practice.GetPortalLearningStatePath || request.URL.RawQuery != "page=2&page_size=1&wrong=true" || request.Header.Get("X-Actor-User-Id") != learningStateUserID || request.Header.Get("X-Request-Id") != "req_gateway_learning_state" {
			t.Fatalf("QuizCraft Core request = %s?%s actor=%q request_id=%q", request.URL.Path, request.URL.RawQuery, request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Request-Id"))
		}
		assertActorBoundStatsSignature(t, request, learningStateUserID, "catalog-secret-with-enough-entropy")
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Request-Id", request.Header.Get("X-Request-Id"))
		_, _ = writer.Write([]byte(`{"request_id":"req_gateway_learning_state","data":{"items":[{"bank_id":"33333333-3333-4333-8333-333333333333","question_id":"55555555-5555-4555-8555-555555555555","question_version_id":"66666666-6666-4666-8666-666666666666","wrong":true,"attempt_count":3,"correct_count":1,"updated_at":"2026-08-06T08:00:00Z"}],"pagination":{"page":2,"page_size":1,"total":2,"total_pages":2}}}`))
	}))
	defer core.Close()

	var portalAPICalls atomic.Int32
	portalAPI := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { portalAPICalls.Add(1) }))
	defer portalAPI.Close()
	handler := newPersonalStatsHandler(t, platform.URL, core.URL, portalAPI.URL)

	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/learning-state?page=2&page_size=1&wrong=true", nil)
	request.Header.Set("X-Request-Id", "req_gateway_learning_state")
	request.AddCookie(sessionCookie(t, handler, learningStateUserID))
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("learning state = %d %s", response.Code, response.Body.String())
	}
	var envelope struct {
		RequestID string `json:"request_id"`
		Data      struct {
			Items      []contract.LearningStateItem `json:"items"`
			Pagination struct {
				Page       int64 `json:"page"`
				PageSize   int   `json:"page_size"`
				Total      int64 `json:"total"`
				TotalPages int64 `json:"total_pages"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.RequestID != "req_gateway_learning_state" || len(envelope.Data.Items) != 1 || !envelope.Data.Items[0].Wrong || envelope.Data.Items[0].AttemptCount != 3 || envelope.Data.Items[0].CorrectCount != 1 || envelope.Data.Pagination.Page != 2 || envelope.Data.Pagination.PageSize != 1 || envelope.Data.Pagination.Total != 2 || envelope.Data.Pagination.TotalPages != 2 {
		t.Fatalf("learning state envelope = %+v", envelope)
	}
	if platformCalls.Load() != 1 || coreCalls.Load() != 1 || portalAPICalls.Load() != 0 {
		t.Fatalf("learning state chain calls = platform:%d core:%d portal-api:%d", platformCalls.Load(), coreCalls.Load(), portalAPICalls.Load())
	}
}

func TestLearningStateRejectsSessionOrAuthorizationActorBeforeCore(t *testing.T) {
	tests := []struct {
		name          string
		withSession   bool
		platformState string
		wantStatus    int
	}{
		{name: "signed out", wantStatus: http.StatusUnauthorized},
		{name: "permission denied", withSession: true, platformState: "denied", wantStatus: http.StatusForbidden},
		{name: "authorization actor mismatch", withSession: true, platformState: "mismatch", wantStatus: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var platformCalls atomic.Int32
			platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				platformCalls.Add(1)
				switch test.platformState {
				case "denied":
					writer.WriteHeader(http.StatusForbidden)
				case "mismatch":
					writeProductPermissionDecision(t, writer, "99999999-9999-4999-8999-999999999999", practice.PortalReadPermission)
				default:
					writeProductPermissionDecision(t, writer, learningStateUserID, practice.PortalReadPermission)
				}
			}))
			defer platform.Close()
			var coreCalls atomic.Int32
			core := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { coreCalls.Add(1) }))
			defer core.Close()
			handler := newPersonalStatsHandler(t, platform.URL, core.URL, "http://127.0.0.1:9")
			request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/learning-state", nil)
			if test.withSession {
				request.AddCookie(sessionCookie(t, handler, learningStateUserID))
			}
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)
			if response.Code != test.wantStatus || coreCalls.Load() != 0 || strings.Contains(response.Body.String(), `"data"`) {
				t.Fatalf("learning state boundary = %d core:%d %s", response.Code, coreCalls.Load(), response.Body.String())
			}
			if !test.withSession && platformCalls.Load() != 0 {
				t.Fatalf("signed-out learning state contacted Platform Core %d times", platformCalls.Load())
			}
		})
	}
}

func TestLearningStateRejectsInvalidPaginationBeforeOwnerCalls(t *testing.T) {
	var platformCalls atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { platformCalls.Add(1) }))
	defer platform.Close()
	var coreCalls atomic.Int32
	core := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { coreCalls.Add(1) }))
	defer core.Close()
	handler := newPersonalStatsHandler(t, platform.URL, core.URL, "http://127.0.0.1:9")

	for _, query := range []string{
		"page=0",
		"page_size=101",
		"page=1&page=2",
		"unknown=1",
		"wrong=1",
		"wrong=true&wrong=false",
		"page=1;page_size=20",
		"page=9223372036854775807&page_size=100",
	} {
		request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/learning-state?"+query, nil)
		request.AddCookie(sessionCookie(t, handler, learningStateUserID))
		response := httptest.NewRecorder()
		handler.Router().ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), `"data"`) {
			t.Fatalf("learning state query %q = %d %s", query, response.Code, response.Body.String())
		}
	}
	if platformCalls.Load() != 0 || coreCalls.Load() != 0 {
		t.Fatalf("invalid pagination contacted Platform/Core = %d/%d", platformCalls.Load(), coreCalls.Load())
	}
}

func TestLearningStateKeepsInvalidAndUnavailableOwnerResponsesHonest(t *testing.T) {
	tests := []struct {
		name       string
		coreStatus int
		coreBody   string
		wantStatus int
	}{
		{name: "invalid owner envelope", coreStatus: http.StatusOK, coreBody: `{"request_id":"req_invalid","data":null}`, wantStatus: http.StatusBadGateway},
		{name: "owner unavailable", coreStatus: http.StatusServiceUnavailable, coreBody: `{"error":"private-upstream-detail"}`, wantStatus: http.StatusServiceUnavailable},
		{name: "owner service auth rejected", coreStatus: http.StatusUnauthorized, coreBody: `{"error":"private-auth-detail"}`, wantStatus: http.StatusServiceUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeProductPermissionDecision(t, writer, learningStateUserID, practice.PortalReadPermission)
			}))
			defer platform.Close()
			core := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.coreStatus)
				_, _ = writer.Write([]byte(test.coreBody))
			}))
			defer core.Close()
			handler := newPersonalStatsHandler(t, platform.URL, core.URL, "http://127.0.0.1:9")
			request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/learning-state", nil)
			request.AddCookie(sessionCookie(t, handler, learningStateUserID))
			response := httptest.NewRecorder()
			handler.Router().ServeHTTP(response, request)
			if response.Code != test.wantStatus || strings.Contains(response.Body.String(), "private-") || strings.Contains(response.Body.String(), `"data"`) {
				t.Fatalf("owner failure = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func writeProductPermissionDecision(t *testing.T, writer http.ResponseWriter, actorUserID, permissionCode string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(map[string]any{
		"request_id": "req_learning_state_authz",
		"data": map[string]any{
			"allowed": true, "actor_user_id": actorUserID, "permission_code": permissionCode,
			"scope":    map[string]string{"kind": "product", "product_code": "quizcraft"},
			"grant_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "authorization_revision": 1,
			"checked_at": "2026-08-09T00:00:00Z",
		},
	}); err != nil {
		t.Fatal(err)
	}
}
