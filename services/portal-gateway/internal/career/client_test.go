package career

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestCareerStatusAcceptsBoundedLargeResultAndRejectsOversize(t *testing.T) {
	for _, test := range []struct {
		name      string
		summary   string
		wantError error
	}{
		{name: "official multi-page result", summary: strings.Repeat("岗", 80_000)},
		{name: "response over one MiB", summary: strings.Repeat("x", maxResponseBodyBytes+1), wantError: ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"search":{"result":{"summary":` + strconv.Quote(test.summary) + `}}},"request_id":"req_large_result"}`))
			}))
			defer upstream.Close()
			client, err := NewClient(upstream.URL, "portal-gateway-career", strings.Repeat("s", 40), "active")
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.Search(context.Background(), "11111111-1111-4111-8111-111111111111", "req_large", "22222222-2222-4222-8222-222222222222")
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("error = %v, want %v", err, test.wantError)
				}
				return
			}
			if err != nil || !strings.Contains(string(result), "req_large_result") {
				t.Fatalf("large bounded result error=%v body=%d bytes", err, len(result))
			}
		})
	}
}
