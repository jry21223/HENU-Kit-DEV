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
	"net/smtp"
	"strings"
	"time"
)

type SMTPMailer struct {
	address, host, username, password, from string
	timeout                                 time.Duration
	implicitTLS                             bool
}

func NewSMTPMailer(address, username, password, from string, timeout time.Duration) (*SMTPMailer, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || username == "" || password == "" || from == "" || timeout <= 0 || strings.ContainsAny(from, "\r\n") {
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
	content, err := buildVerificationMIME(mailer.from, message, time.Now().UTC())
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

func buildVerificationMIME(from string, message Mail, now time.Time) (string, error) {
	if strings.ContainsAny(from, "\r\n") ||
		strings.ContainsAny(message.Recipient, "\r\n") ||
		strings.ContainsAny(message.MessageID, "\r\n") {
		return "", errors.New("mail headers must not contain CR or LF")
	}
	expiresText := message.ExpiresAt.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05 MST")
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
	fmt.Fprintf(&builder, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: %s\r\n", from, message.Recipient, mime.BEncoding.Encode("UTF-8", verificationSubject), message.MessageID, now.Format(time.RFC1123Z))
	fmt.Fprintf(&builder, "MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary)
	fmt.Fprintf(&builder, "--%s\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, plain)
	fmt.Fprintf(&builder, "--%s\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, htmlBody)
	fmt.Fprintf(&builder, "--%s--\r\n", boundary)
	return builder.String(), nil
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
