package mailworker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
