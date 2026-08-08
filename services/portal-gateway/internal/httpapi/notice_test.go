package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/contract"
)

const noticeHTTPUserID = "33333333-3333-4333-8333-333333333333"

const validNoticeOwnerResponse = `{"data":{"items":[{"id":"22222222-2222-4222-8222-222222222222","source":{"id":"11111111-1111-4111-8111-111111111111","code":"registrar","name":"教务处"},"version":1,"title":"暑期安排","body":"暑期服务时间以本公告为准。","source_url":"https://example.edu/notices/summer","content_hash":"0000000000000000000000000000000000000000000000000000000000000000","state":"distributed","revision":1,"created_at":"2026-08-07T00:00:00Z","distribution_count":1,"distribution_status":"delivered"}],"generated_at":"2026-08-07T00:01:00Z"},"request_id":"req_notice_owner"}`

func TestNoticesReturnsOnlyContractValidOwnerSnapshot(t *testing.T) {
	handler, ownerCalls := newNoticeHandler(t, validNoticeOwnerResponse)
	response := getNotices(t, handler)

	if response.Code != http.StatusOK {
		t.Fatalf("notices = %d %s", response.Code, response.Body.String())
	}
	var envelope contract.NoticeFeedEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.RequestID != "req_gateway_notice" || len(envelope.Data.Items) != 1 || envelope.Data.Items[0].Title != "暑期安排" || envelope.Data.Items[0].Source.Name != "教务处" {
		t.Fatalf("notice envelope = %+v", envelope)
	}
	if ownerCalls.Load() != 1 {
		t.Fatalf("Notice owner calls = %d, want 1", ownerCalls.Load())
	}
}

func TestNoticesRejectsUnsafeOwnerSnapshotAsUnavailable(t *testing.T) {
	handler, ownerCalls := newNoticeHandler(t, `{"data":{"items":[{"id":"22222222-2222-4222-8222-222222222222","state":"distributed","body":"正文"}],"generated_at":"2026-08-07T00:01:00Z"},"request_id":"req_notice_owner"}`)
	response := getNotices(t, handler)

	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("unsafe notices = %d %s", response.Code, response.Body.String())
	}
	var envelope contract.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error != "notice temporarily unavailable" || envelope.RequestID != "req_gateway_notice" {
		t.Fatalf("unsafe notice error = %+v", envelope)
	}
	if ownerCalls.Load() != 1 {
		t.Fatalf("Notice owner calls = %d, want 1", ownerCalls.Load())
	}
}

func TestNoticesChecksSessionBeforeDependencyAvailability(t *testing.T) {
	handler, _ := newNoticeHandler(t, validNoticeOwnerResponse)
	handler.noticeClient = nil
	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/notices", nil)
	request.Header.Set("X-Request-Id", "req_gateway_notice")
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("signed-out notices = %d %s, want 401", response.Code, response.Body.String())
	}
}

func newNoticeHandler(t *testing.T, ownerResponse string) (*Handler, *atomic.Int32) {
	t.Helper()
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/authorization/check" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(platform.Close)

	var ownerCalls atomic.Int32
	owner := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ownerCalls.Add(1)
		if request.URL.Path != "/api/v1/console-notices" || request.Header.Get("X-Actor-User-Id") != noticeHTTPUserID || request.Header.Get("X-Permission-Code") != "notice.read" {
			t.Fatalf("Notice request = %s actor=%q permission=%q", request.URL.Path, request.Header.Get("X-Actor-User-Id"), request.Header.Get("X-Permission-Code"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(ownerResponse))
	}))
	t.Cleanup(owner.Close)

	handler, err := New(config.Config{
		SessionKey:        []byte("0123456789abcdef0123456789abcdef"),
		PlatformCoreURL:   platform.URL,
		PlatformClientID:  "portal-gateway",
		PlatformSecret:    "portal-client-secret-with-enough-entropy",
		PlatformKeyID:     "platform-key-1",
		PortalRedirectURI: "https://portal.test/api/v1/auth/callback",
		PortalOrigin:      "https://portal.test",
		PortalAPIURL:      "http://127.0.0.1:9",
		NoticeURL:         owner.URL,
		NoticeAuth: config.ServiceAuth{
			ClientID: "portal-gateway", ClientSecret: "notice-secret-with-enough-entropy", KeyID: "notice-key-1",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler, &ownerCalls
}

func getNotices(t *testing.T, handler *Handler) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/notices", nil)
	request.Header.Set("X-Request-Id", "req_gateway_notice")
	request.AddCookie(sessionCookie(t, handler, noticeHTTPUserID))
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	return response
}
