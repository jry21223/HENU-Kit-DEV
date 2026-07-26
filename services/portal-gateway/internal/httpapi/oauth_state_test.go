package httpapi

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"henukit.dev/portal-gateway/internal/config"
)

func TestOAuthCallbackConsumesStateCreatedByLogin(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	platformCore := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/oauth/token" {
			t.Fatalf("Platform Core path = %q", request.URL.Path)
		}
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(platformCore.Close)

	handler, err := New(config.Config{
		PlatformCoreURL:       platformCore.URL,
		PlatformCorePublicURL: "https://accounts.example",
		PlatformClientID:      "portal-gateway",
		PlatformSecret:        "portal-client-secret-with-enough-entropy",
		PlatformKeyID:         "active-key",
		PortalRedirectURI:     "https://portal.example/api/v1/auth/callback",
		SessionKey:            []byte("0123456789abcdef0123456789abcdef"),
	}, redisClient)
	if err != nil {
		t.Fatal(err)
	}

	loginRequest := httptest.NewRequest(http.MethodGet, "https://portal.example/api/v1/auth/login?return_to=/account", nil)
	loginRequest.TLS = &tls.ConnectionState{}
	loginRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(loginRecorder, loginRequest)

	loginResponse := loginRecorder.Result()
	if loginResponse.StatusCode != http.StatusFound {
		t.Fatalf("login status = %d", loginResponse.StatusCode)
	}
	location, err := url.Parse(loginResponse.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	state := location.Query().Get("state")
	if state == "" {
		t.Fatal("login redirect omitted OAuth state")
	}
	var oauthCookie *http.Cookie
	for _, cookie := range loginResponse.Cookies() {
		if cookie.Name == "__Host-henukit_portal_oauth" {
			oauthCookie = cookie
			break
		}
	}
	if oauthCookie == nil {
		t.Fatal("login response omitted OAuth cookie")
	}

	callbackURL := "https://portal.example/api/v1/auth/callback?code=test-code&state=" + url.QueryEscape(state)
	callbackRequest := httptest.NewRequest(http.MethodGet, callbackURL, nil)
	callbackRequest.TLS = &tls.ConnectionState{}
	callbackRequest.AddCookie(oauthCookie)
	callbackRecorder := httptest.NewRecorder()
	handler.Router().ServeHTTP(callbackRecorder, callbackRequest)

	if callbackRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("callback status = %d, body = %s; want code exchange response", callbackRecorder.Code, strings.TrimSpace(callbackRecorder.Body.String()))
	}
}
