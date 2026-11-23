package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
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

	cfg := campusGatewayTestConfig(upstream.URL)

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

	authCookie := campusGatewayTestCookie(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8088/api/v1/campus/contact-book", nil)
	req.AddCookie(authCookie)
	rec = httptest.NewRecorder()
	routerUnconf.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "CAMPUS_WRITES_UNAVAILABLE") {
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

func campusGatewayTestConfig(upstreamURL string) config.Config {
	sessionKey := []byte("0123456789abcdef0123456789abcdef")
	return config.Config{
		PlatformCoreURL:        "http://127.0.0.1:8081",
		PlatformClientID:       "portal-gateway",
		PlatformSecret:         "secret-at-least-32-bytes-long-123456",
		PlatformKeyID:          "k1",
		PortalRedirectURI:      "http://localhost:8088/api/v1/auth/callback",
		SessionKey:             sessionKey,
		PortalAPIURL:           upstreamURL,
		PortalOrigin:           "http://localhost:8088",
		LocalOAuthCookieName:   "henukit_portal_oauth_local",
		LocalSessionCookieName: "henukit_portal_session_local",
	}

}

func campusGatewayTestCookie(t *testing.T, cfg config.Config) *http.Cookie {
	t.Helper()
	codec, err := session.NewCodec(cfg.SessionKey)
	if err != nil {
		t.Fatal(err)
	}
	cookieVal, err := codec.Encode(session.Value{
		UserID:        "22222222-2222-4222-8222-222222222222",
		DisplayName:   "李明轩",
		ExchangeToken: "exchange-token-at-least-32-bytes-long-1234",
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: cfg.LocalSessionCookieName, Value: cookieVal}

}

func TestCampusUploadProxySignsEntireRequestAndKeepsRouteLimits(t *testing.T) {
	const secret = "campus-portal-secret-at-least-32-bytes"
	const itemsPath = "/api/v1/campus/items"
	wantBody := `{"images":[{"content_type":"image/png","data":"` + strings.Repeat("A", 1<<20) + `"}]}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != wantBody || r.Method != http.MethodPost || r.URL.Path != itemsPath {
			t.Errorf("upstream request = %s %s, %d bytes, err = %v", r.Method, r.URL.Path, len(body), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mac := hmac.New(sha256.New, []byte(secret))
		fmt.Fprintf(mac, "POST\n%s\n%s\n%s\n%x", itemsPath, r.Header.Get("X-Timestamp"), r.Header.Get("X-Nonce"), sha256.Sum256([]byte(wantBody)))
		if got, want := r.Header.Get("X-Signature"), base64.RawURLEncoding.EncodeToString(mac.Sum(nil)); got != want {
			t.Errorf("signature = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()

	cfg := campusGatewayTestConfig(upstream.URL)
	cfg.CampusPortalAuth = config.ServiceAuth{ClientID: "portal-gw", KeyID: "pk1", ClientSecret: secret}
	h, err := New(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := h.Router()
	for _, tc := range []struct {
		name, method, path, body string
		status                   int
	}{
		{"forward entire upload", http.MethodPost, itemsPath, wantBody, http.StatusCreated},
		{"reject oversized upload", http.MethodPost, itemsPath, strings.Repeat("x", (20<<20)+1), http.StatusRequestEntityTooLarge},
		{"reject oversized message", http.MethodPost, itemsPath + "/s-01/messages", strings.Repeat("x", (256<<10)+1), http.StatusRequestEntityTooLarge},
		{"reject oversized contact book", http.MethodPut, "/api/v1/campus/contact-book", strings.Repeat("x", (256<<10)+1), http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://localhost:8088"+tc.path, strings.NewReader(tc.body))
			req.AddCookie(campusGatewayTestCookie(t, cfg))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, tc.status)
			}
		})
	}
}

func TestCampusImageProxyPreservesPublicResponse(t *testing.T) {
	const path = "/api/v1/campus/items/s-01/images/0"
	const version = "v=0123456789ab"
	const photo = "\x89PNG\r\n\x1a\nphoto"
	headers := map[string]string{
		"Content-Type":            "image/png",
		"ETag":                    `"0123456789ab"`,
		"Cache-Control":           "public, max-age=31536000, immutable",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "default-src 'none'; sandbox",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/campus/items/hidden/images/0" {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != path || r.URL.RawQuery != version || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL)
		}
		for name, value := range headers {
			w.Header().Set(name, value)
		}
		if r.Header.Get("If-None-Match") == headers["ETag"] {
			w.Header().Del("Content-Type")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = io.WriteString(w, photo)
	}))
	defer upstream.Close()

	h := Handler{portalAPIURL: upstream.URL, portalAPI: upstream.Client()}
	router := h.Router()
	for _, tc := range []struct {
		name, url, ifNoneMatch, body string
		status                       int
	}{
		{"initial read", path + "?" + version, "", photo, http.StatusOK},
		{"conditional read", path + "?" + version, headers["ETag"], "", http.StatusNotModified},
		{"hidden listing", "/api/v1/campus/items/hidden/images/0", "", "missing\n", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.ifNoneMatch != "" {
				req.Header.Set("If-None-Match", tc.ifNoneMatch)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status || rec.Body.String() != tc.body {
				t.Fatalf("response = %d %q, want %d %q", rec.Code, rec.Body.String(), tc.status, tc.body)
			}
			if tc.status == http.StatusNotFound {
				return
			}
			for name, value := range headers {
				if tc.status == http.StatusNotModified && name == "Content-Type" {
					value = ""
				}
				if got := rec.Header().Get(name); got != value {
					t.Errorf("%s = %q, want %q", name, got, value)
				}
			}
		})
	}
}
