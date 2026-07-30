package smtpprovider

import (
	"strings"
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

func TestBuildVerificationMIMEUsesMultipartCurrentBrandAndEscapesHTML(t *testing.T) {
	content, err := buildVerificationMIME("noreply@example.com", Mail{
		Recipient: "student@henu.edu.cn",
		Code:      `12<script>3`,
		ExpiresAt: time.Date(2099, 7, 22, 12, 0, 0, 0, time.UTC),
		MessageID: "verification@example.com",
	}, time.Date(2026, 7, 26, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build verification MIME: %v", err)
	}
	for _, expected := range []string{
		"Content-Type: multipart/alternative; boundary=",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Type: text/html; charset=UTF-8",
		"Subject: =?UTF-8?",
		`From: "HENU Kit" <noreply@example.com>`,
		"12&lt;script&gt;3",
		"#f2f0ea",
		"#161513",
		"#ff4d00",
		"2099-07-22 20:00:00 Asia/Shanghai（北京时间）",
		"学生自主运营 · 非河南大学官方项目",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("MIME omitted %q: %s", expected, content)
		}
	}
	htmlIndex := strings.Index(content, "Content-Type: text/html")
	if htmlIndex < 0 || strings.Contains(content[htmlIndex:], "<script>") {
		t.Fatal("HTML part contains unescaped code")
	}
	if strings.Index(content, "Content-Type: text/plain") > strings.Index(content, "Content-Type: text/html") {
		t.Fatal("text/plain must precede text/html in multipart/alternative")
	}
}

func TestBuildVerificationMIMERejectsHeaderInjection(t *testing.T) {
	_, err := buildVerificationMIME("noreply@example.com", Mail{
		Recipient: "student@henu.edu.cn\r\nBcc: attacker@example.com",
		Code:      "123456",
		ExpiresAt: time.Now().Add(time.Minute),
		MessageID: "verification@example.com",
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected header injection rejection")
	}
}
