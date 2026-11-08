package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libraryapi "henukit.dev/console-gateway/internal/library"
	"henukit.dev/console-gateway/internal/platformcore"
	"henukit.dev/console-gateway/internal/session"
)

func TestLibraryReadinessForwardingAndRollingCompatibility(t *testing.T) {
	const userID = "171f1c6f-7b10-4c92-91a2-b39bf5af5302"
	const token = "exchange_token_with_at_least_32_characters"
	const readiness = `{"management":"not_ready","reason":"management_migration_pending","capabilities":{"workspace":false,"commands":false,"operation_results":false},"checked_at":"2026-11-07T18:00:00Z"}`
	for _, tc := range []struct {
		name                string
		ownerStatus, status int
		coreErr             error
		cookie              bool
	}{
		{"explicit readiness", 200, 200, nil, true},
		{"old owner lacks readiness", 404, 503, nil, true},
		{"source failure", 503, 503, nil, true},
		{"owner service authentication failure", 401, 503, nil, true},
		{"no session", 200, 401, nil, false},
		{"permission or Scope denied", 200, 403, platformcore.ErrForbidden, true},
		{"revoked session", 200, 401, platformcore.ErrUnauthorized, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ownerCalls := 0
			owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ownerCalls++
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/console-readiness" || r.Header.Get("X-Actor-User-Id") != userID || r.Header.Get("X-Permission-Code") != "library.read" || r.Header.Get("X-Scope-Kind") != "product" || r.Header.Get("X-Product-Code") != "library" || r.Header.Get("X-Signature") == "" {
					t.Errorf("unexpected readiness forwarding: %s %s / %v", r.Method, r.URL.Path, r.Header)
				}
				w.WriteHeader(tc.ownerStatus)
				_, _ = io.WriteString(w, `{"data":`+readiness+`,"request_id":"req_owner_readiness"}`)
			}))
			defer owner.Close()
			client, err := libraryapi.New(owner.URL, "console-gateway-library", "0123456789abcdef0123456789abcdef", "test", owner.Client())
			if err != nil {
				t.Fatal(err)
			}
			codec, _ := session.New([]byte("0123456789abcdef0123456789abcdef"))
			platform := &fakePlatform{exchange: platformcore.Exchange{ExchangeToken: token}, checkErr: tc.coreErr}
			handler, err := New("https://account.henukit.test", "console-gateway", "https://console.henukit.test/api/v1/auth/callback", platform, nil, fakeOverview{}, testRedis(t), codec, slog.New(slog.NewJSONHandler(io.Discard, nil)), client)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/library/readiness", nil)
			request.Header.Set("X-Actor-User-Id", "forged-browser-actor")
			if tc.cookie {
				encoded, _ := codec.Encode(session.Value{UserID: userID, ExchangeToken: token, ExpiresAt: time.Now().Add(time.Minute)})
				request.AddCookie(&http.Cookie{Name: sessionCookie, Value: encoded})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("HTTP status = %d, want %d; body=%s", response.Code, tc.status, response.Body.String())
			}
			if tc.ownerStatus == http.StatusUnauthorized && (len(response.Result().Cookies()) != 0 || !strings.Contains(response.Body.String(), "DEPENDENCY_UNAVAILABLE")) {
				t.Fatalf("owner service failure changed browser Session: cookies=%v body=%s", response.Result().Cookies(), response.Body.String())
			}
			if tc.coreErr == platformcore.ErrUnauthorized && (len(response.Result().Cookies()) != 1 || response.Result().Cookies()[0].MaxAge != -1) {
				t.Fatal("verified invalid Session must expire the local cookie")
			}
			if tc.status == 200 {
				var envelope struct {
					Data json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || string(envelope.Data) != readiness {
					t.Fatalf("readiness changed: %s / %v", envelope.Data, err)
				}
			}
			if !tc.cookie || tc.coreErr != nil {
				if ownerCalls != 0 {
					t.Fatalf("denied request reached owner %d times", ownerCalls)
				}
			}
		})
	}
}
