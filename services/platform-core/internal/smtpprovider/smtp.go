package smtpprovider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type SMTPMailer struct {
	address, host, username, password, from string
	timeout                                 time.Duration
}

func NewSMTPMailer(address, username, password, from string, timeout time.Duration) (*SMTPMailer, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" || username == "" || password == "" || from == "" || timeout <= 0 || strings.ContainsAny(from, "\r\n") {
		return nil, errors.New("SMTP address, credentials, sender, and timeout are required")
	}
	return &SMTPMailer{address: address, host: host, username: username, password: password, from: from, timeout: timeout}, nil
}

func (mailer *SMTPMailer) Send(ctx context.Context, message Mail) error {
	dialer := net.Dialer{Timeout: mailer.timeout}
	connection, err := dialer.DialContext(ctx, "tcp", mailer.address)
	if err != nil {
		return err
	}
	defer connection.Close()
	client, err := smtp.NewClient(connection, mailer.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not support STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: mailer.host}); err != nil {
		return err
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
	subject := "HENU Kit 登录验证码"
	body := fmt.Sprintf("你的 HENU Kit 验证码是：%s\r\n\r\n验证码将在 %s 过期。如非本人操作，请忽略此邮件。\r\n", message.Code, message.ExpiresAt.In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05 MST"))
	content := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s", mailer.from, message.Recipient, subject, message.MessageID, time.Now().UTC().Format(time.RFC1123Z), body)
	if _, err := writer.Write([]byte(content)); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
