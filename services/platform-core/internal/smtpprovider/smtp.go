package smtpprovider

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
	_ "time/tzdata"

	"henukit.dev/platform-core/internal/verificationmail"
)

type SMTPMailer struct {
	address, host, username, password, from string
	timeout                                 time.Duration
	implicitTLS                             bool
}

func NewSMTPMailer(address, username, password, from string, timeout time.Duration) (*SMTPMailer, error) {
	host, port, err := net.SplitHostPort(address)
	sender, senderErr := mail.ParseAddress(from)
	if err != nil || host == "" || username == "" || password == "" || senderErr != nil || sender.Address != from || timeout <= 0 || strings.ContainsAny(from, "\r\n") {
		return nil, errors.New("SMTP address, credentials, sender, and timeout are required")
	}
	// Port 465 is SMTPS (implicit TLS). Port 587/25 typically use STARTTLS.
	implicitTLS := port == "465"
	return &SMTPMailer{address: address, host: host, username: username, password: password, from: from, timeout: timeout, implicitTLS: implicitTLS}, nil
}

func (mailer *SMTPMailer) Send(ctx context.Context, message Mail) error {
	dialer := net.Dialer{Timeout: mailer.timeout}
	var (
		connection net.Conn
		err        error
	)
	if mailer.implicitTLS {
		tlsDialer := &tls.Dialer{NetDialer: &dialer, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: mailer.host}}
		connection, err = tlsDialer.DialContext(ctx, "tcp", mailer.address)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", mailer.address)
	}
	if err != nil {
		return err
	}
	defer connection.Close()
	client, err := smtp.NewClient(connection, mailer.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if !mailer.implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: mailer.host}); err != nil {
			return err
		}
	}
	if err := client.Auth(smtp.PlainAuth("", mailer.username, mailer.password, mailer.host)); err != nil {
		return err
	}
	if err := client.Mail(mailer.from); err != nil {
		return err
	}
	if err := client.Rcpt(message.Recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	content, err := buildMailMIME(mailer.from, message, time.Now().UTC())
	if err != nil {
		_ = writer.Close()
		return err
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	// A successful DATA close means the SMTP server accepted responsibility for
	// the message. A later QUIT transport error must not trigger a duplicate send.
	_ = client.Quit()
	return nil
}

const verificationSubject = "HENU Kit 邮箱验证码"
const careerDigestSubject = "HENU Kit 求职雷达 · 扫描结果"

// buildMailMIME renders the registered template for one mail kind. The
// verification_code path stays byte-identical to buildVerificationMIME; the
// career_digest path renders only the browser-safe digest summary.
func buildMailMIME(from string, message Mail, now time.Time) (string, error) {
	if message.Digest != nil {
		return buildCareerDigestMIME(from, message, now)
	}
	return buildVerificationMIME(from, message, now)
}

func buildVerificationMIME(from string, message Mail, now time.Time) (string, error) {
	if strings.ContainsAny(from, "\r\n") ||
		strings.ContainsAny(message.Recipient, "\r\n") ||
		strings.ContainsAny(message.MessageID, "\r\n") {
		return "", errors.New("mail headers must not contain CR or LF")
	}
	expiresText, err := formatVerificationExpiry(message.ExpiresAt)
	if err != nil {
		return "", err
	}
	plain := fmt.Sprintf(
		"你的 HENU Kit 验证码是：%s\r\n\r\n验证码将在 %s 过期。如非本人操作，请忽略此邮件。\r\n\r\n学生自主运营 · 非河南大学官方项目\r\n",
		message.Code,
		expiresText,
	)
	htmlBody := buildVerificationHTML(message.Code, expiresText)
	boundary, err := mimeBoundary()
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.Grow(len(plain) + len(htmlBody) + 512)
	fromHeader := (&mail.Address{Name: "HENU Kit", Address: from}).String()
	fmt.Fprintf(&builder, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: %s\r\n", fromHeader, message.Recipient, mime.BEncoding.Encode("UTF-8", verificationSubject), message.MessageID, now.Format(time.RFC1123Z))
	fmt.Fprintf(&builder, "MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&builder, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, plain)
	fmt.Fprintf(&builder, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, htmlBody)
	fmt.Fprintf(&builder, "--%s--\r\n", boundary)
	return builder.String(), nil
}

func formatVerificationExpiry(expiresAt time.Time) (string, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return "", fmt.Errorf("load Asia/Shanghai timezone: %w", err)
	}
	return expiresAt.In(location).Format("2006-01-02 15:04:05") + " Asia/Shanghai（北京时间）", nil
}

func buildVerificationHTML(code, expiresText string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HENU Kit 邮箱验证码</title></head>
<body style="margin:0;padding:0;background:#f2f0ea;color:#161513;-webkit-text-size-adjust:100%%;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="background:#f2f0ea;padding:28px 12px;">
<tr><td align="center">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="max-width:520px;background:#f2f0ea;border:1px solid #161513;">
<tr><td style="padding:18px 22px;border-bottom:1px solid #161513;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;">
<span style="font-size:18px;font-weight:800;letter-spacing:.04em;">henukit®</span>
<span style="float:right;color:#ff4d00;font-family:Consolas,'Courier New',monospace;font-size:12px;letter-spacing:.18em;">ACCOUNT / VERIFY</span>
</td></tr>
<tr><td style="padding:30px 22px 10px;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;font-size:15px;line-height:1.7;">
你正在完成 HENU Kit 账号验证。请使用以下验证码：
</td></tr>
<tr><td style="padding:12px 22px 18px;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="border:1px solid #161513;background:#ffffff;">
<tr><td align="center" style="padding:20px 12px;color:#161513;font-family:Consolas,'SFMono-Regular','Courier New',monospace;font-size:34px;font-weight:800;letter-spacing:10px;">%s</td></tr>
</table>
</td></tr>
<tr><td style="padding:0 22px 26px;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;font-size:13px;line-height:1.7;color:#57534e;">
有效期至 <strong style="color:#161513;">%s</strong>。<br>如非本人操作，请忽略此邮件，切勿向他人透露验证码。
</td></tr>
<tr><td style="padding:14px 22px;border-top:4px solid #ff4d00;font-family:Consolas,'Courier New',monospace;font-size:11px;line-height:1.6;color:#57534e;">
HENU — STUDENT PLATFORM<br>学生自主运营 · 非河南大学官方项目
</td></tr>
</table>
</td></tr></table>
</body></html>`, html.EscapeString(code), html.EscapeString(expiresText))
}

func mimeBoundary() (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "henukit_" + hex.EncodeToString(value[:]), nil
}

// buildCareerDigestMIME renders the #397 Opportunity Digest: a bounded summary
// of one completed Career search. It carries no internal service facts, no raw
// profile text, and no source implementation detail; every user-supplied field
// is HTML-escaped and every link is restricted to http(s).
func buildCareerDigestMIME(from string, message Mail, now time.Time) (string, error) {
	if message.Digest == nil ||
		strings.ContainsAny(from, "\r\n") ||
		strings.ContainsAny(message.Recipient, "\r\n") ||
		strings.ContainsAny(message.MessageID, "\r\n") {
		return "", errors.New("career digest mail headers must not contain CR or LF")
	}
	digest := message.Digest
	completedText, err := formatCareerDigestCompleted(digest.CompletedAt)
	if err != nil {
		return "", err
	}
	plain := buildCareerDigestPlain(digest, completedText)
	htmlBody := buildCareerDigestHTML(digest, completedText)
	boundary, err := mimeBoundary()
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.Grow(len(plain) + len(htmlBody) + 512)
	fromHeader := (&mail.Address{Name: "HENU Kit", Address: from}).String()
	fmt.Fprintf(&builder, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: %s\r\n", fromHeader, message.Recipient, mime.BEncoding.Encode("UTF-8", careerDigestSubject), message.MessageID, now.Format(time.RFC1123Z))
	fmt.Fprintf(&builder, "MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&builder, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, plain)
	fmt.Fprintf(&builder, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, htmlBody)
	fmt.Fprintf(&builder, "--%s--\r\n", boundary)
	return builder.String(), nil
}

func formatCareerDigestCompleted(value string) (string, error) {
	completedAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", errors.New("career digest completed time is invalid")
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return "", fmt.Errorf("load Asia/Shanghai timezone: %w", err)
	}
	return completedAt.In(location).Format("2006-01-02 15:04") + " Asia/Shanghai（北京时间）", nil
}

func buildCareerDigestPlain(digest *verificationmail.CareerDigest, completedText string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "你的 HENU Kit 求职雷达扫描已完成。\r\n\r\n")
	fmt.Fprintf(&builder, "扫描时间：%s\r\n", completedText)
	fmt.Fprintf(&builder, "扫描来源：%d 个 · 发现岗位：%d 个 · 推荐：%d 个\r\n\r\n", digest.SourceCount, digest.JobCount, digest.MatchedCount)
	if digest.Summary != "" {
		fmt.Fprintf(&builder, "%s\r\n\r\n", digest.Summary)
	}
	if len(digest.TopJobs) > 0 {
		builder.WriteString("热门推荐：\r\n")
		for index, job := range digest.TopJobs {
			fmt.Fprintf(&builder, "%d. %s · %s（%s）匹配 %d 分\r\n", index+1, job.Title, job.Company, job.Location, job.MatchScore)
			if len(job.MatchReasons) > 0 {
				fmt.Fprintf(&builder, "   匹配原因：%s\r\n", strings.Join(job.MatchReasons, "；"))
			}
			if job.URL != "" && validWebURL(job.URL) {
				fmt.Fprintf(&builder, "   岗位链接：%s\r\n", job.URL)
			}
		}
		builder.WriteString("\r\n")
	}
	if digest.CareerURL != "" {
		fmt.Fprintf(&builder, "查看全部结果：%s\r\n\r\n", digest.CareerURL)
	}
	builder.WriteString("岗位信息以招聘官网为准。\r\n\r\n学生自主运营 · 非河南大学官方项目\r\n")
	return builder.String()
}

func buildCareerDigestHTML(digest *verificationmail.CareerDigest, completedText string) string {
	summary := html.EscapeString(digest.Summary)
	var jobs strings.Builder
	for index, job := range digest.TopJobs {
		title := html.EscapeString(job.Title)
		company := html.EscapeString(job.Company)
		location := html.EscapeString(job.Location)
		reasons := make([]string, 0, len(job.MatchReasons))
		for _, reason := range job.MatchReasons {
			reasons = append(reasons, html.EscapeString(reason))
		}
		fmt.Fprintf(&jobs, `<tr><td style="padding:14px 22px;border-bottom:1px solid #e7e5df;">
<div style="font-size:15px;font-weight:800;color:#161513;">%d. %s</div>
<div style="font-size:13px;color:#57534e;margin-top:4px;">%s · %s</div>
<div style="font-size:12px;color:#ff4d00;margin-top:4px;">匹配 %d 分</div>
%s%s</td></tr>`, index+1, title, company, location, job.MatchScore, renderDigestReasons(reasons), renderDigestJobLink(job.URL, title))
	}
	careerLink := ""
	if digest.CareerURL != "" {
		careerLink = fmt.Sprintf(`<tr><td style="padding:16px 22px;"><table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0"><tr><td align="center" style="border:1px solid #161513;background:#161513;"><a href="%s" style="display:block;padding:12px 20px;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;">查看全部结果</a></td></tr></table></td></tr>`, html.EscapeString(digest.CareerURL))
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HENU Kit 求职雷达 · 扫描结果</title></head>
<body style="margin:0;padding:0;background:#f2f0ea;color:#161513;-webkit-text-size-adjust:100%%;">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="background:#f2f0ea;padding:28px 12px;">
<tr><td align="center">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" border="0" style="max-width:520px;background:#f2f0ea;border:1px solid #161513;">
<tr><td style="padding:18px 22px;border-bottom:1px solid #161513;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;">
<span style="font-size:18px;font-weight:800;letter-spacing:.04em;">henukit®</span>
<span style="float:right;color:#ff4d00;font-family:Consolas,'Courier New',monospace;font-size:12px;letter-spacing:.18em;">CAREER / DIGEST</span>
</td></tr>
<tr><td style="padding:26px 22px 6px;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;font-size:15px;line-height:1.7;">
你的 HENU Kit 求职雷达扫描已完成。
</td></tr>
<tr><td style="padding:6px 22px 8px;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;font-size:13px;line-height:1.7;color:#57534e;">
扫描时间：<strong style="color:#161513;">%s</strong><br>
扫描来源：<strong style="color:#161513;">%d</strong> 个 · 发现岗位：<strong style="color:#161513;">%d</strong> 个 · 推荐：<strong style="color:#161513;">%d</strong> 个
</td></tr>
%s
<tr><td style="padding:0 22px 14px;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;font-size:13px;line-height:1.7;color:#57534e;">%s</td></tr>
%s
<tr><td style="padding:0 22px 22px;font-family:Arial,'PingFang SC','Microsoft YaHei',sans-serif;font-size:12px;line-height:1.6;color:#57534e;">岗位信息以招聘官网为准。</td></tr>
<tr><td style="padding:14px 22px;border-top:4px solid #ff4d00;font-family:Consolas,'Courier New',monospace;font-size:11px;line-height:1.6;color:#57534e;">
HENU — STUDENT PLATFORM<br>学生自主运营 · 非河南大学官方项目
</td></tr>
</table>
</td></tr></table>
</body></html>`, completedText, digest.SourceCount, digest.JobCount, digest.MatchedCount, jobs.String(), summary, careerLink)
}

func renderDigestReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return `<div style="font-size:12px;color:#57534e;margin-top:4px;">匹配原因：` + strings.Join(reasons, "；") + `</div>`
}

func renderDigestJobLink(rawURL, escapedTitle string) string {
	if rawURL == "" || !validWebURL(rawURL) {
		return ""
	}
	return fmt.Sprintf(`<div style="font-size:12px;margin-top:4px;"><a href="%s" style="color:#2457d6;">岗位链接：%s</a></div>`, html.EscapeString(rawURL), escapedTitle)
}
