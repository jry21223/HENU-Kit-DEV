package mailprovider

import (
	"context"
	"errors"
	"testing"
)

func TestFakeOnlyReportsAccepted(t *testing.T) {
	provider := &Fake{}
	result, err := provider.Send(context.Background(), Message{To: "student@example.edu", Subject: "通知", Text: "正文"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusAccepted || len(provider.Messages) != 1 {
		t.Fatalf("unexpected fake result: %#v messages=%d", result, len(provider.Messages))
	}
}

func TestFakePropagatesProviderFailure(t *testing.T) {
	want := errors.New("SMTP unavailable")
	provider := &Fake{Err: want}
	if _, err := provider.Send(context.Background(), Message{}); !errors.Is(err, want) {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestSMTPValidatesConfiguration(t *testing.T) {
	if _, err := NewSMTP(SMTPConfig{}); err == nil {
		t.Fatal("expected incomplete SMTP configuration to fail")
	}
	if _, err := NewSMTP(SMTPConfig{Host: "smtp.example.edu", Port: "587", From: "bad\nfrom"}); err == nil {
		t.Fatal("expected invalid from address to fail")
	}
}
