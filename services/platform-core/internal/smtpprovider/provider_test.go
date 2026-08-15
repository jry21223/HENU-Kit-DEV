package smtpprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type captureMailer struct {
	calls int
	err   error
	mails []Mail
}

func (mailer *captureMailer) Send(_ context.Context, message Mail) error {
	mailer.calls++
	mailer.mails = append(mailer.mails, message)
	if mailer.err != nil {
		return mailer.err
	}
	if message.Recipient != "student@henu.edu.cn" || message.Code != "123456" || message.MessageID == "" {
		return context.Canceled
	}
	return nil
}

func TestProviderAuthenticatesAndDeduplicatesAcceptedMail(t *testing.T) {
	mailer := &captureMailer{}
	ledger := filepath.Join(t.TempDir(), "ledger")
	var logs bytes.Buffer
	handler, err := newTestProvider(ledger, mailer, &logs)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	body := []byte(`{"recipient":"student@henu.edu.cn","template":"henukit_verification_code","variables":{"code":"123456","purpose":"login","expires_at":"2099-07-22T12:00:00Z"},"request_id":"req_provider_001","idempotency_key":"verification:job-001"}`)
	send := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/internal/send", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "verification:job-001")
		request.Header.Set("X-Request-ID", "req_provider_001")
		request.Header.Set("X-Mail-Attempt", "2")
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
	if got := mailer.mails[0].MessageID; !strings.HasSuffix(got, "@notify.henukit.cn") {
		t.Fatalf("message ID = %q, want notify.henukit.cn domain", got)
	}
	restarted, err := newTestProvider(ledger, mailer, &logs)
	if err != nil {
		t.Fatalf("restart provider: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/internal/send", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer provider-token-at-least-32-characters")
	request.Header.Set("Idempotency-Key", "verification:job-001")
	request.Header.Set("X-Request-ID", "req_provider_001")
	request.Header.Set("X-Mail-Attempt", "3")
	restartReplay := httptest.NewRecorder()
	restarted.ServeHTTP(restartReplay, request)
	if restartReplay.Code != http.StatusOK || mailer.calls != 1 {
		t.Fatalf("restart replay = %d calls=%d, want 200/1", restartReplay.Code, mailer.calls)
	}

	events := decodeAuditEvents(t, logs.String())
	assertAuditEvent(t, events[0], "rejected", "AUTHENTICATION_FAILED", 2, 1)
	assertAuditEvent(t, events[1], "succeeded", "NONE", 2, 1)
	assertAuditEvent(t, events[2], "replayed", "NONE", 2, 1)
	assertAuditEvent(t, events[3], "replayed", "NONE", 3, 2)
	for _, secret := range []string{"student@henu.edu.cn", "123456", "wrong-token", "provider-token-at-least-32-characters", "verification:job-001"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("audit log leaked %q: %s", secret, logs.String())
		}
	}
}

func TestProviderPromotesAcceptedMarkerAcrossMessageIDDomainRotation(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "ledger")
	mailer := &captureMailer{}
	oldProvider, err := newTestProviderWithDomain(ledger, mailer, &bytes.Buffer{}, "superhuazai.me")
	if err != nil {
		t.Fatalf("create old provider: %v", err)
	}
	if response := sendValidRequest(oldProvider, "req_rotation_001", 1); response.Code != http.StatusOK || mailer.calls != 1 {
		t.Fatalf("old provider send = %d calls=%d, want 200/1", response.Code, mailer.calls)
	}

	accepted, err := filepath.Glob(filepath.Join(ledger, "*.accepted.json"))
	if err != nil || len(accepted) != 1 {
		t.Fatalf("accepted markers = %v err=%v, want one", accepted, err)
	}
	pending := strings.TrimSuffix(accepted[0], ".accepted.json") + ".pending"
	if err := os.Rename(accepted[0], pending); err != nil {
		t.Fatalf("simulate post-send pre-rename crash: %v", err)
	}

	rotated, err := newTestProviderWithDomain(ledger, mailer, &bytes.Buffer{}, "notify.henukit.cn")
	if err != nil {
		t.Fatalf("create rotated provider: %v", err)
	}
	replay := sendValidRequest(rotated, "req_rotation_001", 2)
	if replay.Code != http.StatusOK || mailer.calls != 1 {
		t.Fatalf("rotated replay = %d calls=%d, want 200/1", replay.Code, mailer.calls)
	}
	if !strings.Contains(replay.Body.String(), "@superhuazai.me") {
		t.Fatalf("rotated replay must preserve accepted provider message ID, got %q", replay.Body.String())
	}
}

