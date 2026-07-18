package mailprovider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

const StatusAccepted = "accepted"

type Message struct {
	To      string
	Subject string
	Text    string
}

type Result struct {
	Status     string
	AcceptedAt time.Time
}

type Provider interface {
	Send(context.Context, Message) (Result, error)
}

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

type SMTP struct{ config SMTPConfig }

func NewSMTP(config SMTPConfig) (SMTP, error) {
	if config.Host == "" || config.Port == "" || config.From == "" {
		return SMTP{}, errors.New("SMTP host, port and from are required")
	}
	if _, err := mail.ParseAddress(config.From); err != nil {
		return SMTP{}, fmt.Errorf("invalid SMTP from address: %w", err)
	}
	return SMTP{config: config}, nil
}

func (provider SMTP) Send(ctx context.Context, message Message) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil {
		return Result{}, fmt.Errorf("invalid recipient: %w", err)
	}
	if strings.ContainsAny(message.Subject, "\r\n") {
		return Result{}, errors.New("subject contains a header injection character")
	}
	from, _ := mail.ParseAddress(provider.config.From)
	raw := []byte("From: " + from.String() + "\r\n" +
		"To: " + to.String() + "\r\n" +
		"Subject: " + message.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + message.Text)
	var auth smtp.Auth
	if provider.config.Username != "" {
		auth = smtp.PlainAuth("", provider.config.Username, provider.config.Password, provider.config.Host)
	}
	if err := smtp.SendMail(net.JoinHostPort(provider.config.Host, provider.config.Port), auth, from.Address, []string{to.Address}, raw); err != nil {
		return Result{}, err
	}
	// SMTP acceptance is not proof of delivery. A DSN or provider callback must
	// transition the delivery record to delivered later.
	return Result{Status: StatusAccepted, AcceptedAt: time.Now().UTC()}, nil
}

type Fake struct {
	mu       sync.Mutex
	Messages []Message
	Err      error
}

func (provider *Fake) Send(ctx context.Context, message Message) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.Err != nil {
		return Result{}, provider.Err
	}
	provider.Messages = append(provider.Messages, message)
	return Result{Status: StatusAccepted, AcceptedAt: time.Now().UTC()}, nil
}
