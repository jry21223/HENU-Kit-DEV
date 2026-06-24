package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"final-review-platform/services/api/internal/server"
	"final-review-platform/services/api/pkg/config"
	applogger "final-review-platform/services/api/pkg/logger"
)

func TestVersionEndpoint(t *testing.T) {
	cfg := config.Config{
		Environment:        "test",
		Port:               "0",
		Version:            "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		RateLimitRPS:       100,
		RateLimitBurst:     100,
	}
	router := server.NewRouter(cfg, applogger.New("test"), nil, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
}

func TestHealthzAndReadyzHaveSeparateSemantics(t *testing.T) {
	db := newTestDB(t)
	cfg := testConfig()
	router := server.NewRouter(cfg, applogger.New("test"), db, nil)

	health := performJSON(router, http.MethodGet, "/healthz", "", "")
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"redis":"not_configured"`) {
		t.Fatalf("expected healthz to report dependencies with 200, got %d: %s", health.Code, health.Body.String())
	}

	ready := performJSON(router, http.MethodGet, "/readyz", "", "")
	if ready.Code != http.StatusServiceUnavailable || !strings.Contains(ready.Body.String(), "not_ready") || !strings.Contains(ready.Body.String(), `"ready":false`) {
		t.Fatalf("expected readyz to fail when redis is unavailable, got %d: %s", ready.Code, ready.Body.String())
	}
}
