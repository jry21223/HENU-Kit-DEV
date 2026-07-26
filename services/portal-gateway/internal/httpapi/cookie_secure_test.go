package httpapi

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"henukit.dev/portal-gateway/internal/config"
	"henukit.dev/portal-gateway/internal/session"
)

func TestBrowserCookies(t *testing.T) {
	_, trustedLoopback, err := net.ParseCIDR("127.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{
		localOAuthCookie:   "local_oauth",
		localSessionCookie: "local_session",
		trustedProxies:     []*net.IPNet{trustedLoopback},
	}

	tests := []struct {
		name        string
		tls         bool
		remote      string
		forwarded   string
		wantOAuth   string
		wantSession string
		wantSecure  bool
	}{
		{name: "direct HTTPS", tls: true, wantOAuth: "__Host-henukit_portal_oauth", wantSession: "__Host-henukit_portal_session", wantSecure: true},
		{name: "trusted TLS terminator", remote: "127.0.0.1:43120", forwarded: "https", wantOAuth: "__Host-henukit_portal_oauth", wantSession: "__Host-henukit_portal_session", wantSecure: true},
		{name: "trimmed case insensitive forwarded proto", remote: "127.0.0.1:43120", forwarded: " HTTPS ", wantOAuth: "__Host-henukit_portal_oauth", wantSession: "__Host-henukit_portal_session", wantSecure: true},
		{name: "untrusted peer cannot claim HTTPS", remote: "203.0.113.9:43120", forwarded: "https", wantOAuth: "local_oauth", wantSession: "local_session"},
		{name: "local HTTP", wantOAuth: "local_oauth", wantSession: "local_session"},
		{name: "trusted proxy forwarded HTTP", remote: "127.0.0.1:43120", forwarded: "http", wantOAuth: "local_oauth", wantSession: "local_session"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://portal.test/", nil)
			if test.tls {
				request.TLS = &tls.ConnectionState{}
			}
			if test.remote != "" {
				request.RemoteAddr = test.remote
			}
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-Proto", test.forwarded)
			}

			got := handler.browserCookies(request)
			if got.oauth != test.wantOAuth || got.session != test.wantSession || got.secure != test.wantSecure {
				t.Fatalf("handler.browserCookies() = %+v, want oauth=%q session=%q secure=%v", got, test.wantOAuth, test.wantSession, test.wantSecure)
			}
		})
	}
}

func TestLogoutUsesHTTPSCookieProfile(t *testing.T) {
	handler := &Handler{localSessionCookie: "local_session"}
	request := httptest.NewRequest(http.MethodPost, "https://portal.test/api/v1/session/logout", nil)
	request.TLS = &tls.ConnectionState{}
	recorder := httptest.NewRecorder()

	handler.logout(recorder, request)

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("logout set %d cookies, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "__Host-henukit_portal_session" || !cookie.Secure || !cookie.HttpOnly ||
		cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != -1 {
		t.Fatalf("logout cookie = %+v", cookie)
	}
}

func TestReadSessionUsesSelectedCookieProfile(t *testing.T) {
	codec, err := session.NewCodec([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := codec.Encode(session.Value{
		UserID: "user-1", ExchangeToken: strings.Repeat("x", 32), ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := &Handler{sessionCodec: codec, localSessionCookie: "local_session"}

	httpsRequest := httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/session", nil)
	httpsRequest.TLS = &tls.ConnectionState{}
	httpsRequest.AddCookie(&http.Cookie{Name: "__Host-henukit_portal_session", Value: encoded})
	if _, err := handler.readSession(httpsRequest); err != nil {
		t.Fatalf("HTTPS profile session was rejected: %v", err)
	}

	wrongNameRequest := httptest.NewRequest(http.MethodGet, "https://portal.test/api/v1/session", nil)
	wrongNameRequest.TLS = &tls.ConnectionState{}
	wrongNameRequest.AddCookie(&http.Cookie{Name: "local_session", Value: encoded})
	if _, err := handler.readSession(wrongNameRequest); err == nil {
		t.Fatal("HTTPS profile accepted the local HTTP cookie name")
	}

	localRequest := httptest.NewRequest(http.MethodGet, "http://portal.test/api/v1/session", nil)
	localRequest.AddCookie(&http.Cookie{Name: "local_session", Value: encoded})
	if _, err := handler.readSession(localRequest); err != nil {
		t.Fatalf("local HTTP profile session was rejected: %v", err)
	}
}

func TestNewRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	_, err := New(config.Config{
		SessionKey:        []byte("0123456789abcdef0123456789abcdef"),
		TrustedProxyCIDRs: []string{"not-a-cidr"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid trusted proxy CIDR") {
		t.Fatalf("New() error = %v, want invalid trusted proxy CIDR", err)
	}
}
