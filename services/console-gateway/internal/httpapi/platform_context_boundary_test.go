package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// These exercise browser HTTP -> signed Core HTTP without a database. Core
// authorization/Scope policy remains covered by the sealed real PG/Redis suite.
func TestPlatformContextReadPreservesCoreFailureMeaning(t *testing.T) {
	for _, test := range []struct {
		name         string
		coreStatus   int
		coreCode     string
		want         int
		clearSession bool
	}{
		{"invalid cursor", 400, "INVALID_REQUEST", 400, false},
		{"denied scope", 403, "ACCESS_DENIED", 403, false},
		{"expired session", 401, "SESSION_INVALID", 401, true},
		{"service authentication failure", 401, "SIGNATURE_INVALID", 503, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var received url.Values
			server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
				received = r.URL.Query()
				w.WriteHeader(test.coreStatus)
				_, _ = fmt.Fprintf(w, `{"error":{"code":%q,"message":"isolated rejection"}}`, test.coreCode)
			})
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/operations?accounts_page=2&accounts_cursor=account-opaque&sessions_page=1&inbox_page=3&inbox_cursor=inbox-opaque&audit_page=1&snapshot_at=2026-11-07T12%3A00%3A00Z", nil)
			request.AddCookie(cookie)
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.want {
				t.Fatalf("status=%d, want %d", response.StatusCode, test.want)
			}
			if received.Get("accounts_cursor") != "account-opaque" || received.Get("inbox_cursor") != "inbox-opaque" || received.Get("sessions_page") != "1" || received.Get("audit_page") != "1" || received.Get("snapshot_at") != "2026-11-07T12:00:00Z" {
				t.Fatalf("independent context changed: %v", received)
			}
			cleared := false
			for _, value := range response.Cookies() {
				if value.Name == sessionCookie && value.MaxAge < 0 {
					cleared = true
				}
			}
			if cleared != test.clearSession {
				t.Fatalf("session cleared=%t, want %t", cleared, test.clearSession)
			}
		})
	}
}

func TestPlatformContextAccountSearchKeepsIdentityInBodyThroughSignedHTTP(t *testing.T) {
	var receivedPath, receivedQuery string
	var body map[string]any
	server, cookie := sessionBoundaryGateway(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPath, receivedQuery = r.URL.Path, r.URL.RawQuery
		payload, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(payload, &body); err != nil {
			t.Errorf("body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"accounts":[],"next_page":null,"next_cursor":null},"request_id":"req_search_boundary"}`)
	})
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/operations/accounts/search", strings.NewReader(`{"query":"isolated.search@henu.edu.cn","page":1,"snapshot_at":"2026-11-07T12:00:00Z"}`))
	request.AddCookie(cookie)
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || receivedPath != "/api/v1/platform-operations/accounts/search" || receivedQuery != "" || body["query"] != "isolated.search@henu.edu.cn" {
		t.Fatalf("search status=%d path=%q query=%q body=%v", response.StatusCode, receivedPath, receivedQuery, body)
	}
}
