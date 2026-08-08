package notice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// validTestConfig mirrors the minimum accepted by NewClient: an absolute
// http(s) URL without credentials and a clientSecret of at least 32 bytes.
const testClientSecret = "notice-test-client-secret-at-least-32-bytes"

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(serverURL, "notice-test-client", testClientSecret, "notice-test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestListReturnsOnlyDistributedItems verifies that only items in the
// distributed lifecycle state leave the Gateway, that items whose lifecycle
// cannot be parsed are skipped without failing the whole read, and that a
// genuine empty feed stays a success.
func TestListReturnsOnlyDistributedItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[` +
			`{"state":"distributed","title":"已发布公告"},` +
			`{"state":"approved","title":"待发布公告"},` +
			`{"state":123,"title":"畸形条目"},` +
			`{"state":"pending","title":"草稿公告"}` +
			`],"generated_at":"2026-08-07T00:00:00Z"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	data, err := client.List(context.Background(), "actor-user", "req-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var feed struct {
		Items []struct {
			State string `json:"state"`
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &feed); err != nil {
		t.Fatalf("unmarshal filtered feed: %v", err)
	}
	if len(feed.Items) != 1 {
		t.Fatalf("filtered items = %d, want 1 (distributed only)", len(feed.Items))
	}
	if feed.Items[0].State != "distributed" || feed.Items[0].Title != "已发布公告" {
		t.Fatalf("filtered item = %+v, want the distributed one", feed.Items[0])
	}
}

// TestListEmptyItemsSucceeds verifies that an explicitly empty items array is
// a successful read (the honest "no published announcements" state).
func TestListEmptyItemsSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[],"generated_at":"2026-08-07T00:00:00Z"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	data, err := client.List(context.Background(), "actor-user", "req-2")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(string(data), `"items":[]`) {
		t.Fatalf("filtered feed = %s, want empty items array", data)
	}
}

// TestListRejectsMissingItems verifies that an absent or null items field is
// an invalid snapshot rather than a genuine empty feed.
func TestListRejectsMissingItems(t *testing.T) {
	for _, body := range []string{
		`{"data":{"generated_at":"2026-08-07T00:00:00Z"}}`,
		`{"data":{"items":null,"generated_at":"2026-08-07T00:00:00Z"}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		client := newTestClient(t, server.URL)
		if _, err := client.List(context.Background(), "actor-user", "req-3"); err == nil {
			server.Close()
			t.Fatalf("List with %s: want error, got nil", body)
		}
		server.Close()
	}
}

// TestListRejectsNonOKStatus verifies that a failing owner read surfaces as
// an error carrying the status code for operator logs.
func TestListRejectsNonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.List(context.Background(), "actor-user", "req-4")
	if err == nil {
		t.Fatal("List: want error on 500, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("List error = %v, want status 500", err)
	}
}
