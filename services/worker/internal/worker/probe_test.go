package worker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"final-review-platform/services/worker/pkg/config"
)

func TestProbeHealthzAndReadyz(t *testing.T) {
	handler := NewProbeHandler(config.Config{Environment: "test"}, newWorkerTestDB(t), nil)

	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusOK || !strings.Contains(healthResponse.Body.String(), "final-review-worker") {
		t.Fatalf("expected worker healthz 200, got %d: %s", healthResponse.Code, healthResponse.Body.String())
	}

	readyRequest := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyResponse := httptest.NewRecorder()
	handler.ServeHTTP(readyResponse, readyRequest)
	if readyResponse.Code != http.StatusServiceUnavailable || !strings.Contains(readyResponse.Body.String(), `"ready":false`) || !strings.Contains(readyResponse.Body.String(), `"redis":"not_configured"`) {
		t.Fatalf("expected worker readyz 503 without redis, got %d: %s", readyResponse.Code, readyResponse.Body.String())
	}
}
