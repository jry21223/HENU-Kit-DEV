package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/contract"
	portalnotice "henukit.dev/portal-gateway/internal/notice"
	"henukit.dev/portal-gateway/internal/session"
)

const (
	noticeActorID        = "11111111-1111-4111-8111-111111111111"
	noticePlatformSecret = "portal-platform-client-secret-at-least-32-bytes"
)

type noticeFeedStub func(context.Context, string, string) (contract.PortalNoticeFeed, error)

func (stub noticeFeedStub) List(ctx context.Context, actorUserID, requestID string) (contract.PortalNoticeFeed, error) {
	return stub(ctx, actorUserID, requestID)
}

func TestPortalNoticesChecksExactPortalScopeBeforeCallingOwner(t *testing.T) {
	var ownerCalls atomic.Int32
	owner := noticeFeedStub(func(_ context.Context, actorUserID, requestID string) (contract.PortalNoticeFeed, error) {
		ownerCalls.Add(1)
		if actorUserID != noticeActorID || requestID != "req_gateway_notices" {
			t.Fatalf("Notice Owner call = actor:%q request_id:%q", actorUserID, requestID)
		}
		return contract.PortalNoticeFeed{Notices: []contract.PortalNotice{{
			ID:        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			Title:     "开学安排",
			Body:      "请按时返校。",
			Source:    contract.PortalNoticeSource{Name: "学校办公室", URL: "https://example.edu/notices/1"},
			CreatedAt: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
		}}}, nil
	})

	var legacyCalls atomic.Int32
	legacy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		legacyCalls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer legacy.Close()

	var platformCalls atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		platformCalls.Add(1)
		if request.URL.Path != "/api/v1/authorization/check" {
			t.Fatalf("Platform request path = %q", request.URL.Path)
		}
		var body struct {
			SessionExchangeToken string `json:"session_exchange_token"`
			PermissionCode       string `json:"permission_code"`
			Scope                struct {
				Kind        string `json:"kind"`
				ProductCode string `json:"product_code"`
			} `json:"scope"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.SessionExchangeToken != strings.Repeat("x", 32) || body.PermissionCode != "portal.notice.read" || body.Scope.Kind != "product" || body.Scope.ProductCode != "portal" {
			t.Fatalf("Platform authorization body = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"allowed":true,"actor_user_id":"` + noticeActorID + `","permission_code":"portal.notice.read","scope":{"kind":"product","product_code":"portal"},"grant_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","authorization_revision":1,"checked_at":"2026-08-01T00:00:00Z"},"request_id":"req_platform"}`))
	}))
	defer platform.Close()

	handler := newNoticeHandler(t, platform.URL, owner, legacy.URL)
	response := getPortalNotices(t, handler, noticeSessionCookie(t, handler, noticeActorID))
	if response.Code != http.StatusOK {
		t.Fatalf("Portal notices = %d: %s", response.Code, response.Body.String())
	}
	if platformCalls.Load() != 1 || ownerCalls.Load() != 1 || legacyCalls.Load() != 0 {
		t.Fatalf("Portal notice chain = Platform:%d Owner:%d legacy:%d", platformCalls.Load(), ownerCalls.Load(), legacyCalls.Load())
	}
	var compatibility struct {
		Notices []struct {
			ID          string    `json:"id"`
			Title       string    `json:"title"`
			Source      string    `json:"source"`
			PublishedAt time.Time `json:"published_at"`
		} `json:"notices"`
		Data struct {
			Notices []struct {
				Title string `json:"title"`
			} `json:"notices"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &compatibility); err != nil {
		t.Fatal(err)
	}
	if compatibility.Notices == nil || len(compatibility.Notices) != 1 || compatibility.Notices[0].ID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || compatibility.Notices[0].Title != "开学安排" || compatibility.Notices[0].Source != "学校办公室" || compatibility.Notices[0].PublishedAt.IsZero() {
		t.Fatalf("legacy top-level notice facade = %#v", compatibility.Notices)
	}
	if len(compatibility.Data.Notices) != 1 || compatibility.Data.Notices[0].Title != "开学安排" {
		t.Fatalf("rich data notice feed = %#v", compatibility.Data)
	}
	if !strings.Contains(response.Body.String(), `"开学安排"`) || strings.Contains(response.Body.String(), "authorization_revision") {
		t.Fatalf("Portal notices response leaked or omitted fields: %s", response.Body.String())
	}
}

func TestPortalNoticesStopsBeforeOwnerWhenCoreDeniesOrSessionIsMissing(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var ownerCalls atomic.Int32
			owner := noticeFeedStub(func(context.Context, string, string) (contract.PortalNoticeFeed, error) {
				ownerCalls.Add(1)
				return contract.PortalNoticeFeed{}, nil
			})
			var legacyCalls atomic.Int32
			legacy := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { legacyCalls.Add(1) }))
			defer legacy.Close()
			var platformCalls atomic.Int32
			platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				platformCalls.Add(1)
				writer.WriteHeader(status)
			}))
			defer platform.Close()

			handler := newNoticeHandler(t, platform.URL, owner, legacy.URL)
			response := getPortalNotices(t, handler, noticeSessionCookie(t, handler, noticeActorID))
			if response.Code != status || platformCalls.Load() != 1 || ownerCalls.Load() != 0 || legacyCalls.Load() != 0 {
				t.Fatalf("Core %d Portal notices = %d Platform:%d Owner:%d legacy:%d: %s", status, response.Code, platformCalls.Load(), ownerCalls.Load(), legacyCalls.Load(), response.Body.String())
			}
		})
	}

	var platformCalls atomic.Int32
	var ownerCalls atomic.Int32
	platform := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { platformCalls.Add(1) }))
	defer platform.Close()
	owner := noticeFeedStub(func(context.Context, string, string) (contract.PortalNoticeFeed, error) {
		ownerCalls.Add(1)
		return contract.PortalNoticeFeed{}, nil
	})
	handler := newNoticeHandler(t, platform.URL, owner, "http://127.0.0.1:1")
	response := getPortalNotices(t, handler, nil)
	if response.Code != http.StatusUnauthorized || platformCalls.Load() != 0 || ownerCalls.Load() != 0 {
		t.Fatalf("missing session = %d Platform:%d Owner:%d: %s", response.Code, platformCalls.Load(), ownerCalls.Load(), response.Body.String())
	}
}

func TestPortalNoticesReturnsCoreDenialBeforeCheckingOwnerConfiguration(t *testing.T) {
	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer platform.Close()

	handler := newNoticeHandler(t, platform.URL, nil, "http://127.0.0.1:1")
	response := getPortalNotices(t, handler, noticeSessionCookie(t, handler, noticeActorID))
	if response.Code != http.StatusForbidden {
		t.Fatalf("Core denial with no Owner config = %d: %s", response.Code, response.Body.String())
	}
}

func TestPortalNoticesRejectsMalformedOwnerResponseWithoutLeakingIt(t *testing.T) {
	const ownerDetail = "owner-private-malformed-detail"
	owner := noticeFeedStub(func(context.Context, string, string) (contract.PortalNoticeFeed, error) {
		return contract.PortalNoticeFeed{Notices: []contract.PortalNotice{{Title: ownerDetail}}}, portalnotice.ErrInvalid
	})
	platform := permittedNoticePlatform(t, noticeActorID)
	defer platform.Close()
	handler := newNoticeHandler(t, platform.URL, owner, "http://127.0.0.1:1")
	response := getPortalNotices(t, handler, noticeSessionCookie(t, handler, noticeActorID))
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), ownerDetail) || strings.Contains(response.Body.String(), `"data"`) {
		t.Fatalf("malformed Owner = %d: %s", response.Code, response.Body.String())
	}
}

func TestPortalNoticesRejectsMissingOwnerNoticesArrayWithoutLeakingIt(t *testing.T) {
	owner := noticeFeedStub(func(context.Context, string, string) (contract.PortalNoticeFeed, error) {
		return contract.PortalNoticeFeed{}, portalnotice.ErrInvalid
	})
	platform := permittedNoticePlatform(t, noticeActorID)
	defer platform.Close()
	handler := newNoticeHandler(t, platform.URL, owner, "http://127.0.0.1:1")
	response := getPortalNotices(t, handler, noticeSessionCookie(t, handler, noticeActorID))
	if response.Code != http.StatusBadGateway || strings.Contains(response.Body.String(), `"data"`) || strings.Contains(response.Body.String(), `"notices":null`) {
		t.Fatalf("missing Owner notices array = %d: %s", response.Code, response.Body.String())
	}
}

func TestPortalNoticesPreservesAnEmptyOwnerArray(t *testing.T) {
	owner := noticeFeedStub(func(context.Context, string, string) (contract.PortalNoticeFeed, error) {
		return contract.PortalNoticeFeed{Notices: []contract.PortalNotice{}}, nil
	})
	platform := permittedNoticePlatform(t, noticeActorID)
	defer platform.Close()
	handler := newNoticeHandler(t, platform.URL, owner, "http://127.0.0.1:1")
	response := getPortalNotices(t, handler, noticeSessionCookie(t, handler, noticeActorID))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"notices":[]`) || strings.Contains(response.Body.String(), `"notices":null`) {
		t.Fatalf("empty Owner notices array = %d: %s", response.Code, response.Body.String())
	}
	var compatibility map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &compatibility); err != nil {
		t.Fatal(err)
	}
	if rootNotices, ok := compatibility["notices"]; !ok || string(rootNotices) != "[]" {
		t.Fatalf("legacy top-level notices = %q, want non-null empty array", rootNotices)
	}
	var data struct {
		Notices json.RawMessage `json:"notices"`
	}
	if err := json.Unmarshal(compatibility["data"], &data); err != nil || string(data.Notices) != "[]" {
		t.Fatalf("rich data notices = %q, %v; want non-null empty array", data.Notices, err)
	}
}

