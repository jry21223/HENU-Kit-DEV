package sessionclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"henukit.dev/food-mcp/internal/sessionclient"
)

func TestResolveDoesNotSendCookiesToRedirectTarget(t *testing.T) {
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Add(1)
	}))
	defer target.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer gateway.Close()
	client, err := sessionclient.NewClient(gateway.URL+"/api/v1/session", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Resolve(context.Background(), http.Header{"Cookie": {"__Host-henukit_portal_session=private-fixture"}})
	if !errors.Is(err, sessionclient.ErrUnavailable) || leaked.Load() != 0 {
		t.Fatalf("redirect followed: err=%v requests=%d", err, leaked.Load())
	}
}

func TestResolveUsesOnlyConfiguredLocalCookieAndSkipsMissingSession(t *testing.T) {
	var lookups atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lookups.Add(1)
		if r.Header.Get("Cookie") != "henukit_portal_session_local=local-fixture" || r.Header.Get("X-Forwarded-Proto") != "" {
			t.Error("local session profile was not preserved")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id": "44444444-4444-4444-8444-444444444444", "expires_at": time.Now().Add(time.Hour),
		})
	}))
	defer gateway.Close()
	client, err := sessionclient.NewClient(gateway.URL+"/api/v1/session", "henukit_portal_session_local")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resolve(context.Background(), nil); !errors.Is(err, sessionclient.ErrNoSession) || lookups.Load() != 0 {
		t.Fatalf("missing session made a request: %v", err)
	}
	account, err := client.Resolve(context.Background(), http.Header{"Cookie": {"henukit_portal_session_local=local-fixture; unrelated=private"}})
	if err != nil || account.UserID != "44444444-4444-4444-8444-444444444444" || account.DisplayName != "Kit 用户" {
		t.Fatalf("legacy local account = %+v, %v", account, err)
	}
}

func TestNewClientAllowsDisabledLookupButRejectsInvalidConfiguration(t *testing.T) {
	client, err := sessionclient.NewClient("", "")
	if err != nil || client != nil {
		t.Fatalf("disabled lookup = %v, %v", client, err)
	}
	for _, endpoint := range []string{
		"file:///api/v1/session", "https://example.invalid/other",
		"https://user:password@example.invalid/api/v1/session", "https://example.invalid/api/v1/session?secret=value",
		"https://example.invalid/api/v1/session#fragment",
	} {
		if _, err := sessionclient.NewClient(endpoint, ""); err == nil {
			t.Fatalf("invalid endpoint accepted: %q", endpoint)
		}
	}
}
