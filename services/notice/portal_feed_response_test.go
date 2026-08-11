package notice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPortalFeedResponseBudgetIncludesSourceURLBytes(t *testing.T) {
	// notice_versions has a btree unique index that PostgreSQL refuses for a
	// multi-MiB source URL, so this serializer-level regression covers the
	// Owner's defensive handling of an otherwise valid in-memory candidate.
	// The public HTTP feed test covers persisted candidates and cap ordering.
	item := map[string]any{
		"id":    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		"title": "来源地址预算",
		"body":  "正文仍然很短。",
		"source": map[string]string{
			"name": "学校办公室",
			"url":  "https://example.edu/notices/" + strings.Repeat("u", portalFeedResponseByteLimit),
		},
		"created_at": "2026-08-01T00:00:00Z",
	}
	payload, err := portalFeedResponseBytes([]map[string]any{item}, "req_source_url_budget")
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) <= portalFeedResponseByteLimit {
		t.Fatalf("portal response with oversized source URL = %d bytes, want over budget", len(payload))
	}
	budget, err := newPortalFeedResponseBudget("req_source_url_budget")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := budget.accepts(item)
	if err != nil || accepted {
		t.Fatalf("over-budget source URL candidate accepted=%t err=%v; want skip", accepted, err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://notice.test/api/v1/portal/notices", nil)
	request = request.WithContext(context.WithValue(request.Context(), requestIDKey, "req_source_url_budget"))
	response := httptest.NewRecorder()
	writePortalFeed(response, request, []map[string]any{item})
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "https://example.edu/notices/") {
		t.Fatalf("over-budget source URL Owner response = %d: %s", response.Code, response.Body.String())
	}
}
