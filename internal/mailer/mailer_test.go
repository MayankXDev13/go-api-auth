package mailer

import (
	"context"
	"testing"

	"go-api-auth/internal/config"
)

func TestNew_SelectsImplementation(t *testing.T) {
	cfg := config.Default()

	cfg.SMTP.Host = ""
	if _, ok := New(cfg).(Console); !ok {
		t.Fatal("expected Console mailer when SMTP host is empty")
	}

	cfg.SMTP.Host = "smtp.example.com"
	if _, ok := New(cfg).(SMTP); !ok {
		t.Fatal("expected SMTP mailer when SMTP host is set")
	}
}

func TestConsole_Send(t *testing.T) {
	if err := (Console{}).Send(context.Background(), Message{
		To: "user@example.com", Subject: "hi", Body: "body",
	}); err != nil {
		t.Fatalf("console send: %v", err)
	}
}
