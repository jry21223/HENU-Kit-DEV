package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersSetsProductionHSTS(t *testing.T) {
	router := gin.New()
	router.Use(SecurityHeaders("production"))
	router.GET("/probe", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"ok": true})
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/probe", nil)
	router.ServeHTTP(response, request)

	if response.Header().Get("Strict-Transport-Security") != "max-age=31536000; includeSubDomains" {
		t.Fatalf("expected production HSTS, got %q", response.Header().Get("Strict-Transport-Security"))
	}
	if response.Header().Get("Permissions-Policy") == "" {
		t.Fatal("expected Permissions-Policy header")
	}
}
