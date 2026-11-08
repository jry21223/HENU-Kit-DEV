package food

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const zeroFacts = `{"scope":"global","as_of":"2026-11-07T12:00:00Z","stale":false,"counts":{"pending_submissions":0,"open_anomalies":0,"pending_tier_adjustments":0},"windows":{"submissions":{"filter":"pending","limit":200,"returned":0,"has_more":false,"items":[]},"anomalies":{"filter":"open","limit":200,"returned":0,"has_more":false,"items":[]},"tier_adjustments":{"filter":"pending","limit":200,"returned":0,"has_more":false,"items":[]}}}`

func TestFoodFactsHTTPReadValidatesShapeScopeAndDependencyFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		data   string
		want   bool
	}{
		{"real zero", 200, zeroFacts, true}, {"missing counts", 200, `{"scope":"global"}`, false}, {"owner401", 401, "", false}, {"old owner404", 404, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.RequestURI() != "/api/v1/workspace/facts" || r.Header.Get("X-Permission-Code") != "food.read" || r.Header.Get("X-Scope-Kind") != "product" || r.Header.Get("X-Product-Code") != "food" || r.Header.Get("X-Actor-User-Id") != "11111111-1111-4111-8111-111111111111" || r.Header.Get("X-Signature") == "" {
					t.Errorf("wrong signed food request")
				}
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": json.RawMessage(tc.data), "request_id": "req_facts"})
				}
			}))
			defer server.Close()
			client, err := New(server.URL, "console-gateway-food", "0123456789abcdef0123456789abcdef", "food-key", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			raw, err := client.Facts(WithRequestID(t.Context(), "req_facts"), "11111111-1111-4111-8111-111111111111")
			if (err == nil) != tc.want || (!tc.want && len(raw) > 0) {
				t.Fatalf("facts raw=%s err=%v", raw, err)
			}
			if tc.status == 401 && !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("owner401 lost classification=%v", err)
			}
		})
	}
}
func TestFoodFactsClientCapsDependencyDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client, err := New(server.URL, "console-gateway-food", "0123456789abcdef0123456789abcdef", "food-key", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	raw, err := client.Facts(t.Context(), "11111111-1111-4111-8111-111111111111")
	if err == nil || len(raw) > 0 || time.Since(started) > 2800*time.Millisecond {
		t.Fatalf("deadline raw=%s err=%v elapsed=%s", raw, err, time.Since(started))
	}
}