func TestNewRejectsInvalidMessageIDDomains(t *testing.T) {
	for _, domain := range []string{"", "a..b", "-notify.henukit.cn", "notify-.henukit.cn", strings.Repeat("a", 64) + ".cn"} {
		t.Run(domain, func(t *testing.T) {
			_, err := newTestProviderWithDomain(filepath.Join(t.TempDir(), "ledger"), &captureMailer{}, &bytes.Buffer{}, domain)
			if err == nil {
				t.Fatalf("New accepted invalid message ID domain %q", domain)
			}
		})
	}
}

type captureDigestMailer struct {
	calls int
	mails []Mail
}

func (mailer *captureDigestMailer) Send(_ context.Context, message Mail) error {
	mailer.calls++
	mailer.mails = append(mailer.mails, message)
	if message.Recipient != "student@henu.edu.cn" || message.Digest == nil || message.MessageID == "" {
		return context.Canceled
	}
	return nil
}

func TestProviderAcceptsCareerDigestTemplate(t *testing.T) {
	mailer := &captureDigestMailer{}
	handler, err := newTestProvider(filepath.Join(t.TempDir(), "ledger"), mailer, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	body := []byte(`{"recipient":"student@henu.edu.cn","template":"henukit_career_digest","variables":{"search_id":"search-001","completed_at":"2026-08-15T06:30:00Z","source_count":2,"job_count":3,"matched_count":1,"summary":"已扫描 2 个来源，发现 3 个岗位，1 个推荐","career_url":"https://portal.henukit.cn/career?search=search-001","top_jobs":[{"company":"测试公司","title":"后端开发实习生","location":"郑州","url":"https://example.test/jobs/1","match_score":90,"match_reasons":["匹配目标岗位 后端开发"]}]},"request_id":"req_digest_001","idempotency_key":"career_search_completed:search-001"}`)
	request := httptest.NewRequest(http.MethodPost, "/internal/send", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer provider-token-at-least-32-characters")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "career_search_completed:search-001")
	request.Header.Set("X-Request-ID", "req_digest_001")
	request.Header.Set("X-Mail-Attempt", "1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("career digest = %d body=%q, want 200", response.Code, response.Body.String())
	}
	if mailer.calls != 1 {
		t.Fatalf("career digest sent %d messages, want 1", mailer.calls)
	}
	delivered := mailer.mails[0]
	if delivered.Digest == nil || delivered.Digest.SearchID != "search-001" || delivered.Digest.SourceCount != 2 {
		t.Fatalf("mailer received no digest payload: %+v", delivered)
	}
	if delivered.Code != "" || delivered.Purpose != "" {
		t.Fatalf("career digest mail carried verification fields: %+v", delivered)
	}
}

func TestProviderRejectsCareerDigestWithVerificationFields(t *testing.T) {
	mailer := &captureMailer{}
	handler, err := newTestProvider(filepath.Join(t.TempDir(), "ledger"), mailer, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	body := []byte(`{"recipient":"student@henu.edu.cn","template":"henukit_career_digest","variables":{"code":"123456"},"request_id":"req_digest_bad","idempotency_key":"career_search_completed:search-bad"}`)
	request := httptest.NewRequest(http.MethodPost, "/internal/send", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer provider-token-at-least-32-characters")
	request.Header.Set("Idempotency-Key", "career_search_completed:search-bad")
	request.Header.Set("X-Request-ID", "req_digest_bad")
	request.Header.Set("X-Mail-Attempt", "1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("incomplete digest = %d, want 400", response.Code)
	}
	if mailer.calls != 0 {
		t.Fatalf("invalid digest sent %d messages, want 0", mailer.calls)
	}
}

func TestProviderAcceptsRegisterPurpose(t *testing.T) {
	mailer := &captureMailer{}
	handler, err := newTestProvider(filepath.Join(t.TempDir(), "ledger"), mailer, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	body := []byte(`{"recipient":"student@henu.edu.cn","template":"henukit_verification_code","variables":{"code":"123456","purpose":"register","expires_at":"2099-07-22T12:00:00Z"},"request_id":"req_register_001","idempotency_key":"verification:job-register-001"}`)
	request := httptest.NewRequest(http.MethodPost, "/internal/send", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer provider-token-at-least-32-characters")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "verification:job-register-001")
	request.Header.Set("X-Request-ID", "req_register_001")
	request.Header.Set("X-Mail-Attempt", "1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("register purpose = %d body=%q, want 200 (not invalid_request)", response.Code, response.Body.String())
	}
	if mailer.calls != 1 {
		t.Fatalf("register purpose sent %d messages, want 1", mailer.calls)
	}
	var accepted map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil || accepted["message_id"] == "" {
		t.Fatalf("register purpose response = %q, want accepted message_id", response.Body.String())
	}
}

func TestProviderAuditsInvalidRequestWithoutSensitiveContent(t *testing.T) {
	var logs bytes.Buffer
	handler, err := newTestProvider(filepath.Join(t.TempDir(), "ledger"), &captureMailer{}, &logs)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	body := `{"recipient":"secret@henu.edu.cn","variables":{"code":"654321"},"request_id":"req_bad_001","password":"do-not-log"}`
	request := httptest.NewRequest(http.MethodPost, "/internal/send", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer provider-token-at-least-32-characters")
	request.Header.Set("X-Request-ID", "req_bad_001")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid request = %d, want 400", response.Code)
	}
	events := decodeAuditEvents(t, logs.String())
	assertAuditEvent(t, events[0], "rejected", "INVALID_REQUEST", 1, 0)
	if strings.Contains(logs.String(), "secret@henu.edu.cn") || strings.Contains(logs.String(), "654321") || strings.Contains(logs.String(), "do-not-log") {
		t.Fatalf("invalid request audit leaked body: %s", logs.String())
	}
}

func TestProviderClassifiesAndAuditsSMTPFailures(t *testing.T) {
	for _, test := range []struct {
		name, result, code string
		smtpCode           int
		status             int
	}{
		{name: "temporary", smtpCode: 451, status: http.StatusServiceUnavailable, result: "retry", code: "SMTP_TEMPORARY_FAILURE"},
		{name: "permanent", smtpCode: 550, status: http.StatusUnprocessableEntity, result: "failed", code: "SMTP_PERMANENT_FAILURE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			mailer := &captureMailer{err: &textproto.Error{Code: test.smtpCode, Msg: "server included secret@henu.edu.cn and 123456"}}
			handler, err := newTestProvider(filepath.Join(t.TempDir(), "ledger"), mailer, &logs)
			if err != nil {
				t.Fatalf("create provider: %v", err)
			}
			response := sendValidRequest(handler, "req_failure_001", 4)
			if response.Code != test.status {
				t.Fatalf("SMTP failure = %d, want %d", response.Code, test.status)
			}
			events := decodeAuditEvents(t, logs.String())
			assertAuditEvent(t, events[0], test.result, test.code, 4, 3)
			if strings.Contains(logs.String(), "secret@henu.edu.cn") || strings.Contains(logs.String(), "123456") {
				t.Fatalf("SMTP failure audit leaked raw error: %s", logs.String())
			}
		})
	}
}

func TestProviderRestartDoesNotResendAfterAcceptedLedgerPersistenceFails(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "ledger")
	mailer := &captureMailer{}
	var logs bytes.Buffer
	handler, err := newTestProvider(ledger, mailer, &logs)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	handler.persistAccepted = func(_, _ string, _ []byte) error {
		return errors.New("simulated ledger failure with secret@henu.edu.cn 123456")
	}
	first := sendValidRequest(handler, "req_ledger_001", 1)
	if first.Code != http.StatusServiceUnavailable || mailer.calls != 1 {
		t.Fatalf("ledger failure = %d calls=%d, want 503/1", first.Code, mailer.calls)
	}

	restarted, err := newTestProvider(ledger, mailer, &logs)
	if err != nil {
		t.Fatalf("restart provider: %v", err)
	}
	replay := sendValidRequest(restarted, "req_ledger_001", 2)
	if replay.Code != http.StatusConflict || mailer.calls != 1 {
		t.Fatalf("unknown-state replay = %d calls=%d, want 409/1", replay.Code, mailer.calls)
	}
	events := decodeAuditEvents(t, logs.String())
	assertAuditEvent(t, events[0], "retry", "LEDGER_UNAVAILABLE", 1, 0)
	assertAuditEvent(t, events[1], "retry", "DELIVERY_IN_PROGRESS", 2, 1)
	if strings.Contains(logs.String(), "secret@henu.edu.cn") || strings.Contains(logs.String(), "123456") {
		t.Fatalf("ledger failure audit leaked raw error: %s", logs.String())
	}
}

func TestProviderDoesNotResendWhenAcceptedLedgerIsTemporarilyUnreadable(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "ledger")
	mailer := &captureMailer{}
	var logs bytes.Buffer
	handler, err := newTestProvider(ledger, mailer, &logs)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if response := sendValidRequest(handler, "req_unreadable_001", 1); response.Code != http.StatusOK {
		t.Fatalf("initial send = %d, want 200", response.Code)
	}

	restarted, err := newTestProvider(ledger, mailer, &logs)
	if err != nil {
		t.Fatalf("restart provider: %v", err)
	}
	restarted.readAccepted = func(string) ([]byte, error) {
		return nil, errors.New("simulated I/O error with secret@henu.edu.cn 123456")
	}
	replay := sendValidRequest(restarted, "req_unreadable_001", 2)
	if replay.Code != http.StatusServiceUnavailable || mailer.calls != 1 {
		t.Fatalf("unreadable accepted replay = %d calls=%d, want 503/1", replay.Code, mailer.calls)
	}
	events := decodeAuditEvents(t, logs.String())
	assertAuditEvent(t, events[1], "retry", "LEDGER_UNAVAILABLE", 2, 1)
	if strings.Contains(logs.String(), "secret@henu.edu.cn") || strings.Contains(logs.String(), "123456") {
		t.Fatalf("accepted-ledger audit leaked raw error: %s", logs.String())
	}
}

func newTestProvider(ledger string, mailer Mailer, logs *bytes.Buffer) (*Provider, error) {
	return newTestProviderWithDomain(ledger, mailer, logs, "notify.henukit.cn")
}

func newTestProviderWithDomain(ledger string, mailer Mailer, logs *bytes.Buffer, domain string) (*Provider, error) {
	return New(Config{
		Token: "provider-token-at-least-32-characters", LedgerDirectory: ledger, Mailer: mailer,
		Logger: slog.New(slog.NewJSONHandler(logs, nil)), ProviderID: "local-smtp", KeyID: "mail-provider-active",
		MessageIDDomain: domain,
	})
}

func sendValidRequest(handler http.Handler, requestID string, attempt int) *httptest.ResponseRecorder {
	body := strings.Replace(`{"recipient":"student@henu.edu.cn","template":"henukit_verification_code","variables":{"code":"123456","purpose":"login","expires_at":"2099-07-22T12:00:00Z"},"request_id":"REQUEST_ID","idempotency_key":"verification:job-001"}`, "REQUEST_ID", requestID, 1)
	request := httptest.NewRequest(http.MethodPost, "/internal/send", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer provider-token-at-least-32-characters")
	request.Header.Set("Idempotency-Key", "verification:job-001")
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("X-Mail-Attempt", strconv.Itoa(attempt))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeAuditEvents(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var events []map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	for {
		var event map[string]any
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatalf("decode audit event: %v\n%s", err, raw)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		t.Fatal(errors.New("no SMTP provider audit events"))
	}
	return events
}

func assertAuditEvent(t *testing.T, event map[string]any, result, code string, attempt, retry int) {
	t.Helper()
	for key, want := range map[string]any{
		"msg": "smtp_provider_delivery", "result": result, "error_code": code,
		"provider_id": "local-smtp", "key_id": "mail-provider-active",
		"attempt_count": float64(attempt), "retry_count": float64(retry),
	} {
		if event[key] != want {
			t.Errorf("audit %s = %#v, want %#v (event=%#v)", key, event[key], want, event)
		}
	}
	if _, ok := event["request_id"]; !ok {
		t.Errorf("audit event lacks request_id: %#v", event)
	}
	if _, ok := event["duration_ms"]; !ok {
		t.Errorf("audit event lacks duration_ms: %#v", event)
	}
}
