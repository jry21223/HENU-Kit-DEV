package tests

import (
	"net/http"
	"net/http/httptest"
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
