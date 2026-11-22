package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

type fakeCareerClient struct {
	workspace json.RawMessage
	retryResp json.RawMessage
}

func (f *fakeCareerClient) Workspace(_ context.Context, _ string) (json.RawMessage, error) {
	return f.workspace, nil
}
func (f *fakeCareerClient) RetryDigest(_ context.Context, _, _, _ string) (json.RawMessage, error) {
	return f.retryResp, nil
}

type fakeCampusClient struct {
	workspace json.RawMessage
	itemResp  json.RawMessage
	msgResp   json.RawMessage
}

func (f *fakeCampusClient) Workspace(_ context.Context, _ string) (json.RawMessage, error) {
	return f.workspace, nil
}
func (f *fakeCampusClient) ModerateItem(_ context.Context, _, _ string, _ []byte) (json.RawMessage, error) {
	return f.itemResp, nil
}
func (f *fakeCampusClient) ModerateMessage(_ context.Context, _, _ string, _ []byte) (json.RawMessage, error) {
	return f.msgResp, nil
}

type careerCampusPlatform struct {
	*fakePlatform
	careerErr error
	campusErr error
}

func (p *careerCampusPlatform) CheckCareer(_ context.Context, _, _ string) error { return p.careerErr }
func (p *careerCampusPlatform) CheckCampus(_ context.Context, _, _ string) error { return p.campusErr }

func TestCareerAndCampusRoutesAndSessionDiscovery(t *testing.T) {
	codec, err := session.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	basePlatform := &fakePlatform{
		exchange: platformcore.Exchange{
			UserID:        "11111111-1111-4111-8111-111111111111",
			ExchangeToken: "exchange-token-at-least-32-bytes-long-1234",
			ExpiresAt:     time.Now().Add(time.Hour),
		},
	}
	platform := &careerCampusPlatform{fakePlatform: basePlatform}
	careerFake := &fakeCareerClient{
		workspace: json.RawMessage(`{"job_source_enabled":false,"ai_mode":"mock","counts":{"profiles":2},"searches":[],"extractions":[],"digest_deliveries":[]}`),
		retryResp: json.RawMessage(`{"search_id":"22222222-2222-4222-8222-222222222222","status":"retry"}`),
	}
	campusFake := &fakeCampusClient{
		workspace: json.RawMessage(`{"counts":{"open":3},"items":[],"messages":[]}`),
		itemResp:  json.RawMessage(`{"id":"h-01","status":"hidden","version":2,"moderation_note":"违规"}`),
		msgResp:   json.RawMessage(`{"id":"m1","hidden":true}`),
	}
	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	t.Cleanup(func() { _ = redisClient.Close() })
	h, err := New(
		"https://account.henukit.cn", "console-gateway", "https://console.henukit.cn/api/v1/auth/callback",
		platform, nil, fakeOverview{}, redisClient, codec, nil,
		nil, nil, nil, nil, careerFake, campusFake,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cookieVal, err := codec.Encode(session.Value{
		UserID:        "11111111-1111-4111-8111-111111111111",
		ExchangeToken: "exchange-token-at-least-32-bytes-long-1234",
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: cookieVal}

	// 1. Unauthenticated -> 401
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/career/workspace", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth career workspace = %d, want 401", rec.Code)
	}

	// 2. Session advertises career & campus permissions
	sessReq := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	sessReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, sessReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "career.read") || !strings.Contains(rec.Body.String(), "campus.moderate") {
		t.Fatalf("session missing career/campus permissions: status=%d body=%s", rec.Code, rec.Body.String())
	}

	// 3. Career workspace + retry digest
	req := httptest.NewRequest(http.MethodGet, "/api/v1/career/workspace", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"profiles":2`) {
		t.Fatalf("career workspace = %d %s", rec.Code, rec.Body.String())
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/api/v1/career/searches/22222222-2222-4222-8222-222222222222/retry-digest", nil)
	retryReq.Host = "localhost:8089"
	retryReq.Header.Set("Origin", "http://localhost:8089")
	retryReq.Header.Set("Idempotency-Key", "idem_career_retry_123456")
	retryReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, retryReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"retry"`) {
		t.Fatalf("career retry digest = %d %s", rec.Code, rec.Body.String())
	}

	// 4. Campus workspace + item/message moderation
	cReq := httptest.NewRequest(http.MethodGet, "/api/v1/campus/workspace", nil)
	cReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, cReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"open":3`) {
		t.Fatalf("campus workspace = %d %s", rec.Code, rec.Body.String())
	}

	modReq := httptest.NewRequest(http.MethodPost, "/api/v1/campus/items/h-01/moderate", strings.NewReader(`{"expected_version":1,"action":"hide","note":"违规"}`))
	modReq.Host = "localhost:8089"
	modReq.Header.Set("Origin", "http://localhost:8089")
	modReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, modReq)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"hidden"`) {
		t.Fatalf("campus moderate item = %d %s", rec.Code, rec.Body.String())
	}

	// 5. Forbidden when lacking campus permission
	platform.campusErr = platformcore.ErrForbidden
	forbidReq := httptest.NewRequest(http.MethodGet, "/api/v1/campus/workspace", nil)
	forbidReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, forbidReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("forbidden campus workspace = %d, want 403", rec.Code)
	}
}
