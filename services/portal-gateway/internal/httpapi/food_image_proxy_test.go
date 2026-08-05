package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"henukit.dev/portal-gateway/internal/config"
)

// Food photos are served as image bytes by the Portal API. The proxy used to
// stamp application/json on every response it forwarded, which left a browser
// unable to render them.
func TestProxyForwardsFoodImageContentTypeAndCacheHeaders(t *testing.T) {
	const body = "\x89PNG\r\n\x1a\nfake-bytes"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/food/posts/survey-01/images/0" {
			t.Errorf("upstream path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "inline")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()

	handler := newFoodImageHandler(t, upstream.URL)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/v1/food/posts/survey-01/images/0", nil),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := recorder.Header().Get("ETag"); got != `"abc123"` {
		t.Errorf("ETag = %q", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if recorder.Body.String() != body {
		t.Errorf("body = %q, want the upstream bytes unchanged", recorder.Body.String())
	}
}

// Without the conditional request reaching the owner, every viewport would
// refetch the full photo instead of revalidating it.
func TestProxyForwardsConditionalRequestForFoodImage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"abc123"` {
			t.Errorf("upstream If-None-Match = %q", r.Header.Get("If-None-Match"))
		}
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer upstream.Close()

	handler := newFoodImageHandler(t, upstream.URL)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/food/posts/survey-01/images/0", nil)
	request.Header.Set("If-None-Match", `"abc123"`)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", recorder.Code)
	}
}

// A route that answers without a content type must still read as JSON, which is
// what every other proxied Portal API route returns.
func TestProxyDefaultsToJSONWhenUpstreamOmitsContentType(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	handler := newFoodImageHandler(t, upstream.URL)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/api/v1/food/posts", nil),
	)

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func newFoodImageHandler(t *testing.T, portalAPIURL string) http.Handler {
	t.Helper()
	handler, err := New(config.Config{
		SessionKey:        []byte("0123456789abcdef0123456789abcdef"),
		PlatformCoreURL:   "https://core.test",
		PlatformClientID:  "portal-gateway",
		PlatformSecret:    "portal-client-secret-with-enough-entropy",
		PlatformKeyID:     "platform-key-1",
		PortalRedirectURI: "https://portal.test/api/v1/auth/callback",
		PortalOrigin:      "https://portal.test",
		PortalAPIURL:      portalAPIURL,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return handler.Router()
}
