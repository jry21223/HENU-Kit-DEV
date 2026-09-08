package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestOAuthContinuationAuditUsesBoundedSchema(t *testing.T) {
	var logs bytes.Buffer
	handler := &Handler{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	wrapped := handler.requestAudit(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		audit := auditFrom(request.Context())
		audit.oauthContinuation = true
		audit.continuationClientID = "portal-gateway"
		audit.continuationOutcome = "authorization_code_issued"
		writer.WriteHeader(http.StatusSeeOther)
	}))

	request := httptest.NewRequest(
		http.MethodGet,
		"https://account.example/api/v1/oauth/authorize?state=secret-state&code_challenge=secret-challenge",
		nil,
	)
	request.Header.Set("X-Request-Id", "req_bounded_continuation_event")
	wrapper := httptest.NewRecorder()
	wrapped.ServeHTTP(wrapper, request)

	var event map[string]any
	if err := json.NewDecoder(&logs).Decode(&event); err != nil {
		t.Fatalf("decode continuation event: %v; log=%s", err, logs.String())
	}
	keys := make([]string, 0, len(event))
	for key := range event {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	wantKeys := []string{"client_id", "duration_ms", "level", "msg", "outcome", "request_id", "time"}
	if !slices.Equal(keys, wantKeys) {
		t.Fatalf("continuation event keys = %v, want %v", keys, wantKeys)
	}
	if event["msg"] != "oauth_continuation" || event["request_id"] != "req_bounded_continuation_event" || event["client_id"] != "portal-gateway" || event["outcome"] != "authorization_code_issued" {
		t.Fatalf("continuation event = %#v, want bounded trusted classification", event)
	}
	for _, secret := range []string{"secret-state", "secret-challenge", request.URL.RawQuery} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("continuation event exposed %q: %s", secret, logs.String())
		}
	}
}
