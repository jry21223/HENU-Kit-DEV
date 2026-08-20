package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRouterLiveMissingDSN(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "live")
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORTAL_ORIGIN", "https://portal.example.com")
	t.Setenv("STUDY_DATABASE_URL", "")
	t.Setenv("PORTAL_DATABASE_URL", "")

	handler, err := NewRouter()
	if err == nil {
		t.Fatal("expected live mode without DSN to fail router construction")
	}
	if handler != nil {
		t.Fatal("expected nil handler when live startup fails")
	}
}

func TestNewRouterLiveMissingOrigin(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "live")
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORTAL_ORIGIN", "")
	t.Setenv("STUDY_DATABASE_URL", "postgres://x")
	t.Setenv("PORTAL_DATABASE_URL", "mysql://x")

	_, err := NewRouter()
	if err == nil {
		t.Fatal("expected live mode without PORTAL_ORIGIN to fail")
	}
}

func TestNewRouterMockNoDSN(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORTAL_ORIGIN", "")
	t.Setenv("STUDY_DATABASE_URL", "")
	t.Setenv("PORTAL_DATABASE_URL", "")

	handler, err := NewRouter()
	if err != nil {
		t.Fatalf("mock mode without DSN should start: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notices", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("notices status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode notices: %v", err)
	}
	notices, ok := body["notices"].([]any)
	if !ok {
		t.Fatalf("notices payload missing: %#v", body)
	}
	if len(notices) != 0 {
		t.Fatalf("expected empty notices, got %#v", notices)
	}

	// Practice reads were removed with ADR-0036: the legacy stats route no
	// longer exists on portal-api and fails closed with a 404 instead of a
	// mock success body.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/practice/stats", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("removed practice stats route = %d, want 404", rec.Code)
	}
}

func TestCORSNeverStarWithCredentials(t *testing.T) {
	t.Setenv("PORTAL_API_MODE", "mock")
	t.Setenv("PORTAL_ORIGIN", "")
	t.Setenv("STUDY_DATABASE_URL", "")
	t.Setenv("PORTAL_DATABASE_URL", "")

	handler, err := NewRouter()
	if err != nil {
		t.Fatalf("router: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3001")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Fatal("must never emit Access-Control-Allow-Origin: *")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:3001" {
		t.Fatalf("Allow-Origin = %q, want reflected origin", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("expected credentials true with reflected origin")
	}
}
