package smtpprovider

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

type captureMailer struct{ calls int }

func (mailer *captureMailer) Send(_ context.Context, message Mail) error {
	mailer.calls++
	if message.Recipient != "student@henu.edu.cn" || message.Code != "123456" || message.MessageID == "" {
		return context.Canceled
	}
	return nil
}

func TestProviderAuthenticatesAndDeduplicatesAcceptedMail(t *testing.T) {
	mailer := &captureMailer{}
	handler, err := New(Config{Token: "provider-token-at-least-32-characters", LedgerDirectory: filepath.Join(t.TempDir(), "ledger"), Mailer: mailer})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	body := []byte(`{"recipient":"student@henu.edu.cn","template":"henukit_verification_code","variables":{"code":"123456","purpose":"login","expires_at":"2099-07-22T12:00:00Z"},"request_id":"req_provider_001","idempotency_key":"verification:job-001"}`)
	send := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/internal/send", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "verification:job-001")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := send("wrong-token"); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized provider request = %d, want 401", response.Code)
	}
	first, replay := send("provider-token-at-least-32-characters"), send("provider-token-at-least-32-characters")
	if first.Code != http.StatusOK || replay.Code != http.StatusOK || first.Body.String() != replay.Body.String() {
		t.Fatalf("provider acceptance/replay = %d/%d %q/%q", first.Code, replay.Code, first.Body.String(), replay.Body.String())
	}
	if mailer.calls != 1 {
		t.Fatalf("idempotent provider sent %d messages, want 1", mailer.calls)
	}
}
