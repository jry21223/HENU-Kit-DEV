package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/session"
)

func TestCampusAuthenticatedRoutesBoundary(t *testing.T) {
	var seenActor, seenDisplay, seenSig string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenActor = r.Header.Get("X-Actor-User-Id")
		seenDisplay = r.Header.Get("X-Actor-Display-Name")
		seenSig = r.Header.Get("X-Signature")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"contact":{"qq":"123456","wechat":"wx_li","phone":"","note":""},"request_id":"req_test"}`))
	}))
	defer upstream.Close()

	sessionKey := []byte("0123456789abcdef0123456789abcdef")
	cfg := config.Config{
		PlatformCoreURL:        "http://127.0.0.1:8081",
		PlatformClientID:       "portal-gateway",
		PlatformSecret:         "secret-at-least-32-bytes-long-123456",
		PlatformKeyID:          "k1",
		PortalRedirectURI:      "http://localhost:8088/api/v1/auth/callback",
		SessionKey:             sessionKey,
		PortalAPIURL:           upstream.URL,
		PortalOrigin:           "http://localhost:8088",
		LocalOAuthCookieName:   "henukit_portal_oauth_local",
		LocalSessionCookieName: "henukit_portal_session_local",
	}

	// 1. Unconfigured campus credentials -> 503 when signed in, 401 when anonymous.
	hUnconf, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New unconfigured: %v", err)
	}
	routerUnconf := hUnconf.Router()
	rec := httptest.NewRecorder()
	routerUnconf.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/campus/contact-book", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous contact-book = %d, want 401", rec.Code)
	}

	codec, _ := session.NewCodec(sessionKey)
	cookieVal, _ := codec.Encode(session.Value{
		UserID:        "22222222-2222-4222-8222-222222222222",
		DisplayName:   "李明轩",
		ExchangeToken: "exchange-token-at-least-32-bytes-long-1234",
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	authCookie := &http.Cookie{Name: "henukit_portal_session_local", Value: cookieVal}

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8088/api/v1/campus/contact-book", nil)
	req.AddCookie(authCookie)
	rec = httptest.NewRecorder()
	routerUnconf.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured signed-in contact-book = %d, want 503", rec.Code)
	}

	// 2. Configured campus credentials -> signs request and overwrites any spoofed actor header.
	cfg.CampusPortalAuth = config.ServiceAuth{
		ClientID:     "portal-gateway-campus",
		KeyID:        "campus-portal-key",
		ClientSecret: "campus-portal-secret-at-least-32-bytes",
	}
	h, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New configured: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "http://localhost:8088/api/v1/campus/contact-book", nil)
	req.Header.Set("X-Actor-User-Id", "99999999-9999-4999-8999-999999999999")
	req.AddCookie(authCookie)
	rec = httptest.NewRecorder()
	h.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"qq":"123456"`) {
		t.Fatalf("configured contact-book = %d %s", rec.Code, rec.Body.String())
	}
	if seenActor != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("upstream actor = %q, want real session user (must ignore spoofed header)", seenActor)
	}
	if seenDisplay != "李明轩" || seenSig == "" {
		t.Fatalf("upstream display=%q sig_empty=%v", seenDisplay, seenSig == "")
	}
}
