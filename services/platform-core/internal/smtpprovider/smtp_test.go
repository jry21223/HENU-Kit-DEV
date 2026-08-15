package smtpprovider

import (
	"strings"
	"testing"
	"time"

	"henukit.dev/platform-core/internal/verificationmail"
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

func TestBuildCareerDigestMIMEIsBrowserSafe(t *testing.T) {
	digest := &verificationmail.CareerDigest{
		SearchID: "search-001", CompletedAt: "2026-08-15T06:30:00Z",
		SourceCount: 2, JobCount: 3, MatchedCount: 1,
		Summary:   "已扫描 2 个来源，发现 3 个岗位，1 个推荐",
		CareerURL: "https://portal.henukit.cn/career?search=search-001",
		TopJobs: []verificationmail.Job{{
			Company: "<script>测试公司</script>", Title: "后端开发实习生", Location: "郑州",
			URL: "javascript:alert(1)", MatchScore: 90,
			MatchReasons: []string{"匹配目标岗位 后端开发"},
		}, {
			Company: "内推科技", Title: "前端实习生", Location: "远程",
			URL: "https://example.test/jobs/2", MatchScore: 70,
			MatchReasons: nil,
		}},
	}
	content, err := buildCareerDigestMIME("noreply@example.com", Mail{
		Recipient: "student@henu.edu.cn",
		Digest:    digest,
		MessageID: "digest@example.com",
	}, time.Date(2026, 8, 15, 6, 45, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build career digest MIME: %v", err)
	}
	for _, expected := range []string{
		"Content-Type: multipart/alternative; boundary=",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Type: text/html; charset=UTF-8",
		"Subject: =?UTF-8?",
		`From: "HENU Kit" <noreply@example.com>`,
		"求职雷达 · 扫描结果",
		"扫描来源：2 个 · 发现岗位：3 个 · 推荐：1 个",
		"2026-08-15 14:30 Asia/Shanghai（北京时间）",
		"以招聘官网为准",
		"学生自主运营 · 非河南大学官方项目",
		"1. 后端开发实习生 · <script>测试公司</script>",
		"2. 前端实习生 · 内推科技",
		"匹配 90 分",
		"https://example.test/jobs/2",
		"career?search=search-001",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("digest MIME omitted %q: %s", expected, content)
		}
	}
	// The HTML part must escape user content; no javascript: link may be emitted.
	htmlIndex := strings.Index(content, "Content-Type: text/html")
	if htmlIndex < 0 || !strings.Contains(content[htmlIndex:], "&lt;script&gt;测试公司&lt;/script&gt;") || strings.Contains(content[htmlIndex:], "<script>") {
		t.Fatal("digest HTML contains unescaped user content")
	}
	if strings.Contains(content, "javascript:") {
		t.Fatal("digest MIME leaked a javascript: URL as a link")
	}
	// Internal crawler facts and raw profile text must never appear.
	for _, internal := range []string{"source_key", "description", "requirements", "job_type", "profile_snapshot", "getwork"} {
		if strings.Contains(content, internal) {
			t.Fatalf("digest MIME leaked internal detail %q: %s", internal, content)
		}
	}
	if strings.Contains(content, "CAREER / DIGEST") == false {
		t.Fatal("digest MIME must carry the CAREER / DIGEST brand label")
	}
}

func TestBuildCareerDigestMIMERequiresDigest(t *testing.T) {
	_, err := buildCareerDigestMIME("noreply@example.com", Mail{
		Recipient: "student@henu.edu.cn", MessageID: "digest@example.com",
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("career digest MIME without a digest must be rejected")
	}
}
