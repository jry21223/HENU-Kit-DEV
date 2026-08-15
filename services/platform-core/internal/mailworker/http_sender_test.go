package mailworker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"henukit.dev/platform-core/internal/verificationmail"
)

func TestHTTPSenderMapsProviderAcceptance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Error("provider authorization header is missing")
		}
		if request.Header.Get("Idempotency-Key") != "verification:job-001" {
			t.Error("provider idempotency header is missing")
		}
		if request.Header.Get("X-Request-ID") != "req_sender_001" || request.Header.Get("X-Mail-Attempt") != "3" {
			t.Error("provider audit correlation headers are missing")
		}
		var payload struct {
			Recipient string `json:"recipient"`
			Template  string `json:"template"`
			Variables struct {
				Code string `json:"code"`
			} `json:"variables"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode provider payload: %v", err)
		}
		if payload.Recipient != "student@henu.edu.cn" || payload.Template != "henukit_verification_code" || payload.Variables.Code != "123456" {
			t.Errorf("unexpected provider payload: %+v", payload)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"message_id":"provider_accepted_001"}`))
	}))
	defer server.Close()
	sender, err := NewHTTPSender(server.URL, "provider-secret", server.Client())
	if err != nil {
		t.Fatalf("create HTTP sender: %v", err)
	}
	messageID, err := sender.Send(context.Background(), Message{
		IdempotencyKey: "verification:job-001",
		Recipient:      "student@henu.edu.cn", Code: "123456", Purpose: "login",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute), RequestID: "req_sender_001",
		AttemptCount: 3,
	})
	if err != nil || messageID != "provider_accepted_001" {
		t.Fatalf("provider acceptance = %q err=%v", messageID, err)
	}
}

func TestHTTPSenderClassifiesProviderRejection(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		permanent bool
	}{
		{name: "bad recipient", status: http.StatusBadRequest, permanent: true},
		{name: "delivery in progress", status: http.StatusConflict, permanent: false},
		{name: "rate limited", status: http.StatusTooManyRequests, permanent: false},
		{name: "provider outage", status: http.StatusServiceUnavailable, permanent: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
			}))
			defer server.Close()
			sender, _ := NewHTTPSender(server.URL, "provider-secret", server.Client())
			_, err := sender.Send(context.Background(), Message{Recipient: "student@henu.edu.cn", Code: "123456", Purpose: "login", ExpiresAt: time.Now(), RequestID: "req_sender_002"})
			var sendError *SendError
			if !errors.As(err, &sendError) || sendError.Permanent != test.permanent {
				t.Fatalf("provider error = %#v, want permanent=%v", err, test.permanent)
			}
		})
	}
}

func TestHTTPSenderSendsCareerDigestTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Error("provider authorization header is missing")
		}
		if request.Header.Get("Idempotency-Key") != "career_search_completed:search-001" {
			t.Error("provider idempotency header is missing")
		}
		if request.Header.Get("X-Request-ID") != "req_digest_001" || request.Header.Get("X-Mail-Attempt") != "1" {
			t.Error("provider audit correlation headers are missing")
		}
		var payload struct {
			Recipient string `json:"recipient"`
			Template  string `json:"template"`
			Variables struct {
				SearchID     string                 `json:"search_id"`
				CompletedAt  string                 `json:"completed_at"`
				SourceCount  int                    `json:"source_count"`
				JobCount     int                    `json:"job_count"`
				MatchedCount int                    `json:"matched_count"`
				Summary      string                 `json:"summary"`
				CareerURL    string                 `json:"career_url"`
				TopJobs      []verificationmail.Job `json:"top_jobs"`
				Code         string                 `json:"code"`
				Purpose      string                 `json:"purpose"`
				ExpiresAt    string                 `json:"expires_at"`
			} `json:"variables"`
			RequestID      string `json:"request_id"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode provider payload: %v", err)
		}
		if payload.Recipient != "student@henu.edu.cn" || payload.Template != "henukit_career_digest" {
			t.Errorf("unexpected provider payload: %+v", payload)
		}
		if payload.Variables.SearchID != "search-001" || payload.Variables.SourceCount != 2 ||
			payload.Variables.JobCount != 3 || payload.Variables.MatchedCount != 1 ||
			payload.Variables.Summary != "已扫描 2 个来源，发现 3 个岗位，1 个推荐" ||
			payload.Variables.CareerURL != "https://portal.henukit.cn/career?search=search-001" ||
			payload.Variables.CompletedAt == "" {
			t.Errorf("unexpected digest variables: %+v", payload.Variables)
		}
		if len(payload.Variables.TopJobs) != 1 || payload.Variables.TopJobs[0].Company != "测试公司" {
			t.Errorf("unexpected digest top jobs: %+v", payload.Variables.TopJobs)
		}
		// Verification-only fields must stay absent from digest payloads.
		if payload.Variables.Code != "" || payload.Variables.Purpose != "" || payload.Variables.ExpiresAt != "" {
			t.Errorf("digest payload leaked verification fields: %+v", payload.Variables)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"message_id":"provider_accepted_digest"}`))
	}))
	defer server.Close()
	sender, err := NewHTTPSender(server.URL, "provider-secret", server.Client())
	if err != nil {
		t.Fatalf("create HTTP sender: %v", err)
	}
	messageID, err := sender.Send(context.Background(), Message{
		IdempotencyKey: "career_search_completed:search-001",
		Recipient:      "student@henu.edu.cn",
		Template:       "henukit_career_digest",
		RequestID:      "req_digest_001",
		AttemptCount:   1,
		Digest: &verificationmail.CareerDigest{
			SearchID: "search-001", CompletedAt: "2026-08-15T06:30:00Z",
			SourceCount: 2, JobCount: 3, MatchedCount: 1,
			Summary:   "已扫描 2 个来源，发现 3 个岗位，1 个推荐",
			CareerURL: "https://portal.henukit.cn/career?search=search-001",
			TopJobs: []verificationmail.Job{{
				Company: "测试公司", Title: "后端开发实习生", Location: "郑州",
				URL: "https://example.test/jobs/1", MatchScore: 90,
				MatchReasons: []string{"匹配目标岗位 后端开发"},
			}},
		},
	})
	if err != nil || messageID != "provider_accepted_digest" {
		t.Fatalf("provider acceptance = %q err=%v", messageID, err)
	}
}

func TestHTTPSenderVerificationPayloadKeepsExactShape(t *testing.T) {
	var rawPayload string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body strings.Builder
		_, _ = io.Copy(&body, request.Body)
		rawPayload = body.String()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"message_id":"provider_accepted_002"}`))
	}))
	defer server.Close()
	sender, _ := NewHTTPSender(server.URL, "provider-secret", server.Client())
	expiresAt := time.Date(2099, 7, 22, 12, 0, 0, 0, time.UTC)
	_, err := sender.Send(context.Background(), Message{
		IdempotencyKey: "verification:job-002",
		Recipient:      "student@henu.edu.cn", Code: "123456", Purpose: "login",
		ExpiresAt: expiresAt, RequestID: "req_sender_002", AttemptCount: 1,
	})
	if err != nil {
		t.Fatalf("send verification: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	variables, ok := payload["variables"].(map[string]any)
	if !ok {
		t.Fatalf("payload variables missing: %s", rawPayload)
	}
	// The verification request body must carry exactly code/purpose/expires_at.
	if len(variables) != 3 {
		t.Fatalf("verification variables = %v, want exactly code/purpose/expires_at", variables)
	}
	for _, key := range []string{"code", "purpose", "expires_at"} {
		if _, ok := variables[key]; !ok {
			t.Fatalf("verification variables omitted %q: %s", key, rawPayload)
		}
	}
}

func TestNewHTTPSenderAllowsDockerComposeHTTP(t *testing.T) {
	t.Setenv("PLATFORM_CORE_SMTP_ALLOW_DOCKER", "1")
	sender, err := NewHTTPSender("http://platform-smtp-provider:18081/internal/send", "provider-secret", &http.Client{Timeout: time.Second})
	if err != nil || sender == nil {
		t.Fatalf("compose service HTTP should be allowed when PLATFORM_CORE_SMTP_ALLOW_DOCKER=1: %v", err)
	}

	t.Setenv("PLATFORM_CORE_SMTP_ALLOW_DOCKER", "")
	if _, err := NewHTTPSender("http://platform-smtp-provider:18081/internal/send", "provider-secret", &http.Client{Timeout: time.Second}); err == nil {
		t.Fatal("compose service HTTP must be rejected without PLATFORM_CORE_SMTP_ALLOW_DOCKER")
	}
	if _, err := NewHTTPSender("http://smtp.example.com/internal/send", "provider-secret", &http.Client{Timeout: time.Second}); err == nil {
		t.Fatal("public HTTP endpoints must stay rejected")
	}
}
