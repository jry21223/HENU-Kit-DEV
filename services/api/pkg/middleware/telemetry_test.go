package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestHTTPRegistryCounts5xxAndCalculatesP95(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := NewHTTPRegistry()
	router := gin.New()
	router.Use(registry.Observe())
	router.GET("/ok", func(ctx *gin.Context) { time.Sleep(time.Millisecond); ctx.Status(http.StatusNoContent) })
	router.GET("/error", func(ctx *gin.Context) { ctx.Status(http.StatusServiceUnavailable) })
	for _, path := range []string{"/ok", "/ok", "/error"} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	snapshot := registry.Snapshot()
	if snapshot.Requests != 3 || snapshot.Errors5xx != 1 || snapshot.P95MS <= 0 {
		t.Fatalf("unexpected telemetry: %#v", snapshot)
	}
}
