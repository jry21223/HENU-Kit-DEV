package accountportfolio

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const zeroTicketFacts = `{"scope":"global","as_of":"2026-11-07T12:00:00Z","stale":false,"counts":{"open":0,"in_progress":0,"resolved":0,"unresolved":0},"window":{"filter":"unresolved","matched_total":0,"limit":100,"returned":0,"has_more":false,"items":[]}}`

func TestTicketFactsHTTPReadSignsFullQueryAndRejectsFalseSuccess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		data   string
		want   bool
	}{
		{"real zero", 200, zeroTicketFacts, true}, {"stale", 200, strings.Replace(zeroTicketFacts, `"stale":false`, `"stale":true`, 1), false}, {"wrong total", 200, strings.Replace(zeroTicketFacts, `"unresolved":0`, `"unresolved":1`, 1), false}, {"missing counts", 200, `{"scope":"global"}`, false}, {"owner401", 401, "", false}, {"old owner404", 404, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.RequestURI() != "/api/v1/console/tickets/facts?status=unresolved" || r.Header.Get("X-Actor-User-Id") != "11111111-1111-4111-8111-111111111111" || r.Header.Get("X-Signature") == "" {
					t.Errorf("wrong signed ticket request")
				}
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": json.RawMessage(tc.data), "request_id": "req_facts"})
				}
			}))
			defer server.Close()
			client, err := New(server.URL, "console-gateway", "0123456789abcdef0123456789abcdef", "console-key", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			raw, err := client.TicketFacts(WithRequestID(t.Context(), "req_facts"), "11111111-1111-4111-8111-111111111111", "unresolved")
			if (err == nil) != tc.want || (!tc.want && len(raw) > 0) {
				t.Fatalf("facts raw=%s err=%v", raw, err)
			}
			if tc.status == 401 && !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("owner401 lost classification=%v", err)
			}
		})
	}
}
func TestTicketFactsClientCapsDependencyDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client, err := New(server.URL, "console-gateway", "0123456789abcdef0123456789abcdef", "console-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	raw, err := client.TicketFacts(WithRequestID(t.Context(), "req_facts"), "11111111-1111-4111-8111-111111111111", "unresolved")
	if err == nil || len(raw) > 0 || time.Since(started) > 2800*time.Millisecond {
		t.Fatalf("deadline raw=%s err=%v elapsed=%s", raw, err, time.Since(started))
	}
}
