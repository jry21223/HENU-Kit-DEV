package smtpprovider

import (
	"testing"
	"time"
)

func TestNewSMTPMailerImplicitTLSFor465(t *testing.T) {
	mailer, err := NewSMTPMailer("smtpdm.aliyun.com:465", "noreply@example.com", "secret", "noreply@example.com", 10*time.Second)
	if err != nil {
		t.Fatalf("create mailer: %v", err)
	}
	if !mailer.implicitTLS {
		t.Fatal("port 465 must use implicit TLS")
	}
	startTLSMailer, err := NewSMTPMailer("smtp.example.com:587", "noreply@example.com", "secret", "noreply@example.com", 10*time.Second)
	if err != nil {
		t.Fatalf("create starttls mailer: %v", err)
	}
	if startTLSMailer.implicitTLS {
		t.Fatal("port 587 must use STARTTLS")
	}
}
