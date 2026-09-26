package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQQBindingRejectsCrossOriginAndAnonymous(t *testing.T) {
	handler := newFoodImageHandler(t, "https://unused.test")
	for _, tc := range []struct {
		origin string
		want   int
	}{{"https://attacker.test", 403}, {"https://portal.test", 401}} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/account/qq-binding/authorize", strings.NewReader(`{"token":"test"}`))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("origin %s status %d want %d", tc.origin, rec.Code, tc.want)
		}
	}
}