func TestPortalNoticesTreatsCoreRedirectAsUnavailableWithoutFollowingIt(t *testing.T) {
	var redirectTargetCalls atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectTargetCalls.Add(1)
	}))
	defer redirectTarget.Close()

	platform := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Redirect(writer, &http.Request{}, redirectTarget.URL, http.StatusFound)
	}))
	defer platform.Close()

	var ownerCalls atomic.Int32
	owner := noticeFeedStub(func(context.Context, string, string) (contract.PortalNoticeFeed, error) {
		ownerCalls.Add(1)
		return contract.PortalNoticeFeed{}, nil
	})

	handler := newNoticeHandler(t, platform.URL, owner, "http://127.0.0.1:1")
	response := getPortalNotices(t, handler, noticeSessionCookie(t, handler, noticeActorID))
	if response.Code != http.StatusServiceUnavailable || redirectTargetCalls.Load() != 0 || ownerCalls.Load() != 0 {
		t.Fatalf("Core redirect = %d target:%d owner:%d: %s", response.Code, redirectTargetCalls.Load(), ownerCalls.Load(), response.Body.String())
	}
}

func newNoticeHandler(t *testing.T, platformURL string, noticeFeed portalNoticeFeedClient, legacyURL string) *Handler {
	t.Helper()
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	handler, err := New(config.Config{
		SessionKey:             []byte("0123456789abcdef0123456789abcdef"),
		PlatformCoreURL:        platformURL,
		PlatformClientID:       "portal-gateway",
		PlatformSecret:         noticePlatformSecret,
		PlatformKeyID:          "platform-key",
		PortalAPIURL:           legacyURL,
		LocalOAuthCookieName:   "portal_oauth_local",
		LocalSessionCookieName: "portal_session_local",
	}, redisClient)
	if err != nil {
		t.Fatal(err)
	}
	handler.noticeFeed = noticeFeed
	return handler
}

func noticeSessionCookie(t *testing.T, handler *Handler, actor string) *http.Cookie {
	t.Helper()
	encoded, err := handler.sessionCodec.Encode(session.Value{UserID: actor, DisplayName: "通知同学", ExchangeToken: strings.Repeat("x", 32), ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: "portal_session_local", Value: encoded}
}

func getPortalNotices(t *testing.T, handler *Handler, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/notices", nil)
	request.Header.Set("X-Request-Id", "req_gateway_notices")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.Router().ServeHTTP(response, request)
	return response
}

func permittedNoticePlatform(t *testing.T, actor string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"allowed":true,"actor_user_id":"` + actor + `","permission_code":"portal.notice.read","scope":{"kind":"product","product_code":"portal"},"grant_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","authorization_revision":1,"checked_at":"2026-08-01T00:00:00Z"},"request_id":"req_platform"}`))
	}))
}
