package notice

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// validTestConfig mirrors the minimum accepted by NewClient: an absolute
// http(s) URL without credentials and a clientSecret of at least 32 bytes.
const testClientSecret = "notice-test-client-secret-at-least-32-bytes"

func noticeItemJSON(id, state, title string) string {
	return fmt.Sprintf(`{"id":%q,"source":{"id":"11111111-1111-4111-8111-111111111111","code":"registrar","name":"教务处"},"version":1,"title":%q,"body":"正文","source_url":"https://example.edu/notices/1","content_hash":"0000000000000000000000000000000000000000000000000000000000000000","state":%q,"revision":1,"created_at":"2026-08-07T00:00:00Z","distribution_count":0}`, id, title, state)
}

func noticeOwnerEnvelope(data string) string {
	return `{"data":` + data + `,"request_id":"req_notice_owner"}`
}

func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	client, err := NewClient(serverURL, "notice-test-client", testClientSecret, "notice-test-key")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// TestListReturnsOnlyDistributedItems verifies that only contract-valid items
// in the distributed lifecycle state leave the Gateway.
func TestListReturnsOnlyDistributedItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(noticeOwnerEnvelope(`{"items":[` +
			noticeItemJSON("22222222-2222-4222-8222-222222222221", "distributed", "已发布公告") + `,` +
			noticeItemJSON("22222222-2222-4222-8222-222222222222", "approved", "待发布公告") + `,` +
			noticeItemJSON("22222222-2222-4222-8222-222222222223", "pending_review", "草稿公告") +
			`],"generated_at":"2026-08-07T00:00:00Z"}`)))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	data, err := client.List(context.Background(), "actor-user", "req-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(data.Items) != 1 {
		t.Fatalf("filtered items = %d, want 1 (distributed only)", len(data.Items))
	}
	if data.Items[0].State != "distributed" || data.Items[0].Title != "已发布公告" {
		t.Fatalf("filtered item = %+v, want the distributed one", data.Items[0])
	}
}

// TestListEmptyItemsSucceeds verifies that an explicitly empty items array is
// a successful read (the honest "no published announcements" state).
func TestListEmptyItemsSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(noticeOwnerEnvelope(`{"items":[],"generated_at":"2026-08-07T00:00:00Z"}`)))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	data, err := client.List(context.Background(), "actor-user", "req-2")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if data.Items == nil || len(data.Items) != 0 {
		t.Fatalf("filtered items = %#v, want non-nil empty items", data.Items)
	}
}

// TestListRejectsMissingItems verifies that an absent or null items field is
// an invalid snapshot rather than a genuine empty feed.
func TestListRejectsMissingItems(t *testing.T) {
	for _, body := range []string{
		noticeOwnerEnvelope(`{"generated_at":"2026-08-07T00:00:00Z"}`),
		noticeOwnerEnvelope(`{"items":null,"generated_at":"2026-08-07T00:00:00Z"}`),
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

func TestListRejectsMissingGeneratedAt(t *testing.T) {
	for _, body := range []string{
		noticeOwnerEnvelope(`{"items":[]}`),
		noticeOwnerEnvelope(`{"items":[],"generated_at":null}`),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		client := newTestClient(t, server.URL)
		if _, err := client.List(context.Background(), "actor-user", "req-missing-generated-at"); err == nil {
			server.Close()
			t.Fatalf("List with %s: want error, got nil", body)
		}
		server.Close()
	}
}

// TestListRejectsDistributedItemMissingPortalFields locks the browser-facing
// contract at the owner boundary: a distributed item is not safe to forward
// unless it contains the fields the Portal renders, including its source.
func TestListRejectsDistributedItemMissingPortalFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(noticeOwnerEnvelope(`{"items":[{"id":"notice-1","state":"distributed","body":"正文"}],"generated_at":"2026-08-07T00:00:00Z"}`)))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if _, err := client.List(context.Background(), "actor-user", "req-missing-fields"); err == nil {
		t.Fatal("List: want error for distributed item missing title and source, got nil")
	}
}

func TestListRejectsFieldsOutsidePortalContract(t *testing.T) {
	itemWithExtraField := strings.TrimSuffix(noticeItemJSON("22222222-2222-4222-8222-222222222224", "distributed", "公告"), "}") + `,"owner_internal":true}`
	for _, body := range []string{
		noticeOwnerEnvelope(`{"items":[],"generated_at":"2026-08-07T00:00:00Z","owner_internal":true}`),
		noticeOwnerEnvelope(`{"items":[` + itemWithExtraField + `],"generated_at":"2026-08-07T00:00:00Z"}`),
		`{"data":{"items":[],"generated_at":"2026-08-07T00:00:00Z"},"request_id":"req_notice_owner","owner_internal":true}`,
		noticeOwnerEnvelope(`{"items":[],"generated_at":"2026-08-07T00:00:00Z"}`) + `{}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		client := newTestClient(t, server.URL)
		if _, err := client.List(context.Background(), "actor-user", "req-extra-field"); err == nil {
			server.Close()
			t.Fatalf("List with %s: want error, got nil", body)
		}
		server.Close()
	}
}

func TestListRejectsMissingOwnerRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"items":[],"generated_at":"2026-08-07T00:00:00Z"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if _, err := client.List(context.Background(), "actor-user", "req-missing-owner-request-id"); err == nil {
		t.Fatal("List: want error for missing owner request_id, got nil")
	}
}

func TestListRejectsSnapshotOverOwnerLimit(t *testing.T) {
	item := noticeItemJSON("22222222-2222-4222-8222-222222222225", "distributed", "公告")
	body := noticeOwnerEnvelope(`{"items":[` + strings.Repeat(item+`,`, 50) + item + `],"generated_at":"2026-08-07T00:00:00Z"}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if _, err := client.List(context.Background(), "actor-user", "req-too-many-items"); err == nil {
		t.Fatal("List: want error for 51 owner items, got nil")
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
