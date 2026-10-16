// Package mailer sends transactional email. Concrete implementations cover
// real SMTP delivery and a console fallback for local development and tests.
package mailer

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/smtp"
	"strings"

	"go-api-auth/internal/config"
)

// Message is a single outbound email.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Mailer delivers a Message.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// New returns an SMTP mailer when SMTP_HOST is configured, otherwise a
// console mailer that logs the message.
func New(cfg config.Config) Mailer {
	if cfg.SMTP.Host == "" {
		return Console{}
	}
	return SMTP{
		Host: cfg.SMTP.Host,
		Port: cfg.SMTP.Port,
		User: cfg.SMTP.User,
		Pass: cfg.SMTP.Pass,
		From: cfg.SMTP.From,
	}
}

// Console logs messages instead of sending them. It is the default when no
// SMTP host is configured, and is used by tests.
type Console struct{}

// Send logs the recipient, subject and body (including any reset link).
func (Console) Send(_ context.Context, msg Message) error {
	log.Printf("mailer: to=%s subject=%q body=%s", msg.To, msg.Subject, msg.Body)
	return nil
}

// SMTP sends mail through an SMTP server.
type SMTP struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// Send delivers msg via SMTP.
func (s SMTP) Send(_ context.Context, msg Message) error {
	addr := net.JoinHostPort(s.Host, s.Port)

	var auth smtp.Auth
	if s.User != "" {
		auth = smtp.PlainAuth("", s.User, s.Pass, s.Host)
	}

	body := strings.ReplaceAll(msg.Body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")

	data := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"utf-8\"\r\n\r\n%s\r\n",
		s.From, msg.To, msg.Subject, body,
	)

	return smtp.SendMail(addr, auth, s.From, []string{msg.To}, []byte(data))
}
